import { useState } from "react";
import { StickyNote } from "lucide-react";
import { Button, Modal } from "@/components/ui";
import { cn } from "@/lib/cn";
import type { Person } from "@/features/team/api";
import { reminderColors, type Reminder } from "./reminders";

export function TransferReminderDialog({ reminder, person, onConfirm, onCancel }: {
  reminder: Reminder;
  person: Person;
  onConfirm: (share: boolean, remember: boolean) => void;
  onCancel: () => void;
}) {
  const [remember, setRemember] = useState(false);
  return <Modal open title="Levar seu lembrete junto?" onClose={onCancel}>
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted">Você tem um lembrete nesta conversa. Se for junto, <span className="font-medium text-fg">{person.name}</span> poderá ler e editar este texto:</p>
      <div className={cn("flex items-start gap-2 rounded-lg px-3 py-2 text-xs", reminderColors[reminder.color].style)}>
        <StickyNote className="mt-0.5 size-3.5 shrink-0" /><p className="min-w-0 whitespace-pre-wrap break-words">{reminder.text}</p>
      </div>
      <label className="flex cursor-pointer items-start gap-2 text-sm">
        <input type="checkbox" className="mt-0.5 size-4 shrink-0" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
        <span>Sempre levar meu lembrete e não perguntar de novo<span className="block text-xs text-muted">Vale para “Transferir com o lembrete”. Dá para mudar em Configurações.</span></span>
      </label>
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Button variant="ghost" onClick={onCancel}>Cancelar</Button>
        <Button variant="secondary" onClick={() => onConfirm(false, false)}>Transferir sem o lembrete</Button>
        <Button onClick={() => onConfirm(true, remember)}>Transferir com o lembrete</Button>
      </div>
    </div>
  </Modal>;
}
