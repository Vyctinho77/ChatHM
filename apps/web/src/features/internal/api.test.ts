import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { meKey, type Me } from "@/features/auth/api";
import { applyInternalEvent, fromServer, messagesKey, roomKey, upsertMessage, type InternalRoom, type Page } from "./api";

const ME = "me";
const me: Me = { user: { id: ME, name: "Ana", email: "ana@example.test", role: "agent", theme: "dark", mfaEnabled: true }, csrfToken: "", mfaRequired: false, mfaSetupRequired: false };
const raw = (over: Record<string, unknown> = {}) => ({
  id: "m1", conversationId: "room", seq: 1, kind: "text", text: "Oi", sentAt: "2026-11-12T10:00:00Z",
  authorId: "bia", authorName: "Bia", authorRole: "manager", authorActive: true, ...over,
});

function setup(room: Partial<InternalRoom> = {}, page: Page | null = { messages: [], hasMore: false }) {
  const qc = new QueryClient();
  qc.setQueryData(meKey, me);
  qc.setQueryData<InternalRoom>(roomKey(ME), { id: "room", name: "Escritório", unread: 0, lastReadSeq: 0, lastSeq: 0, ...room });
  if (page) qc.setQueryData<Page>(messagesKey(ME), page);
  return qc;
}

describe("fromServer", () => {
  it("decides whose message it is by the author, never trusting a direction", () => {
    expect(fromServer(raw(), ME)).toMatchObject({ direction: "in", group: true, noReceipts: true, authorName: "Bia", authorRole: "manager" });
    expect(fromServer(raw({ authorId: ME }), ME)).toMatchObject({ direction: "out", status: "sent", noReceipts: true });
  });

  it("turns the quoted message into the shape the bubble draws", () => {
    const quoted = { id: "q", authorId: ME, author: "Ana", kind: "text", text: "Pergunta original" };
    expect(fromServer(raw({ quoted }), ME).quoted).toEqual({ id: "q", author: "Ana", fromMe: true, kind: "text", text: "Pergunta original" });
    expect(fromServer(raw({ quoted: { ...quoted, authorId: "bia" } }), ME).quoted?.fromMe).toBe(false);
    expect(fromServer(raw({ quoted: { id: "q", deleted: true } }), ME).quoted).toMatchObject({ id: "q", text: "Mensagem apagada", fromMe: false });
  });
});

describe("upsertMessage", () => {
  it("appends new messages and replaces the ones it already has", () => {
    const a = fromServer(raw({ id: "a", seq: 1 }), ME);
    const b = fromServer(raw({ id: "b", seq: 2, text: "Segunda" }), ME);
    const page: Page = { messages: [a], hasMore: true };
    expect(upsertMessage(page, b)?.messages.map((m) => m.id)).toEqual(["a", "b"]);
    const edited = fromServer(raw({ id: "a", seq: 1, text: "Editada", editedAt: "2026-11-12T10:05:00Z" }), ME);
    const next = upsertMessage(upsertMessage(page, b), edited);
    expect(next?.messages).toHaveLength(2);
    expect(next?.messages[0]).toMatchObject({ text: "Editada" });
    expect(next?.hasMore).toBe(true);
    expect(upsertMessage(undefined, a)).toBeUndefined();
  });
});

describe("applyInternalEvent", () => {
  it("adds another person's message and raises the unread count", () => {
    const qc = setup();
    applyInternalEvent(qc, { type: "internal.message.created", data: raw({ id: "x", seq: 7 }) });
    expect(qc.getQueryData<Page>(messagesKey(ME))?.messages.map((m) => m.id)).toEqual(["x"]);
    expect(qc.getQueryData<InternalRoom>(roomKey(ME))).toMatchObject({ unread: 1, lastSeq: 7 });
  });

  it("does not count my own message as unread, and does not duplicate it when the send already put it there", () => {
    const qc = setup();
    const own = raw({ id: "mine", authorId: ME, seq: 3 });
    applyInternalEvent(qc, { type: "internal.message.created", data: own });
    applyInternalEvent(qc, { type: "internal.message.created", data: own });
    expect(qc.getQueryData<Page>(messagesKey(ME))?.messages).toHaveLength(1);
    expect(qc.getQueryData<InternalRoom>(roomKey(ME))?.unread).toBe(0);
  });

  it("ignores events when the chat was never opened (no cached messages) but still counts unread", () => {
    const qc = setup({}, null);
    applyInternalEvent(qc, { type: "internal.message.created", data: raw() });
    expect(qc.getQueryData(messagesKey(ME))).toBeUndefined();
    expect(qc.getQueryData<InternalRoom>(roomKey(ME))?.unread).toBe(1);
  });

  it("replaces an edited or deleted message in place", () => {
    const qc = setup({ unread: 1 }, { messages: [fromServer(raw({ id: "a" }), ME)], hasMore: false });
    applyInternalEvent(qc, { type: "internal.message.updated", data: raw({ id: "a", text: "Corrigida", editedAt: "2026-11-12T10:01:00Z" }) });
    expect(qc.getQueryData<Page>(messagesKey(ME))?.messages[0]).toMatchObject({ text: "Corrigida", editedAt: "2026-11-12T10:01:00Z" });
    applyInternalEvent(qc, { type: "internal.message.updated", data: raw({ id: "a", deleted: true, text: "" }) });
    expect(qc.getQueryData<Page>(messagesKey(ME))?.messages[0]).toMatchObject({ deleted: true, text: "" });
  });

  it("applies the read marker sent by another tab of the same person", () => {
    const qc = setup({ unread: 4, lastReadSeq: 2 });
    applyInternalEvent(qc, { type: "internal.read", data: { unread: 0, lastReadSeq: 9 } });
    expect(qc.getQueryData<InternalRoom>(roomKey(ME))).toMatchObject({ unread: 0, lastReadSeq: 9 });
  });

  it("does nothing without a signed-in person", () => {
    const qc = new QueryClient();
    qc.setQueryData<InternalRoom>(roomKey(ME), { id: "room", name: "Escritório", unread: 0, lastReadSeq: 0, lastSeq: 0 });
    applyInternalEvent(qc, { type: "internal.message.created", data: raw() });
    expect(qc.getQueryData<InternalRoom>(roomKey(ME))?.unread).toBe(0);
  });
});
