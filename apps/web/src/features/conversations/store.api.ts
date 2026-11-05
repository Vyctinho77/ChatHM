// Implementação real: API REST + atualizações via WebSocket (ver features/realtime).
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, patch, post } from "@/lib/api";
import { useMe } from "@/features/auth/api";
import type { Capabilities } from "./store.types";
import type { Conversation, Message } from "./types";

// Fase 2: receber. Responder (com atribuição atômica ao primeiro atendente) chega na Fase 3.
export const capabilities: Capabilities = {
  send: false,
  tags: false,
  close: false,
  sendUnavailableReason: "Responder pelo ChatHM chega na próxima etapa (atribuição e envio).",
};

export const conversationKeys = {
  list: ["conversations"] as const,
  messages: (id: string) => ["conversations", id, "messages"] as const,
};

export function useConversations() {
  return useQuery({
    queryKey: conversationKeys.list,
    queryFn: async () => (await get<{ conversations: Conversation[] }>("/conversations")).conversations,
  });
}

export function useMessages(conversationId: string | undefined) {
  return useQuery({
    queryKey: conversationKeys.messages(conversationId ?? ""),
    enabled: !!conversationId,
    queryFn: async () => (await get<{ messages: Message[] }>(`/conversations/${conversationId}/messages`)).messages,
  });
}

export function useConversationActions() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const myId = me?.user.id ?? "";
  const myName = me?.user.name ?? "";

  return useMemo(() => {
    const list = () => qc.getQueryData<Conversation[]>(conversationKeys.list) ?? [];
    const setUnread = (id: string, n: number) =>
      qc.setQueryData<Conversation[]>(conversationKeys.list, (old) =>
        old?.map((c) => (c.id === id ? { ...c, unreadCount: n } : c)),
      );
    const markRead = (id: string) => {
      const c = list().find((x) => x.id === id);
      if (!c || c.unreadCount === 0) return;
      setUnread(id, 0); // otimista; o servidor confirma via WebSocket
      post(`/conversations/${id}/read`).catch(() => qc.invalidateQueries({ queryKey: conversationKeys.list }));
    };
    return {
      markRead,
      markAllRead() {
        list()
          .filter((c) => c.unreadCount > 0)
          .forEach((c) => markRead(c.id));
      },
      close() {},
      setTags() {},
      setNote(id: string, note: string) {
        const c = list().find((x) => x.id === id);
        if (!c) return;
        qc.setQueryData<Conversation[]>(conversationKeys.list, (old) =>
          old?.map((x) => (x.id === id ? { ...x, contact: { ...x.contact, note } } : x)),
        );
        patch(`/contacts/${c.contact.id}`, { note }).catch(() => qc.invalidateQueries({ queryKey: conversationKeys.list }));
      },
      me: { id: myId, name: myName },
    };
  }, [qc, myId, myName]);
}

export function useSendMessage(conversationId: string) {
  void conversationId;
  return useMutation({
    mutationFn: async (_: { msg: Message }): Promise<Message> => {
      void _;
      throw new Error(capabilities.sendUnavailableReason);
    },
  });
}
