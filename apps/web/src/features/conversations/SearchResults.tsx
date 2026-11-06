import { MessageSquarePlus, UserRound } from "lucide-react";
import { Highlight } from "@/components/Highlight";
import { Spinner } from "@/components/ui";
import { formatListTime } from "@/lib/format";
import type { ContactRecord, MessageHit } from "./types";

/** Resultados do servidor abaixo da lista de conversas: clientes e mensagens em todo o histórico visível. */
export function SearchResults({
  term,
  loading,
  contacts,
  messages,
  onOpenMessage,
  onOpenContact,
}: {
  term: string;
  loading: boolean;
  contacts: ContactRecord[];
  messages: MessageHit[];
  onOpenMessage: (h: MessageHit) => void;
  onOpenContact: (c: ContactRecord) => void;
}) {
  if (!loading && contacts.length === 0 && messages.length === 0) return null;
  return (
    <div className="mt-2 border-t border-line pt-2">
      {contacts.length > 0 && (
        <section aria-label="Clientes encontrados">
          <h2 className="px-3 pb-1 pt-1 text-[11.5px] font-medium text-brand">Clientes</h2>
          <ul>
            {contacts.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => onOpenContact(c)}
                  className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left hover:bg-hover"
                >
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-hover text-muted">
                    <UserRound className="size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13.5px] font-medium">
                      <Highlight text={c.name} term={term} />
                    </span>
                    <span className="block truncate text-xs text-muted">
                      <Highlight text={[c.phone, c.company].filter(Boolean).join(" · ")} term={term} />
                    </span>
                  </span>
                  {!c.conversationId && <MessageSquarePlus className="size-4 shrink-0 text-muted" aria-label="Iniciar conversa" />}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section aria-label="Mensagens encontradas">
        <h2 className="flex items-center gap-2 px-3 pb-1 pt-2 text-[11.5px] font-medium text-brand">
          Mensagens {loading && <Spinner className="size-3" />}
        </h2>
        <ul>
          {messages.map((h) => (
            <li key={h.id}>
              <button
                type="button"
                onClick={() => onOpenMessage(h)}
                className="flex w-full flex-col gap-0.5 rounded-lg px-2.5 py-2 text-left hover:bg-hover"
              >
                <span className="flex items-baseline gap-2">
                  <span className="truncate text-[13.5px] font-medium">{h.contactName}</span>
                  <span className="ml-auto shrink-0 text-[11px] tabular-nums text-muted">{formatListTime(h.sentAt)}</span>
                </span>
                <span className="line-clamp-2 break-words text-[12.5px] text-muted">
                  {h.direction === "out" && <span className="text-fg/70">Você: </span>}
                  <Highlight text={h.snippet} term={term} />
                </span>
                {h.closed && <span className="text-[10.5px] text-muted">Atendimento encerrado</span>}
              </button>
            </li>
          ))}
          {!loading && messages.length === 0 && <li className="px-3 py-2 text-xs text-muted">Nenhuma mensagem com “{term}”.</li>}
        </ul>
      </section>
    </div>
  );
}
