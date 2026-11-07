import { useSyncExternalStore } from "react";

export type TypingUpdate = { conversationId: string; senderId: string; active: boolean; expiresAt: string };
type Timer = ReturnType<typeof setTimeout>;
const conversations = new Map<string, Map<string, Timer>>();
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((listener) => listener());
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };

export function isConversationTyping(id: string) { return !!conversations.get(id)?.size; }

export function clearTyping(id?: string) {
  for (const [conversationId, senders] of conversations) {
    if (id && id !== conversationId) continue;
    for (const timer of senders.values()) clearTimeout(timer);
    conversations.delete(conversationId);
  }
  notify();
}

export function applyTyping(update: TypingUpdate) {
  if (!update.conversationId || !update.senderId) return;
  const remaining = Date.parse(update.expiresAt) - Date.now();
  if (update.active && (!Number.isFinite(remaining) || remaining <= 0)) return;
  let senders = conversations.get(update.conversationId);
  const previous = senders?.get(update.senderId);
  if (previous !== undefined) clearTimeout(previous);
  if (!update.active) {
    senders?.delete(update.senderId);
    if (!senders?.size) conversations.delete(update.conversationId);
  } else {
    if (!senders) { senders = new Map(); conversations.set(update.conversationId, senders); }
    // Bound remote expiry; a missing pause event must never leave the UI stuck.
    const timer = setTimeout(() => {
      const current = conversations.get(update.conversationId);
      if (current?.get(update.senderId) !== timer) return;
      current.delete(update.senderId);
      if (!current.size) conversations.delete(update.conversationId);
      notify();
    }, Math.min(remaining, 10_000));
    senders.set(update.senderId, timer);
  }
  notify();
}

export function useTyping(conversationId: string) {
  return useSyncExternalStore(subscribe, () => isConversationTyping(conversationId), () => false);
}
