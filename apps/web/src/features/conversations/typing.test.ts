import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { applyRealtimeEvent } from "@/features/realtime/useRealtime";
import { conversationKeys } from "./store.api";
import type { Conversation } from "./types";
import { applyTyping, clearTyping, isConversationTyping, type TypingUpdate } from "./typing";

const event = (senderId = "contact", active = true): TypingUpdate => ({ conversationId: "chat", senderId, active, expiresAt: new Date(Date.now() + 10_000).toISOString() });

beforeEach(() => { vi.useFakeTimers(); clearTyping(); });
afterEach(() => { clearTyping(); vi.useRealTimers(); });

describe("transient typing indicators", () => {
  it("expires a missing pause and extends the deadline only when refreshed", () => {
    applyTyping(event());
    expect(isConversationTyping("chat")).toBe(true);
    vi.advanceTimersByTime(6_000);
    applyTyping(event());
    vi.advanceTimersByTime(4_000);
    expect(isConversationTyping("chat")).toBe(true);
    vi.advanceTimersByTime(6_000);
    expect(isConversationTyping("chat")).toBe(false);
  });

  it("stops only the participant who paused in a group", () => {
    applyTyping(event("marta"));
    applyTyping(event("joao"));
    applyTyping(event("marta", false));
    expect(isConversationTyping("chat")).toBe(true);
    applyTyping(event("joao", false));
    expect(isConversationTyping("chat")).toBe(false);
  });

  it("ignores expired events and bounds excessive deadlines", () => {
    applyTyping({ ...event(), expiresAt: new Date(Date.now() - 1).toISOString() });
    expect(isConversationTyping("chat")).toBe(false);
    applyTyping({ ...event(), expiresAt: "not-a-date" });
    expect(isConversationTyping("chat")).toBe(false);
    applyTyping({ ...event(), expiresAt: new Date(Date.now() + 86_400_000).toISOString() });
    vi.advanceTimersByTime(10_000);
    expect(isConversationTyping("chat")).toBe(false);
  });

  it("accepts only visible chats, clears revoked access and leaves messages untouched", () => {
    const qc = new QueryClient();
    const c: Conversation = { id: "chat", contact: { id: "contact", name: "Ana", phone: "", note: "" }, status: "new", segment: "geral", tags: [], unreadCount: 2 };
    qc.setQueryData(conversationKeys.list, [c]);
    applyRealtimeEvent(qc, { type: "conversation.typing", data: { ...event(), conversationId: "hidden" } });
    expect(isConversationTyping("hidden")).toBe(false);
    applyRealtimeEvent(qc, { type: "conversation.typing", data: event() });
    expect(isConversationTyping("chat")).toBe(true);
    expect(qc.getQueryData(conversationKeys.list)).toEqual([c]);
    expect(qc.getQueryData(conversationKeys.messages("chat"))).toBeUndefined();
    applyRealtimeEvent(qc, { type: "conversation.removed", data: { id: "chat" } });
    expect(isConversationTyping("chat")).toBe(false);
    applyRealtimeEvent(qc, { type: "conversation.typing", data: event() });
    expect(isConversationTyping("chat")).toBe(false);
    qc.clear();
  });

  it("clears the indicator when WhatsApp disconnects", () => {
    const qc = new QueryClient();
    applyTyping(event());
    applyRealtimeEvent(qc, { type: "whatsapp.status", data: { state: "disconnected", phone: "", displayName: "" } });
    expect(isConversationTyping("chat")).toBe(false);
    vi.advanceTimersByTime(20_000);
    expect(isConversationTyping("chat")).toBe(false);
    qc.clear();
  });
});
