export type Box = { left: number; right: number; top: number; bottom: number };
export type CopyPopupPlace = { x: number; y: number; below: boolean };

const GAP = 8;
const HALF_WIDTH = 56; // metade da largura do botão: não deixa sair pelos lados
const NEEDS_ROOM = 52; // altura do botão + folga: sem espaço em cima, abre embaixo

/**
 * Onde o botão "Copiar" aparece: em cima do fim da seleção (a última linha), centralizado ali e sem sair da tela.
 * Se não há espaço acima (seleção no topo), vai para baixo da linha. `y` é a borda de contato com o texto.
 */
export function copyPopupPlace(last: Box, viewportWidth: number, minTop = 0): CopyPopupPlace {
  const anchor = last.right;
  const x = Math.min(Math.max(anchor, HALF_WIDTH + GAP), Math.max(HALF_WIDTH + GAP, viewportWidth - HALF_WIDTH - GAP));
  if (last.top - minTop < NEEDS_ROOM) return { x, y: last.bottom + GAP, below: true };
  return { x, y: last.top - GAP, below: false };
}

/** A última linha de uma seleção (ignora retângulos vazios que o navegador devolve entre elementos). */
export function lastLineBox(rects: ArrayLike<Box>): Box | null {
  for (let i = rects.length - 1; i >= 0; i--) {
    const r = rects[i]!;
    if (r.right - r.left > 1 && r.bottom - r.top > 1) return r;
  }
  return null;
}
