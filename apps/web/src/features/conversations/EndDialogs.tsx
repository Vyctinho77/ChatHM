import { useState } from "react";
import { Alert, Button, Modal } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { joinNames } from "./closeOptions";
import { getShareReminderAlways } from "./shareReminderPref";
import type { Conversation } from "./types";

type Person = { id: string; name: string };

/** Roda a ação e mostra o erro dentro do diálogo, sem fechá-lo. */
function useRun(run: () => Promise<void>) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return {
    busy,
    error,
    go: async () => {
      setBusy(true);
      setError("");
      try {
        await run();
      } catch (e) {
        setError(errorMessage(e));
        setBusy(false);
      }
    },
  };
}

/**
 * "Encerrar minha parte": a pessoa sai do atendimento, que continua para quem ficou. O responsável, havendo participantes,
 * escolhe quem continua (a mais antiga vem marcada) e se o lembrete pessoal vai junto.
 */
export function LeaveDialog({ conversation: c, myId, hasReminder, onConfirm, onClose }: {
  conversation: Conversation;
  myId: string;
  hasReminder: boolean;
  onConfirm: (o: { passTo?: string; shareReminder?: boolean }) => Promise<void>;
  onClose: () => void;
}) {
  const handsOver = c.assignee?.id === myId && !!c.participants?.length;
  const candidates: Person[] = c.participants ?? [];
  const [passTo, setPassTo] = useState(candidates[0]?.id ?? "");
  const [share, setShare] = useState(() => getShareReminderAlways(myId));
  const next = candidates.find((p) => p.id === passTo);
  const stays = [...(c.assignee && c.assignee.id !== myId ? [c.assignee] : []), ...candidates.filter((p) => p.id !== myId)];
  const { busy, error, go } = useRun(() => onConfirm(handsOver ? { passTo, shareReminder: hasReminder && share } : {}));

  return (
    <Modal open onClose={() => !busy && onClose()} title="Encerrar minha parte">
      {handsOver ? (
        <>
          <p className="text-sm text-muted">Você sai deste atendimento, que continua com quem ficar. Quem continua como responsável?</p>
          <div role="radiogroup" aria-label="Quem continua o atendimento" className="flex flex-col gap-1">
            {candidates.map((p) => (
              <label key={p.id} className={cn("flex cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2 text-sm", passTo === p.id ? "border-brand-ink bg-selected" : "border-line hover:bg-hover")}>
                <input type="radio" name="passTo" className="size-4" checked={passTo === p.id} onChange={() => setPassTo(p.id)} />
                {p.name}
              </label>
            ))}
          </div>
          {hasReminder && (
            <label className="flex cursor-pointer items-start gap-2 text-sm">
              <input type="checkbox" className="mt-0.5 size-4 shrink-0" checked={share} onChange={(e) => setShare(e.target.checked)} />
              <span>Levar o meu lembrete para {next?.name ?? "quem continua"}<span className="block text-xs text-muted">Ela poderá ler e editar o texto.</span></span>
            </label>
          )}
        </>
      ) : (
        <p className="text-sm text-muted">
          {stays.length
            ? <>Você sai deste atendimento. Ele continua com <span className="font-medium text-fg">{joinNames(stays.map((p) => p.name))}</span>.</>
            : <>Você sai deste atendimento, que volta à fila.</>}
        </p>
      )}
      {error && <Alert>{error}</Alert>}
      <div className="flex flex-col-reverse items-stretch gap-2 sm:flex-row sm:justify-end">
        <Button variant="secondary" disabled={busy} onClick={onClose}>Cancelar</Button>
        <Button loading={busy} disabled={handsOver && !passTo} onClick={() => void go()}>Encerrar minha parte</Button>
      </div>
    </Modal>
  );
}

/** "Encerrar para todos" quando há outras pessoas no atendimento: confirma, dizendo quem será afetado. */
export function CloseAllDialog({ others, canLeave, onConfirm, onClose }: {
  others: Person[];
  /** A pessoa também está no atendimento: lembra que dá para sair só da própria parte. */
  canLeave: boolean;
  onConfirm: () => Promise<void>;
  onClose: () => void;
}) {
  const { busy, error, go } = useRun(onConfirm);
  const names = joinNames(others.map((p) => p.name));
  return (
    <Modal open onClose={() => !busy && onClose()} title="Encerrar para todos?">
      <p className="text-sm text-muted">
        <span className="font-medium text-fg">{names}</span> {others.length === 1 ? "está" : "estão"} neste atendimento. Encerrando para todos, a conversa sai da lista {others.length === 1 ? "dela" : "delas"} e o cliente que escrever depois volta para a fila.
        {canLeave && <> Se só a sua parte terminou, use “Encerrar minha parte”.</>}
      </p>
      {error && <Alert>{error}</Alert>}
      <div className="flex flex-col-reverse items-stretch gap-2 sm:flex-row sm:justify-end">
        <Button variant="secondary" disabled={busy} onClick={onClose}>Cancelar</Button>
        <Button variant="danger" loading={busy} onClick={() => void go()}>Encerrar para todos</Button>
      </div>
    </Modal>
  );
}
