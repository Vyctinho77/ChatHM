// Chat interno do escritório: dados vindos de /api/v1/internal-chat e mantidos em dia pelo tempo real.
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { meKey, useMe, type Me } from "@/features/auth/api";
import { api, del, get, isRejection, patch, post } from "@/lib/api";
import type { Conversation } from "@/features/conversations/types";
import { pendingFiles, type OutgoingDraft, buildOutgoing } from "@/features/conversations/store.types";
import type { MentionRef } from "@/features/conversations/mentions";
import { INTERNAL_SOUND_CONVERSATION, getInternalChatSound, playIncomingSound } from "@/lib/sounds";
import type { Message, Quoted } from "@/features/conversations/types";

export type InternalRoom = { id: string; name: string; unread: number; mentions: number; lastReadSeq: number; lastSeq: number };
/** `seq` é a ordem no servidor (ausente nas mensagens ainda não confirmadas). */
export type InternalMessage = Message & { seq?: number };
export type Page = { messages: InternalMessage[]; hasMore: boolean };

/** Como o servidor entrega uma mensagem: o mesmo formato do chat principal, sem `direction`. */
type Raw = Record<string, unknown> & {
  id: string;
  authorId: string;
  quoted?: { id: string; authorId?: string; author?: string; kind?: string; text?: string; deleted?: boolean };
};

export const roomKey = (userId: string) => ["internal-chat", "room", userId] as const;
export const messagesKey = (userId: string) => ["internal-chat", "messages", userId] as const;

/** Quem escreveu decide se a mensagem é minha (o mesmo evento chega a todas as sessões). */
export function fromServer(raw: Raw, meId: string): InternalMessage {
  const mine = raw.authorId === meId;
  let quoted: Quoted | undefined;
  if (raw.quoted) {
    const q = raw.quoted;
    quoted = q.deleted
      ? { id: q.id, author: "Equipe", fromMe: false, kind: "text", text: "Mensagem apagada" }
      : { id: q.id, author: q.author ?? "", fromMe: q.authorId === meId, kind: q.kind ?? "text", text: q.text ?? "" };
  }
  return {
    ...raw,
    direction: mine ? "out" : "in",
    ...(mine ? { status: "sent" } : {}),
    group: true, // mostra quem escreveu também nas recebidas
    noReceipts: true, // sem tiques: não há entrega nem leitura por pessoa
    ...(quoted ? { quoted } : {}),
  } as unknown as InternalMessage;
}

/** Atualiza a mensagem se já existe (o envio, o evento e a resposta podem chegar em qualquer ordem); senão, acrescenta ao fim. */
export function upsertMessage(page: Page | undefined, m: InternalMessage): Page | undefined {
  if (!page) return page;
  const i = page.messages.findIndex((x) => x.id === m.id);
  if (i < 0) return { ...page, messages: [...page.messages, m] };
  const messages = page.messages.slice();
  messages[i] = m;
  return { ...page, messages };
}

export function useInternalRoom() {
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useQuery({
    queryKey: roomKey(userId),
    enabled: !!userId,
    staleTime: 30_000,
    queryFn: () => get<InternalRoom>("/internal-chat/room"),
  });
}

/** Mensagens não lidas e quantas delas mencionam a pessoa (selo da aba). */
export function useInternalCounts() {
  const room = useInternalRoom().data;
  return { unread: room?.unread ?? 0, mentions: room?.mentions ?? 0 };
}

/** Os dados que o cartão de uma conversa compartilhada mostra; só existem para quem pode ver a conversa. */
export type SharedConversation = { id: string; status: Conversation["status"]; assignee?: string; contact: { name: string; phone: string; saved: boolean; kind: string } };

/** Mesmo título da lista de conversas: grupos e contatos salvos pelo nome; os demais pelo telefone. */
export const contactTitle = (c: SharedConversation["contact"]) => (c.kind === "group" || c.saved ? c.name : c.phone || c.name);

export function useSharedConversation(id: string) {
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useQuery({
    queryKey: ["internal-chat", "shared", userId, id] as const,
    enabled: !!userId,
    staleTime: 30_000,
    retry: false, // 404 = sem acesso; repetir não muda nada
    queryFn: () => get<SharedConversation>(`/internal-chat/shared-conversations/${id}`),
  });
}

