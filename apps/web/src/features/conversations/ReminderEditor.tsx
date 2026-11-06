import { useState, type FormEvent } from "react";
import { Check, StickyNote } from "lucide-react";
import { Alert, Button, Modal } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { cn } from "@/lib/cn";
import { reminderColors, useSaveReminder, type Reminder, type ReminderColor } from "./reminders";

export function ReminderEditor({ conversationId, contactName, reminder, onClose }: {
  conversationId: string; contactName: string; reminder?: Reminder; onClose: () => void;
}) {
  const [text, setText] = useState(reminder?.text ?? "");
  const [color, setColor] = useState<ReminderColor>(reminder?.color ?? "olive");
  const save = useSaveReminder();

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!text.trim() || save.isPending) return;
    try { await save.mutateAsync({ conversationId, text, color }); onClose(); }
    catch { /* Render the mutation error without losing the draft. */ }
  }

  return <Modal open title="Lembrete pessoal" onClose={() => { if (!save.isPending) onClose(); }}>
    <form onSubmit={submit} className="flex flex-col gap-5">
      <p className="text-sm text-muted">Uma anotação para lembrar do atendimento de <span className="font-medium text-fg">{contactName}</span>. Só você pode vê-la.</p>
      <label className="flex flex-col gap-2 text-sm font-medium">
        Lembrete
        <textarea autoFocus required rows={3} maxLength={240} value={text} disabled={save.isPending} onChange={(e) => setText(e.target.value)} placeholder="Ex.: Aguardando os documentos para emitir a nota." className="w-full resize-y rounded-xl border border-line bg-elevated px-3 py-2 text-sm font-normal leading-relaxed outline-none placeholder:text-muted focus:border-brand-ink" />
        <span className="text-right text-[11px] font-normal text-muted">{text.length}/240</span>
      </label>
      <fieldset disabled={save.isPending}>
        <legend className="mb-3 text-sm font-medium">Cor do lembrete</legend>
        <div className="flex flex-wrap gap-2">
          {(Object.entries(reminderColors) as [ReminderColor, typeof reminderColors[ReminderColor]][]).map(([key, option]) => <button key={key} type="button" aria-label={option.name} aria-pressed={color === key} title={option.name} onClick={() => setColor(key)} className={cn("flex size-9 items-center justify-center rounded-full border-2 transition-colors disabled:opacity-60", color === key ? "border-fg" : "border-transparent hover:border-muted")}>
            <span className={cn("flex size-6 items-center justify-center rounded-full", option.dot)}>{color === key && <Check className="size-4 text-black" />}</span>
          </button>)}
        </div>
      </fieldset>
      {text.trim() && <div className={cn("flex items-start gap-2 rounded-lg px-3 py-2 text-xs", reminderColors[color].style)}>
        <StickyNote className="mt-0.5 size-3.5 shrink-0" /><p className="min-w-0 whitespace-pre-wrap break-words">{text}</p>
      </div>}
      {save.error && <Alert>{fieldErrors(save.error).text ?? fieldErrors(save.error).color ?? errorMessage(save.error)}</Alert>}
      <div className="flex flex-wrap items-center gap-2">
        {reminder && <Button type="button" variant="ghost" className="mr-auto text-danger" disabled={save.isPending} onClick={async () => {
          try { await save.mutateAsync({ conversationId, text: "", color }); onClose(); }
          catch { /* Keep the editor open on errors. */ }
        }}>Remover</Button>}
        <Button type="button" variant="ghost" className={cn(!reminder && "ml-auto")} disabled={save.isPending} onClick={onClose}>Cancelar</Button>
        <Button type="submit" loading={save.isPending} disabled={!text.trim()}>Salvar lembrete</Button>
      </div>
    </form>
  </Modal>;
}
