import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { applyRealtimeEvent } from "./useRealtime";
import { connectionKey, type Connection } from "@/features/whatsapp/api";
import { conversationKeys } from "@/features/conversations/store.api";
import type { Conversation } from "@/features/conversations/types";
import { reminderKey } from "@/features/conversations/reminders";
import { clearNotices } from "@/lib/notices";

describe("conversation notices", () => {
  it("tells the people in the atendimento when someone else closed it for everyone, or handed it over to them", async () => {
    const qc = new QueryClient();
    clearNotices();
    applyRealtimeEvent(qc, { type: "conversation.notice", data: { id: "c1", kind: "closed_for_all", by: "Admin Um", contact: "Mercado Silva" } });
    applyRealtimeEvent(qc, { type: "conversation.notice", data: { id: "c2", kind: "handed_over", by: "Ana", contact: "Clínica Sol" } });
    applyRealtimeEvent(qc, { type: "conversation.notice", data: { id: "c2", kind: "handed_over", by: "Ana", contact: "Clínica Sol" } }); // repetido
    const { renderToStaticMarkup } = await import("react-dom/server");
    const { createElement } = await import("react");
    const { useNotices } = await import("@/lib/notices");
    const html = renderToStaticMarkup(createElement(function Probe() { return createElement("div", null, useNotices().map((n) => `${n.text}|${n.link?.to ?? ""}`).join("\n")); }));
    expect(html).toContain("Admin Um encerrou o atendimento de Mercado Silva para todos.|");
    expect(html).toContain("Ana passou o atendimento de Clínica Sol para você.|/conversas/c2");
    expect(html.match(/passou o atendimento/g)).toHaveLength(1);
    clearNotices();
    qc.clear();
  });

  it("falls back to neutral wording without the person or contact name", async () => {
    const qc = new QueryClient();
    clearNotices();
    applyRealtimeEvent(qc, { type: "conversation.notice", data: { id: "c3", kind: "closed_for_all" } });
    const { renderToStaticMarkup } = await import("react-dom/server");
    const { createElement } = await import("react");
    const { useNotices } = await import("@/lib/notices");
    expect(renderToStaticMarkup(createElement(function Probe() { return createElement("div", null, useNotices()[0]?.text); }))).toContain("Alguém encerrou o atendimento para todos.");
    clearNotices();
  });
});

describe("WhatsApp events", () => {
  it("refreshes personal reminders without mixing them into the shared conversation", () => {
    const qc = new QueryClient();
    qc.setQueryData([...reminderKey, "me"], {});
    qc.setQueryData(conversationKeys.list, []);
    applyRealtimeEvent(qc, { type: "conversation.reminder.updated", data: { id: "chat" } });
    expect(qc.getQueryState([...reminderKey, "me"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(conversationKeys.list)?.isInvalidated).toBe(false);
    qc.clear();
  });
  it("updates postponed state and clears it when the server resumes the conversation", () => {
    const qc = new QueryClient();
    const c: Conversation = { id: "chat", contact: { id: "contact", name: "Ana", phone: "", note: "" }, status: "in_progress", segment: "geral", tags: [], unreadCount: 0 };
    qc.setQueryData<Conversation[]>(conversationKeys.list, [c]);
    applyRealtimeEvent(qc, { type: "conversation.updated", data: { ...c, snoozedUntil: "2030-01-01T12:00:00Z" } });
    expect(qc.getQueryData<Conversation[]>(conversationKeys.list)?.[0]?.snoozedUntil).toBeDefined();
    applyRealtimeEvent(qc, { type: "conversation.updated", data: c });
    expect(qc.getQueryData<Conversation[]>(conversationKeys.list)?.[0]?.snoozedUntil).toBeUndefined();
    qc.clear();
  });
  it("removes the QR as soon as the connection is confirmed and ignores late QR events", () => {
    const qc = new QueryClient();
    const qr = { code: "temporary", expiresAt: "2030-01-01T00:00:00Z" };
    qc.setQueryData<Connection>(connectionKey, { state: "pending_qr", phone: "", displayName: "", qr });
    applyRealtimeEvent(qc, { type: "whatsapp.status", data: { state: "connected", phone: "+5592000000000", displayName: "Escritório" } });
    applyRealtimeEvent(qc, { type: "whatsapp.qr", data: qr });
    expect(qc.getQueryData<Connection>(connectionKey)?.state).toBe("connected");
    expect(qc.getQueryData<Connection>(connectionKey)?.qr).toBeNull();
    qc.clear();
  });

  it("refreshes conversations and messages when a history chunk finishes", () => {
    const qc = new QueryClient();
    qc.setQueryData<Connection>(connectionKey, { state: "connected", phone: "", displayName: "" });
    qc.setQueryData(["conversations"], []);
    qc.setQueryData(["conversations", "chat", "messages"], []);
    applyRealtimeEvent(qc, { type: "whatsapp.sync", data: { state: "ready", messages: 20, contacts: 3, failed: 0 } });
    expect(qc.getQueryData<Connection>(connectionKey)?.sync?.messages).toBe(20);
    expect(qc.getQueryState(["conversations"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["conversations", "chat", "messages"])?.isInvalidated).toBe(true);
    qc.clear();
  });
});