/** Mensagem do cartão de conversa compartilhada, ainda não confirmada pelo servidor. */
export function buildSharedConversationMessage(me: Me["user"], roomId: string, sharedConversationId: string, comment: string, quoted?: Quoted): InternalMessage {
  return {
    id: crypto.randomUUID(),
    conversationId: roomId,
    direction: "out",
    sentAt: new Date().toISOString(),
    status: "pending",
    authorName: me.name,
    authorId: me.id,
    authorRole: me.role,
    group: true,
    noReceipts: true,
    kind: "conversation",
    sharedConversationId,
    ...(comment.trim() ? { caption: comment.trim() } : {}),
    ...(quoted ? { quoted } : {}),
  } as InternalMessage;
}

export function useInternalMessages() {
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useQuery({
    queryKey: messagesKey(userId),
    enabled: !!userId,
    queryFn: async (): Promise<Page> => {
      const r = await get<{ messages: Raw[]; hasMore: boolean }>("/internal-chat/messages?limit=50");
      return { messages: r.messages.map((m) => fromServer(m, userId)), hasMore: r.hasMore };
    },
  });
}

/** Busca as mensagens anteriores às já carregadas e as põe no começo. */
export function useLoadOlder() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return async function loadOlder() {
    const page = qc.getQueryData<Page>(messagesKey(userId));
    const first = page?.messages.find((m) => m.seq !== undefined)?.seq;
    if (!page?.hasMore || first === undefined) return false;
    const r = await get<{ messages: Raw[]; hasMore: boolean }>(`/internal-chat/messages?limit=50&before=${first}`);
    const older = r.messages.map((m) => fromServer(m, userId));
    qc.setQueryData<Page>(messagesKey(userId), (old) => {
      if (!old) return old;
      const have = new Set(old.messages.map((m) => m.id));
      return { messages: [...older.filter((m) => !have.has(m.id)), ...old.messages], hasMore: r.hasMore };
    });
    return true;
  };
}

/** Mensagem ainda não confirmada pelo servidor (aparece na hora, com o relógio de "enviando"). */
export function buildInternalOutgoing(me: Me["user"], roomId: string, d: OutgoingDraft, quoted?: Quoted): InternalMessage {
  const m = buildOutgoing(roomId, me.name, d, quoted);
  return { ...m, authorId: me.id, authorRole: me.role, group: true, noReceipts: true } as InternalMessage;
}

export function useSendInternal() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  const key = messagesKey(userId);
  const replace = (id: string, fn: (m: InternalMessage) => InternalMessage | null) =>
    qc.setQueryData<Page>(key, (old) =>
      old && {
        ...old,
        messages: old.messages.flatMap((m) => {
          if (m.id !== id) return [m];
          const next = fn(m);
          return next ? [next] : [];
        }),
      },
    );

  return useMutation({
    mutationFn: async ({ msg }: { msg: InternalMessage }) => {
      const path = "/internal-chat/messages";
      switch (msg.kind) {
        case "text":
          return post<Raw>(path, { id: msg.id, text: msg.text, replyTo: msg.quoted?.id, mentions: msg.mentions ?? [] });
        case "conversation":
          return post<Raw>(path, { id: msg.id, kind: "conversation", sharedConversationId: msg.sharedConversationId, text: msg.caption ?? "", replyTo: msg.quoted?.id });
        case "location":
          return post<Raw>(path, { id: msg.id, kind: "location", lat: msg.lat, lng: msg.lng, name: msg.name ?? "", address: msg.address ?? "" });
        case "image":
        case "audio":
        case "document": {
          const blob = pendingFiles.get(msg.id) ?? (msg.url ? await (await fetch(msg.url, { credentials: "include" })).blob() : undefined);
          if (!blob) throw new Error("Arquivo indisponível para reenviar.");
          const fd = new FormData();
          fd.set("id", msg.id);
          fd.set("kind", msg.kind);
          if (msg.quoted?.id) fd.set("replyTo", msg.quoted.id);
          if (msg.kind !== "audio" && msg.caption) fd.set("text", msg.caption);
          if (msg.kind === "image" && msg.hd) fd.set("hd", "true");
          if (msg.kind === "audio" && msg.waveform) fd.set("waveform", JSON.stringify(msg.waveform));
          fd.set("file", blob, msg.kind === "document" ? msg.fileName : msg.kind === "audio" ? "audio" : "imagem");
          return post<Raw>(path, fd);
        }
        default:
          throw new Error("Tipo de mensagem não suportado.");
      }
    },
    onMutate: ({ msg }) => {
      qc.setQueryData<Page>(key, (old) => {
        if (!old) return old;
        return old.messages.some((m) => m.id === msg.id)
          ? { ...old, messages: old.messages.map((m) => (m.id === msg.id ? ({ ...m, status: "pending" } as InternalMessage) : m)) }
          : { ...old, messages: [...old.messages, msg] };
      });
    },
    onSuccess: (raw) => {
      pendingFiles.delete(raw.id);
      qc.setQueryData<Page>(key, (old) => upsertMessage(old, fromServer(raw, userId)));
    },
    onError: (err, { msg }) => {
      if (isRejection(err) && msg.status !== "failed") {
        pendingFiles.delete(msg.id);
        replace(msg.id, () => null); // o servidor recusou: a mensagem nunca existiu
      } else {
        replace(msg.id, (m) => ({ ...m, status: "failed" }) as InternalMessage);
      }
    },
  });
}

