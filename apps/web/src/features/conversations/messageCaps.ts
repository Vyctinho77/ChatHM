import type { Message } from "./types";

export const EDIT_WINDOW_MS = 20 * 60_000; // limite do WhatsApp para editar
export const REVOKE_WINDOW_MS = 48 * 3_600_000; // limite para apagar para todos

/** O que dá para fazer com a mensagem (o servidor confere de novo). */
export function messageCapabilities(m: Message, now = Date.now()) {
  const age = now - Date.parse(m.sentAt);
  const out = m.direction === "out";
  const delivered = m.status === "sent" || m.status === "delivered" || m.status === "read";
  const live = !m.deleted;
  return {
    reply: live && m.status !== "pending" && m.status !== "failed",
    copy: live && copyText(m) !== "",
    forward: live && m.status !== "pending" && m.status !== "failed" && (m.kind !== "unsupported"),
    download: live && "url" in m && !!m.url && (m.kind === "image" || m.kind === "document" || m.kind === "audio" || m.kind === "sticker"),
    edit: live && out && m.kind === "text" && delivered && age <= EDIT_WINDOW_MS,
    deleteEveryone: live && out && delivered && age <= REVOKE_WINDOW_MS,
    info: live,
  };
}

/** Texto que "Copiar" coloca na área de transferência. */
export function copyText(m: Message): string {
  if (m.deleted) return "";
  switch (m.kind) {
    case "text":
      return m.text;
    case "image":
      return m.caption ?? "";
    case "document":
      return m.caption ?? "";
    case "location":
      return `https://www.google.com/maps?q=${m.lat},${m.lng}`;
    default:
      return "";
  }
}

