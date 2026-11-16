import { copyText } from "@/features/conversations/messageCaps";
import type { Message } from "@/features/conversations/types";

export const INTERNAL_EDIT_WINDOW_MS = 24 * 3_600_000; // o servidor confere de novo

/** O que dá para fazer com uma mensagem do chat interno (sem as regras do WhatsApp: encaminhar, dados de entrega). */
export function internalCapabilities(m: Message, o: { meId: string; canModerate: boolean; now?: number }) {
  const live = !m.deleted;
  const confirmed = m.status !== "pending" && m.status !== "failed";
  const mine = m.authorId === o.meId;
  const age = (o.now ?? Date.now()) - Date.parse(m.sentAt);
  return {
    reply: live && confirmed,
    copy: live && copyText(m) !== "",
    forward: false,
    download: live && "url" in m && !!m.url && (m.kind === "image" || m.kind === "document" || m.kind === "audio"),
    edit: live && confirmed && mine && m.kind === "text" && age <= INTERNAL_EDIT_WINDOW_MS,
    deleteEveryone: live && confirmed && (mine || o.canModerate),
    info: false,
  };
}
