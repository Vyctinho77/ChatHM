/**
 * Células de formulário do PDF (DANFE e parecidos): as bordas desenhadas na página dizem quais textos pertencem ao mesmo campo.
 * Dois textos dentro do mesmo retângulo (o rótulo no alto e o valor embaixo) são lidos juntos, mesmo longe um do outro na página,
 * e o campo ao lado não entra na seleção. Sem bordas no PDF, nada muda (vale a heurística de ordem de leitura).
 */

export type Line = { a: number; b: number; at: number }; // intervalo [a, b] ao longo da linha; `at` = posição fixa (y se horizontal, x se vertical)
export type Lines = { h: Line[]; v: Line[] };
export type Cell = { id: string; left: number; top: number; width: number; height: number };

/** Códigos do desenho de caminho no pdf.js (DrawOPS). */
const MOVE = 0;
const LINE = 1;
const CURVE = 2;
const QUAD = 3;
const CLOSE = 4;

type Matrix = [number, number, number, number, number, number];
const IDENTITY: Matrix = [1, 0, 0, 1, 0, 0];

/** Aplica `m` (como no PDF: [a b c d e f]) a um ponto. */
const apply = (m: Matrix, x: number, y: number): [number, number] => [m[0] * x + m[2] * y + m[4], m[1] * x + m[3] * y + m[5]];
/** `m1` depois de `m2` (a transformação nova `m2` age primeiro, como o operador `cm`). */
const multiply = (m1: Matrix, m2: Matrix): Matrix => [
  m2[0] * m1[0] + m2[1] * m1[2],
  m2[0] * m1[1] + m2[1] * m1[3],
  m2[2] * m1[0] + m2[3] * m1[2],
  m2[2] * m1[1] + m2[3] * m1[3],
  m2[4] * m1[0] + m2[5] * m1[2] + m1[4],
  m2[4] * m1[1] + m2[5] * m1[3] + m1[5],
];

type Ops = { save: number; restore: number; transform: number; constructPath: number; endPath: number };

/**
 * Lê a lista de operações de desenho de uma página e devolve as linhas retas (horizontais e verticais) em pixels da tela.
 * `toView` converte um ponto do PDF para a tela (a viewport do pdf.js). Curvas e linhas inclinadas são ignoradas.
 */
export function extractLines(fnArray: ArrayLike<number>, argsArray: ArrayLike<unknown>, ops: Ops, toView: (x: number, y: number) => [number, number]): Lines {
  const lines: Lines = { h: [], v: [] };
  let ctm = IDENTITY;
  const stack: Matrix[] = [];
  const add = (x1: number, y1: number, x2: number, y2: number) => {
    const dx = Math.abs(x2 - x1);
    const dy = Math.abs(y2 - y1);
    if (dy <= 0.75 && dx >= 4) lines.h.push({ a: Math.min(x1, x2), b: Math.max(x1, x2), at: (y1 + y2) / 2 });
    else if (dx <= 0.75 && dy >= 4) lines.v.push({ a: Math.min(y1, y2), b: Math.max(y1, y2), at: (x1 + x2) / 2 });
  };
  for (let i = 0; i < fnArray.length; i++) {
    const fn = fnArray[i];
    const args = argsArray[i];
    if (fn === ops.save) stack.push(ctm);
    else if (fn === ops.restore) ctm = stack.pop() ?? IDENTITY;
    else if (fn === ops.transform && Array.isArray(args) && args.length >= 6) ctm = multiply(ctm, args.slice(0, 6) as Matrix);
    else if (fn === ops.constructPath && Array.isArray(args) && args[0] !== ops.endPath) {
      const data = (Array.isArray(args[1]) ? args[1][0] : null) as ArrayLike<number> | null;
      if (!data) continue;
      let sx = 0;
      let sy = 0;
      let px = 0;
      let py = 0;
      let k = 0;
      const pt = (x: number, y: number) => toView(...apply(ctm, x, y));
      while (k < data.length) {
        const op = data[k++]!;
        if (op === MOVE) {
          [sx, sy] = pt(data[k]!, data[k + 1]!);
          [px, py] = [sx, sy];
          k += 2;
        } else if (op === LINE) {
          const [x, y] = pt(data[k]!, data[k + 1]!);
          add(px, py, x, y);
          [px, py] = [x, y];
          k += 2;
        } else if (op === CURVE) {
          [px, py] = pt(data[k + 4]!, data[k + 5]!);
          k += 6;
        } else if (op === QUAD) {
          [px, py] = pt(data[k + 2]!, data[k + 3]!);
          k += 4;
        } else if (op === CLOSE) {
          add(px, py, sx, sy);
          [px, py] = [sx, sy];
        } else {
          break; // operação desconhecida: para de ler este caminho
        }
      }
    }
  }
  return lines;
}

/** Folga (px) para a linha "cobrir" um ponto: bordas de células vizinhas raramente encostam no pixel. */
const REACH = 3;

/**
 * O retângulo (célula) que envolve o ponto: a borda mais próxima em cada um dos quatro lados, entre as linhas que realmente passam
 * por ali. `undefined` se faltar algum lado (texto fora de qualquer quadro) ou se o retângulo for grande demais (a página toda,
 * a moldura externa): isso não é um campo.
 */
export function cellAt(x: number, y: number, lines: Lines, page: { width: number; height: number }): Cell | undefined {
  let top = -Infinity;
  let bottom = Infinity;
  let left = -Infinity;
  let right = Infinity;
  for (const l of lines.h) {
    if (x < l.a - REACH || x > l.b + REACH) continue;
    if (l.at <= y && l.at > top) top = l.at;
    else if (l.at > y && l.at < bottom) bottom = l.at;
  }
  for (const l of lines.v) {
    if (y < l.a - REACH || y > l.b + REACH) continue;
    if (l.at <= x && l.at > left) left = l.at;
    else if (l.at > x && l.at < right) right = l.at;
  }
  if (!Number.isFinite(top + bottom + left + right)) return undefined;
  const width = right - left;
  const height = bottom - top;
  if (width * height > page.width * page.height * 0.35) return undefined;
  return { id: `${Math.round(left)},${Math.round(top)},${Math.round(right)},${Math.round(bottom)}`, left, top, width, height };
}
