import { useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { endOptions, type EndKind } from "./closeOptions";
import { CloseAllDialog, LeaveDialog } from "./EndDialogs";
import { useReminders } from "./reminders";
import { capabilities } from "./store";
import type { Conversation } from "./types";

type CloseOptions = { scope?: "all" | "me"; passTo?: string; shareReminder?: boolean };
type EndActions = { me: { id: string }; close: (id: string, opts?: CloseOptions) => Promise<void> };

/**
 * Encerrar o atendimento: o que a pessoa pode fazer (`end`), como começar (`startEnd`) e os diálogos de confirmação (`dialog`,
 * para renderizar no painel). Sozinha no atendimento, encerra direto; com mais gente, escolhe entre sair só da própria parte
 * (e, sendo a responsável, quem continua) e encerrar para todos, confirmando quem será afetado.
 */
export function useEndFlow(c: Conversation, actions: EndActions, supervisor: boolean) {
  const end = endOptions(c, { myId: actions.me.id, supervisor });
  const canEnd = capabilities.close && end.actions.length > 0;
  const [flow, setFlow] = useState<EndKind | null>(null);
  const navigate = useNavigate();
  const { search } = useLocation();
  const reminders = useReminders(true);

  function startEnd(kind: EndKind) {
    if (end.direct) void actions.close(c.id).catch(() => undefined); // única pessoa no atendimento: encerra, sem perguntar
    else setFlow(kind);
  }

  async function run(kind: EndKind, o?: { passTo?: string; shareReminder?: boolean }) {
    await actions.close(c.id, { scope: kind === "leave" ? "me" : "all", ...o });
    setFlow(null);
    if (kind === "leave") navigate({ pathname: "/conversas", search }); // saiu: volta ao painel vazio
  }

  const dialog =
    flow === "leave" ? (
      <LeaveDialog conversation={c} myId={actions.me.id} hasReminder={!!reminders.data?.[c.id]} onConfirm={(o) => run("leave", o)} onClose={() => setFlow(null)} />
    ) : flow === "all" ? (
      <CloseAllDialog others={end.others} canLeave={end.actions.includes("leave")} onConfirm={() => run("all")} onClose={() => setFlow(null)} />
    ) : null;

  return { end, canEnd, startEnd, dialog };
}
