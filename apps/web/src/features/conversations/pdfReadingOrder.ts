/**
 * Ordem de leitura da camada de texto do PDF.
 *
 * O navegador seleciona na ordem em que os elementos estão no DOM, e o pdf.js os cria na ordem em que o PDF os desenhou. Em notas
 * fiscais e formulários isso mistura rótulos e valores: arrastar de A até B seleciona também tudo que o PDF desenhou "entre" eles,
 * mesmo longe na tela. Aqui os textos são reordenados como se lê:
 *   1. palavras da mesma linha e próximas viram um trecho;
 *   2. trechos empilhados na mesma coluna (rótulo + valor da mesma célula, parágrafos) viram um bloco;
 *   3. os blocos seguem de cima para baixo e da esquerda para a direita, e cada bloco é lido de cima para baixo.
 * Assim, selecionar um campo pega o rótulo e o valor dele, e não os campos vizinhos.
 */

import type { Cell, Lines } from "./pdfCells";
import { cellAt } from "./pdfCells";

export type Box = { left: number; top: number; width: number; height: number };

type Seg = { members: number[]; left: number; top: number; right: number; bottom: number };

/** Agrupa as caixas em linhas (centro vertical parecido), cada linha já em ordem da esquerda para a direita. */
function toLines(boxes: Box[], idx: number[]): number[][] {
  const c = (i: number) => boxes[i]!.top + boxes[i]!.height / 2;
  const sorted = [...idx].sort((a, b) => c(a) - c(b) || boxes[a]!.left - boxes[b]!.left);
  const lines: number[][] = [];
  let lineCenter = 0;
  let lineHeight = 0;
  for (const i of sorted) {
    const b = boxes[i]!;
    if (lines.length && Math.abs(c(i) - lineCenter) <= Math.min(lineHeight, b.height) * 0.5) {
      const line = lines[lines.length - 1]!;
      line.push(i);
      lineCenter = line.reduce((s, k) => s + c(k), 0) / line.length;
      lineHeight = Math.min(lineHeight, b.height);
    } else {
      lines.push([i]);
      lineCenter = c(i);
      lineHeight = b.height;
    }
  }
  for (const line of lines) line.sort((a, b) => boxes[a]!.left - boxes[b]!.left);
  return lines;
}

/** Junta, em cada linha, os elementos vizinhos (espaço menor que ~1,5 altura de letra) num só trecho. */
function toSegments(boxes: Box[], lines: number[][]): Seg[] {
  const segs: Seg[] = [];
  for (const line of lines) {
    let cur: Seg | null = null;
    for (const i of line) {
      const b = boxes[i]!;
      const h = Math.max(b.height, 1);
      if (cur && b.left - cur.right <= h * 1.5) {
        cur.members.push(i);
        cur.right = Math.max(cur.right, b.left + b.width);
        cur.top = Math.min(cur.top, b.top);
        cur.bottom = Math.max(cur.bottom, b.top + b.height);
      } else {
        cur = { members: [i], left: b.left, top: b.top, right: b.left + b.width, bottom: b.top + b.height };
        segs.push(cur);
      }
    }
  }
  return segs;
}

type Block = { head: Box; order: number[] };

