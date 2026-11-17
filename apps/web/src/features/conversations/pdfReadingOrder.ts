/**
 * Ordem de leitura da camada de texto do PDF.
 *
 * O navegador seleciona na ordem em que os elementos estão no DOM, e o pdf.js os cria na ordem em que o PDF os desenhou. Em notas
 * fiscais e formulários isso mistura rótulos e valores: arrastar de A até B seleciona também tudo que o PDF desenhou "entre" eles,
 * mesmo longe na tela. Aqui os textos são reordenados como se lê:
 *   1. palavras da mesma linha e próximas viram um trecho;
 *   2. trechos empilhados e alinhados à esquerda (rótulo + valor da mesma célula, parágrafos) viram um bloco/coluna;
 *   3. os blocos seguem de cima para baixo e da esquerda para a direita, e cada bloco é lido de cima para baixo.
 * Assim, selecionar um campo pega o rótulo e o valor dele, e não os campos vizinhos.
 */

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

/** Devolve os índices das caixas em ordem de leitura. Elementos sem tamanho ficam no fim, na ordem original. */
export function readingOrder(boxes: Box[]): number[] {
  const idx = boxes.map((_, i) => i).filter((i) => boxes[i]!.width > 0 || boxes[i]!.height > 0);
  const segs = toSegments(boxes, toLines(boxes, idx));

  // Blocos: cada trecho se liga ao trecho logo acima dele quando estão alinhados à esquerda e quase encostados.
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
      const gap = cur.top - above.bottom;
      const tol = Math.max(3, 0.4 * Math.min(h, above.bottom - above.top));
      if (Math.abs(above.left - cur.left) <= tol && gap >= -0.5 * h && gap <= Math.max(h, above.bottom - above.top) && gap < bestGap) {
        best = p;
        bestGap = gap;
      }
    }
    if (best >= 0) {
      parent.set(s, best);
      hasChild.add(best);
    }
  }
  const blocks = new Map<number, number[]>(); // raiz → trechos de cima para baixo
  for (const s of sorted) {
    let root = s;
    while (parent.has(root)) root = parent.get(root)!;
    blocks.set(root, [...(blocks.get(root) ?? []), s]);
  }

  // Blocos em linhas (pelo primeiro trecho) e, na linha, da esquerda para a direita.
  const roots = [...blocks.keys()];
  const head = roots.map((r) => segs[r]!);
  const rootLines = toLines(
    head.map((s) => ({ left: s.left, top: s.top, width: s.right - s.left, height: s.bottom - s.top })),
    head.map((_, i) => i),
  );
  const out: number[] = [];
  for (const line of rootLines) {
    for (const r of line) {
      for (const s of blocks.get(roots[r]!)!) out.push(...segs[s]!.members);
    }
  }
  const seen = new Set(out);
  for (let i = 0; i < boxes.length; i++) if (!seen.has(i)) out.push(i);
  return out;
}

/** Reposiciona os elementos de texto no DOM em ordem de leitura. `container` é a camada; o marcador `endOfContent` fica por último. */
export function applyReadingOrder(container: HTMLElement, spans: HTMLElement[]): void {
  if (spans.length < 2) return;
  const origin = container.getBoundingClientRect();
  const boxes = spans.map((el) => {
    const r = el.getBoundingClientRect();
    return { left: r.left - origin.left, top: r.top - origin.top, width: r.width, height: r.height };
  });
  const order = readingOrder(boxes);
  const end = container.querySelector(".endOfContent");
  const frag = document.createDocumentFragment();
  for (const i of order) frag.appendChild(spans[i]!);
  if (end) container.insertBefore(frag, end);
  else container.appendChild(frag);
}
