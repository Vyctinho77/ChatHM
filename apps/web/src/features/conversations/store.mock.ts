// Implementação com dados fictícios em memória (somente no build de demonstração).
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMe } from "@/features/auth/api";
import { buildMock } from "./mock";
import type { Capabilities } from "./store.types";
import type { Conversation, Message, SearchResult } from "./types";
import { fold, matches } from "@/lib/search";
import { playSentSound } from "@/lib/sounds";

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
    refetchInterval: (query) => query.state.data?.some((c) => c.snoozedUntil) ? 15_000 : false,
    enabled: !!who.id,
    queryFn: async () => {
      await delay(150);
      getDB(who.id, who.name).conversations.forEach((c) => {
        if (c.snoozedUntil && Date.parse(c.snoozedUntil) <= Date.now()) c.snoozedUntil = undefined;
      });
      return getDB(who.id, who.name).conversations.filter((c) => c.status !== "closed").sort((a, b) =>
        (b.lastMessage?.sentAt ?? "9").localeCompare(a.lastMessage?.sentAt ?? "9"), // sem mensagens (nova conversa) fica no topo
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

/** Demonstração: busca local nos dados fictícios. */
export function useMessageSearch(q: string) {
  const who = useIdentity();
  const term = q.trim();
  return useQuery({
    queryKey: ["search", term, db?.conversations.length ?? 0],
    enabled: term.length >= 2 && !!who.id,
    queryFn: async (): Promise<SearchResult> => {
      const d = getDB(who.id, who.name);
      const f = fold(term);
      const messages = d.conversations
        .filter((c) => c.status !== "closed")
        .flatMap((c) =>
          (d.messages[c.id] ?? []).flatMap((m) => {
            const text = m.kind === "text" ? m.text : m.kind === "document" ? m.fileName : m.kind === "image" ? (m.caption ?? "") : "";
            if (!text || !fold(text).includes(f)) return [];
            const i = fold(text).indexOf(f);
            const snippet = (i > 50 ? "…" : "") + text.slice(Math.max(0, i - 50), i + f.length + 110) + (text.length > i + f.length + 110 ? "…" : "");
            return [{ id: m.id, conversationId: c.id, contactId: c.contact.id, openConversationId: c.id, contactName: c.contact.name, direction: m.direction, kind: m.kind, snippet, sentAt: m.sentAt, closed: false }];
          }),
        )
        .sort((a, b) => b.sentAt.localeCompare(a.sentAt))
        .slice(0, 40);
      const contacts = d.conversations
        .filter((c) => c.status !== "closed" && matches(`${c.contact.name} ${c.contact.phone} ${c.contact.company ?? ""}`, term))
        .map((c) => ({ ...c.contact, conversationId: c.id }))
        .slice(0, 20);
      return { contacts, messages };
    },
  });
}

export function useContactOpener() {
  const qc = useQueryClient();
  const who = useIdentity();
  return useMemo(() => {
    const open = (c: Conversation) => {
      void qc.invalidateQueries({ queryKey: keys.list });
      return c;
    };
    const create = (contact: Conversation["contact"]) => {
      const d = getDB(who.id, who.name);
      const c: Conversation = { id: `n${Date.now()}`, contact, segment: "geral", segmentName: "Não atribuídos", status: "in_progress", assignee: { id: who.id, name: who.name }, unreadCount: 0, tags: [] };
      d.conversations = [c, ...d.conversations];
      d.messages[c.id] = [];
      return open(c);
    };
    return {
      async openContact(contactId: string) {
        const d = getDB(who.id, who.name);
        const found = d.conversations.find((c) => c.contact.id === contactId);
        if (found && found.status !== "closed") return open(found);
        return create(found?.contact ?? { id: contactId, name: "Contato", phone: "", note: "" });
      },
      async openPhone(phone: string, name = "") {
        const digits = phone.replace(/\D/g, "");
        if (digits.length < 10) throw new Error("Informe o telefone com DDD.");
        const found = getDB(who.id, who.name).conversations.find((c) => c.contact.phone.replace(/\D/g, "").endsWith(digits.slice(-10)));
        if (found && found.status !== "closed") return open(found);
        return create({ id: `p${Date.now()}`, name: name || `+55 ${digits}`, phone: `+55 ${digits}`, note: "" });
      },
    };
  }, [qc, who.id, who.name]);
}

export function useConversationActions() {
  const qc = useQueryClient();
  const who = useIdentity();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.list });

  return useMemo(() => ({
    async snooze(id: string, until: string) {
      patchConversation(id, (c) => ({ ...c, snoozedUntil: until }));
      await refresh();
    },
    async resume(id: string) {
      patchConversation(id, (c) => ({ ...c, snoozedUntil: undefined }));
      await refresh();
    },
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
    transfer(id: string, person: { id: string; name: string }) {
      patchConversation(id, (x) => ({ ...x, status: "in_progress", assignee: { id: person.id, name: person.name } }));
      refresh();
    },
    setTags(id: string, tags: string[]) {
      patchConversation(id, (x) => ({ ...x, tags }));
      refresh();
    },
    async setName(id: string, name: string) {
      patchConversation(id, (x) => ({ ...x, contact: { ...x.contact, name: name || x.contact.name } }));
      refresh();
    },
    setNote(id: string, note: string) {
      patchConversation(id, (x) => ({ ...x, contact: { ...x.contact, note } }));
      refresh();
    },
    async loadAllMessages(id: string) {
      return [...(getDB(who.id, who.name).messages[id] ?? [])];
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
    patchConversation(conversationId, (c) => (c.lastMessage?.id === messageId ? { ...c, lastMessage: apply(c.lastMessage) } : c));
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
      void playSentSound(msg.id, "sent");
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
