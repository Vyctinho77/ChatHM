import { useEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { meKey } from "@/features/auth/api";
import { conversationKeys } from "@/features/conversations/store.api";
import type { Conversation, Message } from "@/features/conversations/types";
import { connectionKey, type Connection, type HistoryProgress } from "@/features/whatsapp/api";
import { reminderKey } from "@/features/conversations/reminders";

type ServerEvent =
  | { type: "conversation.updated"; data: Conversation }
  | { type: "conversation.removed"; data: { id: string } }
  | { type: "conversation.reminder.updated"; data: { id: string } }
  | { type: "message.created"; data: Message }
  | { type: "message.updated"; data: Message }
  | { type: "whatsapp.status"; data: Omit<Connection, "qr" | "simulator"> }
  | { type: "whatsapp.qr"; data: Connection["qr"] };
// History import progresses independently from the live connection.
type HistoryEvent = { type: "whatsapp.sync"; data: HistoryProgress } | { type: "contact.avatar"; data: { contactId: string } };

const sortByLast = (a: Conversation, b: Conversation) =>
  (b.lastMessage?.sentAt ?? "").localeCompare(a.lastMessage?.sentAt ?? "");

export function applyRealtimeEvent(qc: QueryClient, ev: ServerEvent | HistoryEvent) {
  switch (ev.type) {
    case "conversation.updated": {
      const c = ev.data;
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
      // Perdeu acesso (ex.: outra pessoa assumiu uma conversa da fila geral).
      qc.setQueryData<Conversation[]>(conversationKeys.list, (old) => old?.filter((c) => c.id !== ev.data.id));
      qc.removeQueries({ queryKey: conversationKeys.messages(ev.data.id) });
      void qc.invalidateQueries({ queryKey: reminderKey });
      break;
    }
    case "conversation.reminder.updated":
      void qc.invalidateQueries({ queryKey: reminderKey });
      break;
    case "message.created":
    case "message.updated": {
      const m = ev.data;
      // O id da mensagem enviada é do cliente: se já existe (bolha otimista), atualiza em vez de duplicar.
      qc.setQueryData<Message[]>(conversationKeys.messages(m.conversationId), (old) => {
        if (!old) return old;
        return old.some((x) => x.id === m.id) ? old.map((x) => (x.id === m.id ? m : x)) : [...old, m];
      });
      break;
    }
    case "whatsapp.status":
      qc.setQueryData<Connection>(connectionKey, (old) => ({
        ...old,
        ...ev.data,
        qr: ev.data.state === "pending_qr" ? old?.qr : null,
      }));
      break;
    case "whatsapp.qr":
      qc.setQueryData<Connection>(connectionKey, (old) => (old && old.state === "pending_qr" && !old.pairingComplete ? { ...old, qr: ev.data } : old));
      break;
    case "contact.avatar":
      // Nova foto de perfil: recarrega listas que mostram avatares.
      void qc.invalidateQueries({ queryKey: ["contacts"] });
      void qc.invalidateQueries({ queryKey: conversationKeys.list });
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
      stopped = true;
      document.removeEventListener("visibilitychange", onVisible);
      stopPolling();
      clearTimeout(timer);
      ws?.close();
    };
  }, [enabled, qc]);
}
