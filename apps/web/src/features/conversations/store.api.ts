// Implementação real: API REST + atualizações via WebSocket (ver features/realtime).
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, isRejection, post, put, patch } from "@/lib/api";
import { useMe } from "@/features/auth/api";
import { pendingFiles, type Capabilities } from "./store.types";
import { tagsKey } from "@/features/tags/api";
import type { Conversation, Message, TagInfo } from "./types";

export const capabilities: Capabilities = {
  send: true,
  tags: true,
  close: true,
  media: true,
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
    const patchConv = (id: string, fn: (c: Conversation) => Conversation) =>
      qc.setQueryData<Conversation[]>(conversationKeys.list, (old) => old?.map((c) => (c.id === id ? fn(c) : c)));
    const resync = () => qc.invalidateQueries({ queryKey: conversationKeys.list });

    const markRead = (id: string) => {
      const c = list().find((x) => x.id === id);
      if (!c || c.unreadCount === 0) return;
      patchConv(id, (x) => ({ ...x, unreadCount: 0 })); // otimista; o servidor confirma via WebSocket
      post(`/conversations/${id}/read`).catch(resync);
    };

    return {
      markRead,
      markAllRead() {
        list()
          .filter((c) => c.unreadCount > 0)
          .forEach((c) => markRead(c.id));
      },
      close(id: string) {
        patchConv(id, (x) => ({ ...x, status: "closed" }));
        post(`/conversations/${id}/close`).catch(resync);
      },
      transfer(id: string, person: { id: string; name: string }) {
        patchConv(id, (x) => ({ ...x, status: "in_progress", assignee: { id: person.id, name: person.name } }));
        post(`/conversations/${id}/transfer`, { userId: person.id }).then(resync, resync);
      },
      /** Aplica a diferença entre as etiquetas atuais e as desejadas. */
      setTags(id: string, names: string[]) {
        const c = list().find((x) => x.id === id);
        if (!c) return;
        const catalog = qc.getQueryData<TagInfo[]>(tagsKey) ?? [];
        const idOf = new Map(catalog.map((t) => [t.name, t.id]));
        const add = names.filter((n) => !c.tags.includes(n) && idOf.has(n));
        const remove = c.tags.filter((n) => !names.includes(n) && idOf.has(n));
        patchConv(id, (x) => ({ ...x, tags: [...names].sort() }));
        Promise.all([
          ...add.map((n) => put<void>(`/conversations/${id}/tags/${idOf.get(n)}`)),
          ...remove.map((n) => del<void>(`/conversations/${id}/tags/${idOf.get(n)}`)),
        ]).catch(resync);
      },
      setNote(id: string, note: string) {
        const c = list().find((x) => x.id === id);
        if (!c) return;
        patchConv(id, (x) => ({ ...x, contact: { ...x.contact, note } }));
        patch(`/contacts/${c.contact.id}`, { note }).catch(resync);
      },
      me: { id: myId, name: myName },
    };
  }, [qc, myId, myName]);
}

/**
 * Envio com atualização otimista. O `id` da mensagem é gerado no cliente e vai ao
 * servidor, então repetir a requisição (duplo clique, "tentar de novo") nunca duplica.
 * Responder uma conversa sem responsável a assume — o servidor decide quem vence.
 */
export function useSendMessage(conversationId: string) {
  const qc = useQueryClient();
  const key = conversationKeys.messages(conversationId);
  const replace = (id: string, fn: (m: Message) => Message | null) =>
    qc.setQueryData<Message[]>(key, (old) =>
      old?.flatMap((m) => {
        if (m.id !== id) return [m];
        const next = fn(m);
        return next ? [next] : [];
      }),
    );

  return useMutation({
    mutationFn: async ({ msg }: { msg: Message }) => {
      const path = `/conversations/${conversationId}/messages`;
      switch (msg.kind) {
        case "text":
          return post<Message>(path, { id: msg.id, text: msg.text });
        case "location":
          return post<Message>(path, { id: msg.id, kind: "location", lat: msg.lat, lng: msg.lng, name: msg.name ?? "", address: msg.address ?? "" });
        case "image":
        case "audio":
        case "document": {
          // Reenvio depois de recarregar a página: o arquivo volta do armazenamento do servidor.
          const blob = pendingFiles.get(msg.id) ?? (msg.url ? await (await fetch(msg.url, { credentials: "include" })).blob() : undefined);
          if (!blob) throw new Error("Arquivo indisponível para reenviar.");
          const fd = new FormData();
          fd.set("id", msg.id);
          fd.set("kind", msg.kind);
          if (msg.kind !== "audio" && msg.caption) fd.set("text", msg.caption);
          if (msg.kind === "image" && msg.hd) fd.set("hd", "true");
          if (msg.kind === "audio" && msg.waveform) fd.set("waveform", JSON.stringify(msg.waveform));
          fd.set("file", blob, msg.kind === "document" ? msg.fileName : msg.kind === "audio" ? "audio" : "imagem");
          return post<Message>(path, fd);
        }
        default:
          throw new Error("Tipo de mensagem não suportado.");
      }
    },
    onMutate: ({ msg }) => {
      qc.setQueryData<Message[]>(key, (old = []) =>
        old.some((m) => m.id === msg.id)
          ? old.map((m) => (m.id === msg.id ? ({ ...m, status: "pending" } as Message) : m))
          : [...old, msg],
      );
    },
    onSuccess: (server) => {
      pendingFiles.delete(server.id);
      replace(server.id, () => server);
      void qc.invalidateQueries({ queryKey: conversationKeys.list });
    },
    onError: (err, { msg }) => {
      if (isRejection(err) && msg.status !== "failed") {
        pendingFiles.delete(msg.id);
        replace(msg.id, () => null); // nunca chegou a existir: remove a bolha
      } else {
        replace(msg.id, (m) => ({ ...m, status: "failed" }) as Message);
      }
      void qc.invalidateQueries({ queryKey: conversationKeys.list });
    },
  });
}
