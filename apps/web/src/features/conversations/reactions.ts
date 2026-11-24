import type { Message } from "./types";

export type Reaction = { emoji: string; count: number; mine: boolean };

/** As reações rápidas da barra (as mesmas do WhatsApp). */
export const QUICK_REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🙏"] as const;

/** O emoji com que o escritório reagiu a esta mensagem (ou undefined). */
export function myReaction(m: Message): string | undefined {
  return m.reactions?.find((r) => r.mine)?.emoji;
}

/** Escolher o mesmo emoji de novo remove a reação (vazio); outro emoji troca. */
export function nextReaction(m: Message, emoji: string): string {
  return myReaction(m) === emoji ? "" : emoji;
}

/** As reações depois de o escritório reagir com `emoji` ("" remove): uma só por conta, trocar substitui. Usada na tela antes de o servidor confirmar. */
export function withMyReaction(list: Reaction[] | undefined, emoji: string): Reaction[] {
  const out: Reaction[] = [];
  for (const r of list ?? []) {
    if (!r.mine) {
      out.push(r);
      continue;
    }
    if (r.count > 1) out.push({ ...r, count: r.count - 1, mine: false }); // outras pessoas ainda usam este emoji
  }
  if (emoji) {
    const same = out.find((r) => r.emoji === emoji);
    if (same) {
      same.count += 1;
      same.mine = true;
    } else {
      out.push({ emoji, count: 1, mine: true });
    }
  }
  return out;
}
