/**
 * Busca de texto no PDF. Trabalha sobre os textos de cada página (os mesmos itens que o pdf.js usa para montar a camada de texto),
 * ignorando maiúsculas/minúsculas e acentos, e devolve cada ocorrência como trechos (item, início, fim) que a tela transforma em
 * destaques. Uma ocorrência pode atravessar itens vizinhos (palavra partida pelo PDF), então ela vem em vários trechos.
 */

export type Piece = { item: number; start: number; end: number };
export type Match = { pieces: Piece[] };

/** Limite de ocorrências por página: evita travar a tela com uma busca de uma letra num PDF enorme. */
export const MAX_MATCHES_PER_PAGE = 500;

/** Minúscula e sem acento, mantendo o tamanho do texto (posição por posição) para os índices continuarem valendo. */
export function fold(s: string): string {
  let out = "";
  for (let i = 0; i < s.length; i++) {
    const c = s.charAt(i);
    if (c === " ") {
      out += " ";
      continue;
    }
    out += c.normalize("NFD").charAt(0).toLowerCase().charAt(0) || c;
  }
  return out;
}

/** Acha as ocorrências de `query` nos itens de texto de uma página (em ordem). */
export function findMatches(items: string[], query: string): Match[] {
  const q = fold(query);
  if (!q.trim()) return [];
  const starts: number[] = [];
  let joined = "";
  for (const it of items) {
    starts.push(joined.length);
    joined += it;
  }
  const hay = fold(joined);
  const out: Match[] = [];
  let from = 0;
  let item = 0;
  for (;;) {
    const at = hay.indexOf(q, from);
    if (at < 0 || out.length >= MAX_MATCHES_PER_PAGE) break;
    const end = at + q.length;
    while (item + 1 < items.length && starts[item + 1]! <= at) item++;
    const pieces: Piece[] = [];
    for (let k = item; k < items.length && starts[k]! < end; k++) {
      const s = Math.max(at, starts[k]!) - starts[k]!;
      const e = Math.min(end, starts[k]! + items[k]!.length) - starts[k]!;
      if (e > s) pieces.push({ item: k, start: s, end: e });
    }
    if (pieces.length) out.push({ pieces });
    from = end;
  }
  return out;
}
