// Implementação com dados fictícios em memória (somente no build de demonstração).
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMe } from "@/features/auth/api";
import { buildMock } from "./mock";
import type { Capabilities } from "./store.types";
import type { Conversation, Message } from "./types";

export const capabilities: Capabilities = { send: true, tags: true, close: true, media: true };

type DB = ReturnType<typeof buildMock>;
let db: DB | null = null;

function getDB(myId: string, myName: string) {
  if (!db) db = buildMock(myId, myName);
  return db;
}

const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

const keys = {
  list: ["conversations"] as const,
  messages: (id: string) => ["conversations", id, "messages"] as const,
};

function useIdentity() {
  const { data: me } = useMe();
  return { id: me?.user.id ?? "", name: me?.user.name ?? "" };
}

export function useConversations() {
  const who = useIdentity();
  return useQuery({
    queryKey: keys.list,
    enabled: !!who.id,
    queryFn: async () => {
      await delay(150);
      return [...getDB(who.id, who.name).conversations].sort((a, b) =>
        b.lastMessage.sentAt.localeCompare(a.lastMessage.sentAt),
      );
    },
  });
}

export function useMessages(conversationId: string | undefined) {
  const who = useIdentity();
  return useQuery({
    queryKey: keys.messages(conversationId ?? ""),
    enabled: !!conversationId && !!who.id,
    queryFn: async () => {
      await delay(120);
      return [...(getDB(who.id, who.name).messages[conversationId!] ?? [])];
    },
  });
}

function patchConversation(id: string, fn: (c: Conversation) => Conversation) {
  if (!db) return;
  db.conversations = db.conversations.map((c) => (c.id === id ? fn(c) : c));
}

export function useConversationActions() {
  const qc = useQueryClient();
  const who = useIdentity();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.list });

  return useMemo(() => ({
    markRead(id: string) {
      const c = db?.conversations.find((x) => x.id === id);
      if (!c || c.unreadCount === 0) return;
      patchConversation(id, (x) => ({ ...x, unreadCount: 0 }));
      refresh();
    },
    markAllRead() {
      db?.conversations.forEach((c) => patchConversation(c.id, (x) => ({ ...x, unreadCount: 0 })));
      refresh();
    },
    close(id: string) {
      patchConversation(id, (x) => ({ ...x, status: "closed" }));
      refresh();
    },
    setTags(id: string, tags: string[]) {
      patchConversation(id, (x) => ({ ...x, tags }));
      refresh();
    },
    setNote(id: string, note: string) {
      patchConversation(id, (x) => ({ ...x, contact: { ...x.contact, note } }));
      refresh();
    },
    me: who,
  }), [qc, who.id, who.name]); // eslint-disable-line react-hooks/exhaustive-deps
}

function patchMessage(
  qc: ReturnType<typeof useQueryClient>,
  conversationId: string,
  messageId: string,
  patch: Partial<Message>,
) {
  const apply = (m: Message) => (m.id === messageId ? ({ ...m, ...patch } as Message) : m);
  if (db) {
    db.messages[conversationId] = (db.messages[conversationId] ?? []).map(apply);
    patchConversation(conversationId, (c) => (c.lastMessage.id === messageId ? { ...c, lastMessage: apply(c.lastMessage) } : c));
  }
  qc.setQueryData<Message[]>(keys.messages(conversationId), (old = []) => old.map(apply));
  void qc.invalidateQueries({ queryKey: keys.list });
}

/**
 * Simula os recibos que o WhatsApp enviará (Fase 2): entregue e depois lida.
 * Na integração real esses eventos chegam pelo WebSocket.
 */
function simulateReceipts(qc: ReturnType<typeof useQueryClient>, conversationId: string, messageId: string) {
  setTimeout(() => patchMessage(qc, conversationId, messageId, { status: "delivered", deliveredAt: new Date().toISOString() }), 1200);
  setTimeout(() => patchMessage(qc, conversationId, messageId, { status: "read", readAt: new Date().toISOString() }), 3600);
}

/**
 * Envio com atualização otimista: a mensagem aparece como "pendente" e muda para
 * "enviada" após confirmação. Responder uma conversa sem responsável a assume.
 */
export function useSendMessage(conversationId: string) {
  const qc = useQueryClient();
  const who = useIdentity();
  return useMutation({
    mutationFn: async ({ msg }: { msg: Message }) => {
      await delay(600);
      return msg;
    },
    onMutate: ({ msg }) => {
      // Reenvio de uma mensagem que falhou reaproveita a mesma bolha (sem duplicar).
      qc.setQueryData<Message[]>(keys.messages(conversationId), (old = []) =>
        old.some((m) => m.id === msg.id)
          ? old.map((m) => (m.id === msg.id ? ({ ...m, status: "pending" } as Message) : m))
          : [...old, msg],
      );
    },
    onSuccess: (msg) => {
      const sent = { ...msg, status: "sent" } as Message;
      if (db) db.messages[conversationId] = [...(db.messages[conversationId] ?? []), sent];
      qc.setQueryData<Message[]>(keys.messages(conversationId), (old = []) =>
        old.map((m) => (m.id === msg.id ? sent : m)),
      );
      simulateReceipts(qc, conversationId, msg.id);
      patchConversation(conversationId, (c) => ({
        ...c,
        lastMessage: sent,
        status: "in_progress",
        assignee: c.assignee ?? { id: who.id, name: who.name },
      }));
      qc.invalidateQueries({ queryKey: keys.list });
    },
    onError: (_e, { msg }) => {
      qc.setQueryData<Message[]>(keys.messages(conversationId), (old = []) =>
        old.map((m) => (m.id === msg.id ? ({ ...m, status: "failed" } as Message) : m)),
      );
    },
  });
}
