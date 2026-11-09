import { describe, expect, it } from "vitest";
import { filterConversations, unreadByTab } from "./conversationFilter";
import type { Conversation } from "./types";

const conversation = (id: string, overrides: Partial<Conversation> = {}): Conversation => ({
  id, contact: { id, name: id, phone: "", note: "" }, segment: "geral", status: "in_progress",
  assignee: { id: "me", name: "Eu" }, unreadCount: 0, tags: [], ...overrides,
});

describe("postponed conversation filters", () => {
  const active = conversation("active");
  const snoozed = conversation("snoozed", { snoozedUntil: "2030-01-01T09:00:00-04:00" });

  it("keeps postponed work out of every active tab, including mine", () => {
    for (const filter of ["all", "mine"] as const) {
      expect(filterConversations([active, snoozed], filter, "me", null).map((c) => c.id)).toEqual(["active"]);
    }
    // Sem responsável, a conversa adiada também fica fora da aba Não atribuídos.
    const waiting = conversation("waiting", { assignee: undefined, status: "new" });
    const waitingSnoozed = conversation("waiting-snoozed", { assignee: undefined, status: "new", snoozedUntil: "2030-01-01T09:00:00-04:00" });
    expect(filterConversations([waiting, waitingSnoozed], "general", "me", null).map((c) => c.id)).toEqual(["waiting"]);
    expect(filterConversations([active, snoozed], "snoozed", "me", null).map((c) => c.id)).toEqual(["snoozed"]);
  });

  it("orders by the actual return time and respects the segment", () => {
    const earlier = conversation("earlier", { segment: "folha", snoozedUntil: "2030-01-01T12:00:00Z" });
    const otherQueue = conversation("other", { segment: "notas", snoozedUntil: "2030-01-01T10:00:00Z" });
    const later = conversation("later", { segment: "folha", snoozedUntil: "2030-01-02T12:00:00Z" });
    expect(filterConversations([later, otherQueue, earlier], "snoozed", "me", "folha").map((c) => c.id)).toEqual(["earlier", "later"]);
  });

  it("waits for the server to confirm return and excludes closed conversations", () => {
    const overdue = conversation("overdue", { snoozedUntil: "2000-01-01T10:00:00Z" });
    const closed = conversation("closed", { status: "closed" });
    expect(filterConversations([overdue, closed], "all", "me", null)).toEqual([]);
    expect(filterConversations([{ ...overdue, snoozedUntil: undefined }, closed], "all", "me", null).map((c) => c.id)).toEqual(["overdue"]);
  });
});

describe("não atribuídos", () => {
  const waiting = conversation("waiting", { assignee: undefined, status: "new" });
  const started = conversation("started", { assignee: { id: "me", name: "Eu" }, status: "in_progress" });
  const startedByOther = conversation("other", { assignee: { id: "ana", name: "Ana" }, status: "in_progress" });
  const folhaWaiting = conversation("folha", { segment: "folha", assignee: undefined, status: "new" });

  it("atendimento iniciado (com responsável) sai da aba, meu ou de outra pessoa", () => {
    const all = [waiting, started, startedByOther, folhaWaiting];
    expect(filterConversations(all, "general", "me", null).map((c) => c.id)).toEqual(["waiting"]);
    // continua nas outras abas
    expect(filterConversations(all, "mine", "me", null).map((c) => c.id)).toEqual(["started"]);
    expect(filterConversations(all, "all", "me", null).map((c) => c.id)).toEqual(["waiting", "started", "other", "folha"]);
  });

  it("o chip da fila geral (Não atribuídos) segue a mesma regra", () => {
    expect(filterConversations([waiting, started], "all", "me", "geral").map((c) => c.id)).toEqual(["waiting"]);
    expect(filterConversations([waiting, started, folhaWaiting], "all", "me", "folha").map((c) => c.id)).toEqual(["folha"]);
  });

  it("assumir a conversa tira da aba na hora", () => {
    const before = [waiting];
    expect(filterConversations(before, "general", "me", null)).toHaveLength(1);
    const after = [{ ...waiting, assignee: { id: "me", name: "Eu" }, status: "in_progress" as const }];
    expect(filterConversations(after, "general", "me", null)).toHaveLength(0);
  });
});

describe("números ao lado das abas", () => {
  it("contam conversas com mensagens novas em cada aba, sem adiadas nem encerradas", () => {
    const list = [
      conversation("m1", { unreadCount: 3 }),
      conversation("m2", { unreadCount: 1 }),
      conversation("m0", { unreadCount: 0 }),
      conversation("u1", { assignee: undefined, status: "new", unreadCount: 2 }),
      conversation("u2", { assignee: undefined, status: "new", unreadCount: 1 }),
      conversation("u3", { assignee: undefined, status: "new", unreadCount: 0 }),
      conversation("g1", { assignee: undefined, status: "new", unreadCount: 4, contact: { id: "g1", name: "Grupo", phone: "", note: "", kind: "group" } }),
      conversation("s1", { unreadCount: 5, snoozedUntil: "2030-01-01T09:00:00Z" }),
      conversation("c1", { unreadCount: 5, status: "closed" }),
      conversation("o1", { assignee: { id: "ana", name: "Ana" }, unreadCount: 1 }),
    ];
    expect(unreadByTab(list, "me")).toEqual({ mine: 2, general: 3, groups: 1, all: 6 });
  });

  it("vazio quando não há mensagens novas", () => {
    expect(unreadByTab([conversation("a")], "me")).toEqual({ mine: 0, general: 0, groups: 0, all: 0 });
  });
});

describe("rótulo da fila geral", () => {
  it("só diz Não atribuídos enquanto ninguém atende", async () => {
    const { segmentLabel } = await import("./types");
    expect(segmentLabel({ segment: "geral" })).toBe("Não atribuídos");
    expect(segmentLabel({ segment: "geral", assignee: { id: "ana", name: "Ana" } })).toBe("Fila geral");
    expect(segmentLabel({ segment: "folha", segmentName: "Folha de pagamento", assignee: { id: "ana" } })).toBe("Folha de pagamento");
  });
});
