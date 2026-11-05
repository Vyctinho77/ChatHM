import { useEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { meKey } from "@/features/auth/api";
import { conversationKeys } from "@/features/conversations/store.api";
import type { Conversation, Message } from "@/features/conversations/types";
import { connectionKey, type Connection } from "@/features/whatsapp/api";

type ServerEvent =
  | { type: "conversation.updated"; data: Conversation }
  | { type: "conversation.removed"; data: { id: string } }
  | { type: "message.created"; data: Message }
  | { type: "message.updated"; data: Message }
  | { type: "whatsapp.status"; data: Omit<Connection, "qr" | "simulator"> }
  | { type: "whatsapp.qr"; data: Connection["qr"] };

const sortByLast = (a: Conversation, b: Conversation) =>
  (b.lastMessage?.sentAt ?? "").localeCompare(a.lastMessage?.sentAt ?? "");

function apply(qc: QueryClient, ev: ServerEvent) {
  switch (ev.type) {
    case "conversation.updated": {
      const c = ev.data;
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
      break;
    }
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
      qc.setQueryData<Connection>(connectionKey, (old) => (old ? { ...old, qr: ev.data } : old));
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

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(`${proto}://${location.host}/api/v1/ws`);
      ws.onopen = () => {
        // Ao (re)conectar, ressincroniza o que pode ter sido perdido.
        if (attempt > 0) {
          void qc.invalidateQueries({ queryKey: conversationKeys.list });
          void qc.invalidateQueries({ queryKey: connectionKey });
        }
        attempt = 0;
      };
      ws.onmessage = (e) => {
        try {
          apply(qc, JSON.parse(e.data as string) as ServerEvent);
        } catch {
          /* evento malformado: ignora */
        }
      };
      ws.onclose = (e) => {
        if (stopped) return;
        if (e.code === 4401) {
          void qc.invalidateQueries({ queryKey: meKey }); // sessão encerrada → volta ao login
          return;
        }
        const delay = Math.min(30_000, 1_000 * 2 ** attempt++);
        timer = setTimeout(connect, delay);
      };
    };

    connect();
    return () => {
      stopped = true;
      clearTimeout(timer);
      ws?.close();
    };
  }, [enabled, qc]);
}
