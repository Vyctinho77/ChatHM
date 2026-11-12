import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { decideTransfer } from "./transferFlow";
import { TransferReminderDialog } from "./TransferReminderDialog";
import type { Reminder } from "./reminders";
import type { Conversation } from "./types";

const reminder: Reminder = { conversationId: "chat", text: "Cliente espera o contrato", color: "sky" };
const conversation = (assigneeId?: string): Conversation => ({
  id: "chat", contact: { id: "contact", name: "Cliente", phone: "", note: "" }, segment: "geral", status: "in_progress",
  assignee: assigneeId ? { id: assigneeId, name: "Alguém" } : undefined, tags: [], unreadCount: 0,
});

describe("decideTransfer", () => {
  it("transfers straight away when there is no reminder to share", () => {
    expect(decideTransfer({ myId: "me", conversation: conversation("me"), always: false })).toBe("direct");
    expect(decideTransfer({ myId: "me", conversation: conversation("me"), always: true })).toBe("direct");
  });

  it("asks for consent the first time and never shares someone else's reminder", () => {
    expect(decideTransfer({ myId: "me", conversation: conversation("me"), reminder, always: false })).toBe("ask");
    expect(decideTransfer({ myId: "me", conversation: conversation("outra"), reminder, always: false })).toBe("direct");
    expect(decideTransfer({ myId: "me", conversation: conversation(), reminder, always: true })).toBe("direct");
  });

  it("shares without asking once the person chose to stop being asked", () => {
    expect(decideTransfer({ myId: "me", conversation: conversation("me"), reminder, always: true })).toBe("share");
  });
});

describe("TransferReminderDialog", () => {
  it("shows the reminder text, who gets it and the three choices", () => {
    const html = renderToStaticMarkup(<TransferReminderDialog reminder={reminder} person={{ id: "p", name: "Bruno", role: "agent", teams: [] }} onConfirm={() => undefined} onCancel={() => undefined} />);
    expect(html).toContain("Cliente espera o contrato");
    expect(html).toContain("Bruno");
    expect(html).toContain("Transferir com o lembrete");
    expect(html).toContain("Transferir sem o lembrete");
    expect(html).toContain("Cancelar");
    expect(html).toContain("não perguntar de novo");
  });
});
