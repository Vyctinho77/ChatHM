import { useEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { meKey } from "@/features/auth/api";
import { conversationKeys } from "@/features/conversations/store.api";
import type { Conversation, Message } from "@/features/conversations/types";
import { connectionKey, type Connection, type HistoryProgress } from "@/features/whatsapp/api";
import { reminderKey } from "@/features/conversations/reminders";
import { applyInternalEvent } from "@/features/internal/api";
import { pushNotice } from "@/lib/notices";
import { applyTyping, clearTyping, type TypingUpdate } from "@/features/conversations/typing";
import { playIncomingSound } from "@/lib/sounds";

type ServerEvent =
  | { type: "conversation.updated"; data: Conversation }
  | { type: "conversation.removed"; data: { id: string } }
  | { type: "conversation.reminder.updated"; data: { id: string } }
  | { type: "conversation.notice"; data: { id: string; kind: "closed_for_all" | "handed_over"; by?: string; contact?: string } }
  | { type: "conversation.typing"; data: TypingUpdate }
  | { type: "message.created"; data: Message }
  | { type: "message.updated"; data: Message }
  | { type: "whatsapp.status"; data: Omit<Connection, "qr" | "simulator"> }
  | { type: "whatsapp.qr"; data: Connection["qr"] };
// History import progresses independently from the live connection.
type HistoryEvent = { type: "whatsapp.sync"; data: HistoryProgress } | { type: "contact.avatar"; data: { contactId: string } }
  | { type: "ai.suggestion"; data: { conversationId: string; id: string } }
  | { type: "internal.message.created" | "internal.message.updated" | "internal.read"; data: unknown };

const sortByLast = (a: Conversation, b: Conversation) =>
  (b.lastMessage?.sentAt ?? "").localeCompare(a.lastMessage?.sentAt ?? "");

export function applyRealtimeEvent(qc: QueryClient, ev: ServerEvent | HistoryEvent) {
  switch (ev.type) {
    case "conversation.updated": {
      const c = ev.data;
      if (c.status === "closed") clearTyping(c.id);
      const previous = qc.getQueryData<Conversation[]>(conversationKeys.list)?.find((x) => x.id === c.id);
      if (previous?.assignee?.id !== c.assignee?.id) void qc.invalidateQueries({ queryKey: reminderKey });
      qc.setQueryData<Conversation[]>(conversationKeys.list, (old) => {
        if (!old) return old;
        const rest = old.filter((x) => x.id !== c.id);
        return [c, ...rest].sort(sortByLast);
      });
      break;
    }
    case "conversation.removed": {
      clearTyping(ev.data.id);
      // Perdeu acesso (ex.: outra pessoa assumiu uma conversa da fila geral).
      qc.setQueryData<Conversation[]>(conversationKeys.list, (old) => old?.filter((c) => c.id !== ev.data.id));
      qc.removeQueries({ queryKey: conversationKeys.messages(ev.data.id) });
      void qc.invalidateQueries({ queryKey: reminderKey });
      break;
    }
    case "conversation.reminder.updated":
      void qc.invalidateQueries({ queryKey: reminderKey });
      break;
    case "conversation.notice": {
      // Outra pessoa mudou o atendimento em que estou: aviso curto na tela.
      const { id, kind, by, contact } = ev.data;
      const who = by || "Alguém";
      const of = contact ? ` de ${contact}` : "";
      if (kind === "handed_over") pushNotice({ id: `notice:${kind}:${id}`, text: `${who} passou o atendimento${of} para você.`, link: { to: `/conversas/${id}`, label: "Abrir" } });
      else pushNotice({ id: `notice:${kind}:${id}`, text: `${who} encerrou o atendimento${of} para todos.` });
      break;
    }
    case "conversation.typing":
      // Never retain typing state for conversations outside the current visible list.
      if (qc.getQueryData<Conversation[]>(conversationKeys.list)?.some((c) => c.id === ev.data.conversationId && c.status !== "closed")) applyTyping(ev.data);
      break;
    case "message.created":
    case "message.updated": {
      const m = ev.data;
      if (ev.type === "message.created") void playIncomingSound(m);
      // O id da mensagem enviada é do cliente: se já existe (bolha otimista), atualiza em vez de duplicar.
      qc.setQueryData<Message[]>(conversationKeys.messages(m.conversationId), (old) => {
        if (!old) return old;
        return old.some((x) => x.id === m.id) ? old.map((x) => (x.id === m.id ? m : x)) : [...old, m];
      });
      break;
    }
    case "whatsapp.status":
      if (ev.data.state !== "connected") clearTyping();
      qc.setQueryData<Connection>(connectionKey, (old) => ({
        ...old,
        ...ev.data,
        qr: ev.data.state === "pending_qr" ? old?.qr : null,
      }));
      break;
    case "whatsapp.qr":
      qc.setQueryData<Connection>(connectionKey, (old) => (old && old.state === "pending_qr" && !old.pairingComplete ? { ...old, qr: ev.data } : old));
      break;
    case "ai.suggestion":
      // A IA deixou um rascunho/pedido de envio para esta pessoa aprovar.
      void qc.invalidateQueries({ queryKey: ["ai", "suggestions", ev.data.conversationId] });
      break;
    case "contact.avatar":
      // Nova foto de perfil: recarrega listas que mostram avatares.
      void qc.invalidateQueries({ queryKey: ["contacts"] });
      void qc.invalidateQueries({ queryKey: conversationKeys.list });
      break;
    case "internal.message.created":
    case "internal.message.updated":
    case "internal.read":
      applyInternalEvent(qc, ev);
      break;
    case "whatsapp.sync":
      qc.setQueryData<Connection>(connectionKey, (old) => old ? { ...old, sync: ev.data } : old);
      if (ev.data.state !== "syncing") {
        void qc.invalidateQueries({ queryKey: ["contacts"] });
        void qc.invalidateQueries({ queryKey: conversationKeys.list });
      }
      break;
  }
}

/** Mantém conversas e status do WhatsApp atualizados em tempo real (com reconexão). */
export function useRealtime(enabled: boolean) {
  const qc = useQueryClient();
  useEffect(() => {
    if (!enabled) return;
    let ws: WebSocket | null = null;
    let attempt = 0;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    // Rede de segurança: enquanto o socket estiver fora, recarrega o que está na tela.
    let poll: ReturnType<typeof setInterval> | undefined;
    const resync = () => {
      void qc.invalidateQueries({ queryKey: conversationKeys.list, refetchType: "active" });
      void qc.invalidateQueries({ queryKey: reminderKey, refetchType: "active" });
    };
    const startPolling = () => {
      if (!poll) poll = setInterval(resync, 5_000);
    };
    const stopPolling = () => {
      clearInterval(poll);
      poll = undefined;
    };
    // Aba volta a ficar visível: o navegador pode ter suspendido o socket.
    const onVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (!ws || ws.readyState === WebSocket.CLOSED) {
        clearTimeout(timer);
        attempt = Math.max(attempt, 1);
        connect();
      } else {
        resync();
      }
    };

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(`${proto}://${location.host}/api/v1/ws`);
      ws.onopen = () => {
        // Ao (re)conectar, ressincroniza o que pode ter sido perdido.
        if (attempt > 0) {
          void qc.invalidateQueries({ queryKey: conversationKeys.list });
          void qc.invalidateQueries({ queryKey: connectionKey });
          void qc.invalidateQueries({ queryKey: reminderKey });
        }
        attempt = 0;
        stopPolling();
      };
      ws.onmessage = (e) => {
        try {
          applyRealtimeEvent(qc, JSON.parse(e.data as string) as ServerEvent | HistoryEvent);
        } catch {
          /* evento malformado: ignora */
        }
      };
      ws.onclose = (e) => {
        clearTyping();
        if (stopped) return;
        startPolling();
        if (e.code === 4401) {
          stopPolling();
          void qc.invalidateQueries({ queryKey: meKey }); // sessão encerrada → volta ao login
          return;
        }
        const delay = Math.min(10_000, 1_000 * 2 ** attempt++);
        timer = setTimeout(connect, delay);
      };
    };

    connect();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearTyping();
      stopped = true;
      document.removeEventListener("visibilitychange", onVisible);
      stopPolling();
      clearTimeout(timer);
      ws?.close();
    };
  }, [enabled, qc]);
}
