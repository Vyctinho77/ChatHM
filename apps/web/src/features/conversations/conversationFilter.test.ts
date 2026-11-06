import { describe, expect, it } from "vitest";
import { filterConversations } from "./conversationFilter";
import type { Conversation } from "./types";

const conversation = (id: string, overrides: Partial<Conversation> = {}): Conversation => ({
  id, contact: { id, name: id, phone: "", note: "" }, segment: "geral", status: "in_progress",
  assignee: { id: "me", name: "Eu" }, unreadCount: 0, tags: [], ...overrides,
});

describe("postponed conversation filters", () => {
  const active = conversation("active");
  const snoozed = conversation("snoozed", { snoozedUntil: "2030-01-01T09:00:00-04:00" });

  it("keeps postponed work out of every active tab, including mine", () => {
    for (const filter of ["all", "mine", "general"] as const) {
      expect(filterConversations([active, snoozed], filter, "me", null).map((c) => c.id)).toEqual(["active"]);
    }
    expect(filterConversations([active, snoozed], "snoozed", "me", null).map((c) => c.id)).toEqual(["snoozed"]);
  });

  it("orders by the actual return time and respects the segment", () => {
    const earlier = conversation("earlier", { snoozedUntil: "2030-01-01T12:00:00Z" });
    const otherQueue = conversation("other", { segment: "folha", snoozedUntil: "2030-01-01T10:00:00Z" });
    expect(filterConversations([snoozed, otherQueue, earlier], "snoozed", "me", "geral").map((c) => c.id)).toEqual(["earlier", "snoozed"]);
  });

  it("waits for the server to confirm return and excludes closed conversations", () => {
    const overdue = conversation("overdue", { snoozedUntil: "2000-01-01T10:00:00Z" });
    const closed = conversation("closed", { status: "closed" });
    expect(filterConversations([overdue, closed], "all", "me", null)).toEqual([]);
    expect(filterConversations([{ ...overdue, snoozedUntil: undefined }, closed], "all", "me", null).map((c) => c.id)).toEqual(["overdue"]);
  });
});
