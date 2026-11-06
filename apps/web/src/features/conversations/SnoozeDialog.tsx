import { useEffect, useState, type FormEvent } from "react";
import { Clock3 } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { useConversationActions } from "./store";
import type { Conversation } from "./types";

function localInput(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}T${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

export function SnoozeDialog({ conversation, onClose }: { conversation: Conversation; onClose: () => void }) {
  const actions = useConversationActions();
  const [value, setValue] = useState(() => localInput(new Date(conversation.snoozedUntil ?? Date.now() + 60 * 60_000)));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 15_000);
    return () => clearInterval(timer);
  }, []);
  const until = new Date(value);
  const valid = Number.isFinite(until.getTime()) && until.getTime() > now;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!valid || busy) return;
    setBusy(true);
    setError("");
    try {
      await actions.snooze(conversation.id, until.toISOString());
      onClose();
    } catch (err) {
      setError(fieldErrors(err).until ?? errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return <Modal open onClose={() => { if (!busy) onClose(); }} title="Adiar atendimento">
    <form onSubmit={submit} className="flex flex-col gap-5">
      <p className="text-sm text-muted">Quando deseja retomar a conversa com <span className="font-medium text-fg">{conversation.contact.name}</span>?</p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="secondary" disabled={busy} onClick={() => setValue(localInput(new Date(Date.now() + 60 * 60_000)))}>Em 1 hora</Button>
        <Button type="button" variant="secondary" disabled={busy} onClick={() => {
          const tomorrow = new Date();
          tomorrow.setDate(tomorrow.getDate() + 1);
          tomorrow.setHours(9, 0, 0, 0);
          setValue(localInput(tomorrow));
        }}>Amanhã às 9h</Button>
      </div>
      <Field label="Escolher data e hora" type="datetime-local" required disabled={busy} min={localInput(new Date(now))} value={value} onChange={(e) => setValue(e.target.value)} />
      {valid && <p className="flex items-start gap-2 text-sm text-muted"><Clock3 className="mt-0.5 size-4 shrink-0" />Retorna em {until.toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" })} ({Intl.DateTimeFormat().resolvedOptions().timeZone}).</p>}
      <p className="text-xs text-muted">Se o cliente enviar uma nova mensagem, o atendimento retorna antes desse horário.</p>
      {error && <Alert>{error}</Alert>}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" disabled={busy} onClick={onClose}>Cancelar</Button>
        <Button type="submit" loading={busy} disabled={!valid}>Adiar atendimento</Button>
      </div>
    </form>
  </Modal>;
}

export function SnoozeNotice({ conversation: c, canManage, onEdit }: { conversation: Conversation; canManage: boolean; onEdit: () => void }) {
  const actions = useConversationActions();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <div className="border-b border-line bg-panel px-4 py-3">
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs">
      <p className="flex flex-1 items-center gap-2 text-muted"><Clock3 className="size-4 shrink-0" />Adiado até {new Date(c.snoozedUntil!).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" })}</p>
      {canManage && <>
        <Button variant="ghost" disabled={busy} className="h-7 px-2 text-xs" onClick={onEdit}>Alterar horário</Button>
        <Button variant="secondary" loading={busy} className="h-7 px-2 text-xs" onClick={async () => {
          setBusy(true);
          setError("");
          try { await actions.resume(c.id); }
          catch (err) { setError(errorMessage(err)); }
          finally { setBusy(false); }
        }}>Retomar agora</Button>
      </>}
    </div>
    {error && <p role="alert" className="mt-2 text-xs text-danger">{error}</p>}
  </div>;
}