export function useEditInternal() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useMutation({
    mutationFn: ({ id, text, mentions }: { id: string; text: string; mentions?: MentionRef[] }) => patch<Raw>(`/internal-chat/messages/${id}`, { text, mentions: mentions ?? [] }),
    onSuccess: (raw) => qc.setQueryData<Page>(messagesKey(userId), (old) => upsertMessage(old, fromServer(raw, userId))),
  });
}

export function useDeleteInternal() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useMutation({
    mutationFn: (id: string) => del<void>(`/internal-chat/messages/${id}`),
    onSuccess: (_r, id) => {
      qc.setQueryData<Page>(messagesKey(userId), (old) =>
        old && {
          ...old,
          messages: old.messages.map((m) => (m.id === id ? ({ id: m.id, conversationId: m.conversationId, seq: m.seq, kind: m.kind, sentAt: m.sentAt, direction: m.direction, deleted: true, text: "", authorId: m.authorId, authorName: m.authorName, authorRole: m.authorRole, group: true, noReceipts: true } as unknown as InternalMessage) : m)),
        },
      );
      void qc.invalidateQueries({ queryKey: roomKey(userId) });
    },
  });
}

export function useMarkInternalRead() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const userId = me?.user.id ?? "";
  return useMutation({
    mutationFn: () => api<InternalRoom>("POST", "/internal-chat/read", {}),
    onSuccess: (room) => qc.setQueryData<InternalRoom>(roomKey(userId), room),
  });
}

/** Eventos do servidor (ver useRealtime). */
export function applyInternalEvent(qc: QueryClient, ev: { type: string; data: unknown }) {
  const meId = qc.getQueryData<Me | null>(meKey)?.user.id;
  if (!meId) return;
  switch (ev.type) {
    case "internal.message.created": {
      const m = fromServer(ev.data as Raw, meId);
      qc.setQueryData<Page>(messagesKey(meId), (old) => upsertMessage(old, m));
      // Mensagem de outra pessoa: sobe o selo. Quem está com o chat aberto e no fim da conversa a marca como lida em seguida.
      if (m.authorId !== meId) {
        const mentionsMe = !!m.mentions?.some((x) => x.id === meId);
        qc.setQueryData<InternalRoom>(roomKey(meId), (old) => old && { ...old, unread: old.unread + 1, mentions: old.mentions + (mentionsMe ? 1 : 0), lastSeq: Math.max(old.lastSeq, m.seq ?? 0) });
        // Som: só quando me mencionam (padrão), a cada mensagem ou nunca, conforme a preferência.
        const mode = getInternalChatSound();
        if (mode === "all" || (mode === "mentions" && mentionsMe)) void playIncomingSound({ id: m.id, conversationId: INTERNAL_SOUND_CONVERSATION, direction: "in", sentAt: m.sentAt });
      }
      break;
    }
    case "internal.message.updated": {
      const m = fromServer(ev.data as Raw, meId);
      qc.setQueryData<Page>(messagesKey(meId), (old) => upsertMessage(old, m));
      // Apagar, ou editar para citar (ou deixar de citar) alguém, muda a conta de não lidas e de menções de quem não é a
      // autora, e o evento não diz quem deixou de ser citado: o servidor refaz a conta. São eventos raros.
      void qc.invalidateQueries({ queryKey: roomKey(meId) });
      break;
    }
    case "internal.read": {
      const d = ev.data as { unread: number; mentions?: number; lastReadSeq: number };
      qc.setQueryData<InternalRoom>(roomKey(meId), (old) => old && { ...old, unread: d.unread, mentions: d.mentions ?? 0, lastReadSeq: d.lastReadSeq });
      break;
    }
  }
}
