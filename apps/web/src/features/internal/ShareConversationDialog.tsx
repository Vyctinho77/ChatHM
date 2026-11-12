import { useMemo, useState } from "react";
import { Avatar } from "@/components/Avatar";
import { Button, Modal, Spinner } from "@/components/ui";
import { cn } from "@/lib/cn";
import { matches } from "@/lib/search";
import { useConversations } from "@/features/conversations/store";
import { segmentLabel, type Conversation } from "@/features/conversations/types";

const MAX_SHOWN = 40;
const title = (c: Conversation) => (c.contact.kind === "group" || c.contact.saved ? c.contact.name : c.contact.phone || c.contact.name);
const statusText = (c: Conversation) => (c.status === "closed" ? "Encerrada" : c.assignee ? `Com ${c.assignee.name}` : "Aguardando atendimento");

/** Escolhe uma das conversas que a pessoa já vê e a compartilha no chat interno, com um comentário opcional. */
export function ShareConversationDialog({ onShare, onClose }: { onShare: (conversationId: string, comment: string) => void; onClose: () => void }) {
  const { data, isPending } = useConversations();
  const [term, setTerm] = useState("");
  const [chosen, setChosen] = useState<string | null>(null);
  const [comment, setComment] = useState("");
  const shown = useMemo(() => (data ?? []).filter((c) => matches(`${c.contact.name} ${c.contact.phone}`, term)).slice(0, MAX_SHOWN), [data, term]);

  return (
    <Modal open onClose={onClose} title="Compartilhar conversa de cliente">
      <p className="text-sm text-muted">O cartão mostra o cliente só para quem já tem acesso a essa conversa; para as demais pessoas, ele aparece sem identificação.</p>
      <input
        autoFocus
        type="search"
        value={term}
        onChange={(e) => setTerm(e.target.value)}
        placeholder="Buscar por nome ou telefone"
        aria-label="Buscar conversa"
        className="h-9 w-full rounded-lg border border-line bg-elevated px-3 text-sm outline-none placeholder:text-muted focus:border-brand-ink"
      />
      {isPending && <Spinner className="mx-auto my-2" />}
      <ul role="listbox" aria-label="Conversas" className="-mx-1 flex max-h-56 flex-col overflow-y-auto">
        {shown.map((c) => (
          <li key={c.id} role="option" aria-selected={chosen === c.id}>
            <button
              type="button"
              onClick={() => setChosen(c.id)}
              className={cn("flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left hover:bg-hover", chosen === c.id && "bg-selected")}
            >
              <Avatar name={c.contact.name} src={c.contact.avatarUrl} group={c.contact.kind === "group"} size="sm" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13px] font-medium">{title(c)}</span>
                <span className="block truncate text-[11.5px] text-muted">{title(c) !== c.contact.name && c.contact.name ? `${c.contact.name} · ` : ""}{statusText(c)} · {segmentLabel(c)}</span>
              </span>
            </button>
          </li>
        ))}
        {!isPending && shown.length === 0 && <li className="px-2 py-3 text-center text-sm text-muted">Nenhuma conversa encontrada.</li>}
      </ul>
      <label className="flex flex-col gap-1.5 text-sm font-medium">
        Comentário (opcional)
        <textarea
          rows={2}
          maxLength={500}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          placeholder="Ex.: o cliente pede segunda via da guia. Alguém já viu algo parecido?"
          className="w-full resize-y rounded-xl border border-line bg-elevated px-3 py-2 text-sm font-normal outline-none placeholder:text-muted focus:border-brand-ink"
        />
      </label>
      <div className="flex flex-col-reverse items-stretch gap-2 sm:flex-row sm:justify-end">
        <Button variant="secondary" onClick={onClose}>Cancelar</Button>
        <Button disabled={!chosen} onClick={() => chosen && (onShare(chosen, comment), onClose())}>Compartilhar</Button>
      </div>
    </Modal>
  );
}
