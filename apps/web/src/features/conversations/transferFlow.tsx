import { useState } from "react";
import type { Person } from "@/features/team/api";
import { useReminders, type Reminder } from "./reminders";
import { getShareReminderAlways, setShareReminderAlways } from "./shareReminderPref";
import { TransferReminderDialog } from "./TransferReminderDialog";
import type { Conversation } from "./types";

type TransferActions = {
  me: { id: string };
  transfer: (id: string, person: { id: string; name: string }, shareReminder?: boolean) => void;
};

/**
 * O que fazer ao escolher a pessoa: transferir direto (sem lembrete, ou lembrete que não é seu),
 * levar o lembrete sem perguntar (preferência salva) ou pedir o consentimento antes.
 * Só o lembrete de quem está atendendo pode ser compartilhado; se não deu para carregar, nada é compartilhado.
 */
export function decideTransfer(opts: { myId: string; conversation: Conversation; reminder?: Reminder; always: boolean }): "direct" | "share" | "ask" {
  if (!opts.reminder || opts.conversation.assignee?.id !== opts.myId) return "direct";
  return opts.always ? "share" : "ask";
}

/** Use `request` no lugar de `actions.transfer` e renderize `dialog` no componente. */
export function useTransferFlow(actions: TransferActions) {
  const reminders = useReminders(true);
  const [pending, setPending] = useState<{ conversation: Conversation; person: Person; reminder: Reminder } | null>(null);

  function request(conversation: Conversation, person: Person) {
    const reminder = reminders.data?.[conversation.id];
    const verdict = decideTransfer({ myId: actions.me.id, conversation, reminder, always: getShareReminderAlways(actions.me.id) });
    if (verdict === "ask" && reminder) setPending({ conversation, person, reminder });
    else if (verdict === "share") actions.transfer(conversation.id, person, true);
    else actions.transfer(conversation.id, person);
  }

  const dialog = pending && <TransferReminderDialog
    reminder={pending.reminder}
    person={pending.person}
    onCancel={() => setPending(null)}
    onConfirm={(share, remember) => {
      if (share && remember) setShareReminderAlways(actions.me.id, true);
      actions.transfer(pending.conversation.id, pending.person, share);
      setPending(null);
    }}
  />;
  return { request, dialog };
}
