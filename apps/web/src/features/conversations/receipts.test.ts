import { describe, expect, it } from "vitest";
import { receiptDetails } from "./receipts";
import type { Message } from "./types";

const base = {
  id: "1",
  conversationId: "c",
  direction: "out",
  kind: "text",
  text: "oi",
  sentAt: new Date(2026, 10, 5, 8, 51).toISOString(),
} as const;

describe("receiptDetails", () => {
  it("mostra cada etapa com horário", () => {
    const m = {
      ...base,
      status: "read",
      deliveredAt: new Date(2026, 10, 5, 8, 51, 20).toISOString(),
      readAt: new Date(2026, 10, 5, 8, 53).toISOString(),
    } as Message;
    expect(receiptDetails(m)).toBe("Enviada 08:51 · Entregue 08:51 · Lida 08:53");
  });

  it("estados transitórios", () => {
    expect(receiptDetails({ ...base, status: "pending" } as Message)).toBe("Enviando…");
    expect(receiptDetails({ ...base, status: "failed" } as Message)).toBe("Falha no envio");
    expect(receiptDetails({ ...base, status: "sent" } as Message)).toBe("Enviada 08:51");
  });
});