/** Blocos dos textos soltos (sem célula): trechos empilhados na mesma coluna viram um bloco. */
function freeBlocks(boxes: Box[], idx: number[]): Block[] {
  if (!idx.length) return [];
  const segs = toSegments(boxes, toLines(boxes, idx));

  // Cada trecho se liga ao trecho logo acima dele quando estão na mesma coluna (os dois ocupam a mesma faixa horizontal, como as
  // linhas de um parágrafo) e a distância vertical é de poucas linhas de texto.
  const sorted = segs.map((_, i) => i).sort((a, b) => segs[a]!.top - segs[b]!.top || segs[a]!.left - segs[b]!.left);
  const parent = new Map<number, number>();
  const hasChild = new Set<number>();
  for (const s of sorted) {
    const cur = segs[s]!;
    const h = cur.bottom - cur.top;
    let best = -1;
    let bestGap = Infinity;
    for (const p of sorted) {
      if (p === s) break;
      const above = segs[p]!;
      if (hasChild.has(p)) continue;
      const hp = above.bottom - above.top;
      const gap = cur.top - above.bottom;
      const overlap = Math.min(above.right, cur.right) - Math.max(above.left, cur.left);
      const narrow = Math.min(above.right - above.left, cur.right - cur.left);
      if (overlap >= 0.4 * narrow && gap >= -0.5 * h && gap <= 3 * Math.max(h, hp) && gap < bestGap) {
        best = p;
        bestGap = gap;
      }
    }
    if (best >= 0) {
      parent.set(s, best);
      hasChild.add(best);
    }
  }
  const byRoot = new Map<number, number[]>(); // raiz → trechos de cima para baixo
  for (const s of sorted) {
    let root = s;
    while (parent.has(root)) root = parent.get(root)!;
    byRoot.set(root, [...(byRoot.get(root) ?? []), s]);
  }
  return [...byRoot.entries()].map(([root, list]) => {
    const h = segs[root]!;
    return { head: { left: h.left, top: h.top, width: h.right - h.left, height: h.bottom - h.top }, order: list.flatMap((s) => segs[s]!.members) };
  });
}

/**
 * Devolve os índices das caixas em ordem de leitura. Elementos sem tamanho ficam no fim, na ordem original.
 * `cells[i]` é a célula (retângulo desenhado no PDF) em que o texto i está, se houver: os textos de uma célula formam um bloco só.
 */
export function readingOrder(boxes: Box[], cells?: (Cell | undefined)[]): number[] {
  const idx = boxes.map((_, i) => i).filter((i) => boxes[i]!.width > 0 || boxes[i]!.height > 0);
  const inCell = new Map<string, { cell: Cell; items: number[] }>();
  const free: number[] = [];
  for (const i of idx) {
    const c = cells?.[i];
    if (!c) free.push(i);
    else {
      const e = inCell.get(c.id) ?? { cell: c, items: [] };
      e.items.push(i);
      inCell.set(c.id, e);
    }
  }
  const blocks: Block[] = [
    ...[...inCell.values()].map(({ cell, items }) => ({
      head: { left: cell.left, top: cell.top, width: cell.width, height: cell.height },
      order: toLines(boxes, items).flat(),
    })),
    ...freeBlocks(boxes, free),
  ];

  // Blocos em linhas (pelo canto superior) e, na linha, da esquerda para a direita.
  const out: number[] = [];
  for (const line of toLines(blocks.map((b) => b.head), blocks.map((_, i) => i))) {
    for (const b of line) out.push(...blocks[b]!.order);
  }
  const seen = new Set(out);
  for (let i = 0; i < boxes.length; i++) if (!seen.has(i)) out.push(i);
  return out;
}

/**
 * Reposiciona os elementos de texto no DOM em ordem de leitura. `container` é a camada; o marcador `endOfContent` fica por último.
 * `lines`: bordas desenhadas na página (px da tela), para agrupar os textos por célula; `page`: tamanho da página (px).
 */
export function applyReadingOrder(container: HTMLElement, spans: HTMLElement[], lines?: Lines, page?: { width: number; height: number }): void {
  if (spans.length < 2) return;
  const origin = container.getBoundingClientRect();
  const boxes = spans.map((el) => {
    const r = el.getBoundingClientRect();
    return { left: r.left - origin.left, top: r.top - origin.top, width: r.width, height: r.height };
  });
  const size = page ?? { width: origin.width, height: origin.height };
  const cells = lines && (lines.h.length || lines.v.length) ? boxes.map((b) => cellAt(b.left + b.width / 2, b.top + b.height / 2, lines, size)) : undefined;
  const order = readingOrder(boxes, cells);
  const end = container.querySelector(".endOfContent");
  const frag = document.createDocumentFragment();
  for (const i of order) frag.appendChild(spans[i]!);
  if (end) container.insertBefore(frag, end);
  else container.appendChild(frag);
}
