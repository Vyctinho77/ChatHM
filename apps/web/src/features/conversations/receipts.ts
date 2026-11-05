import { formatTime } from "@/lib/format";
import type { DeliveryStatus, Message } from "./types";

export const statusLabel: Record<DeliveryStatus, string> = {
  pending: "Enviando",
  sent: "Enviada",
  delivered: "Entregue",
  read: "Lida",
  failed: "Falha no envio",
};

/** Texto completo do recibo, ex.: "Enviada 08:51 · Entregue 08:51 · Lida 08:53". */
export function receiptDetails(m: Message) {
  const parts = [`Enviada ${formatTime(m.sentAt)}`];
  if (m.deliveredAt) parts.push(`Entregue ${formatTime(m.deliveredAt)}`);
  if (m.readAt) parts.push(`Lida ${formatTime(m.readAt)}`);
  if (m.status === "failed") return "Falha no envio";
  if (m.status === "pending") return "Enviando…";
  return parts.join(" · ");
}
