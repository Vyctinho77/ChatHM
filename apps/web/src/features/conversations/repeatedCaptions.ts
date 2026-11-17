import type { Message } from "./types";

/**
 * Legenda repetida em arquivos enviados juntos. Alguns sistemas mandam vários PDFs ou fotos de uma vez repetindo a MESMA legenda em
 * cada arquivo (o texto "Bom dia, segue em anexo as guias…" aparece em cada um). Aqui se decide quais mensagens deixam de mostrar a
 * legenda: arquivos seguidos, do mesmo autor, com a mesma legenda e poucos segundos de diferença mostram o texto só no último.
 * É só exibição: a mensagem guardada continua com a legenda (busca, citação e encaminhar não mudam).
 */

/** Intervalo máximo entre dois arquivos seguidos para contarem como "enviados juntos". */
export const SAME_BATCH_MS = 60_000;

const carriesCaption = (m: Message) => (m.kind === "document" || m.kind === "image") && !m.deleted;

const norm = (s: string | undefined) => (s ?? "").replace(/\s+/g, " ").trim();

/** Ids das mensagens cuja legenda não deve aparecer porque o arquivo seguinte repete a mesma legenda. */
export function repeatedCaptionIds(list: readonly Message[]): Set<string> {
  const hidden = new Set<string>();
  for (let i = 0; i + 1 < list.length; i++) {
    const a = list[i]!;
    const b = list[i + 1]!;
    if (!carriesCaption(a) || !carriesCaption(b)) continue;
    const ca = norm((a as { caption?: string }).caption);
    if (!ca || ca !== norm((b as { caption?: string }).caption)) continue;
    if (a.conversationId !== b.conversationId || a.direction !== b.direction || (a.authorName ?? "") !== (b.authorName ?? "")) continue;
    const gap = Date.parse(b.sentAt) - Date.parse(a.sentAt);
    if (!Number.isFinite(gap) || gap < 0 || gap > SAME_BATCH_MS) continue;
    hidden.add(a.id);
  }
  return hidden;
}
