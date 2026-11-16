import { useState } from "react";
import { useNavigate } from "react-router";
import { Phone, Search } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Highlight } from "@/components/Highlight";
import { Alert, Modal } from "@/components/ui";
import { errorMessage, fieldErrors } from "@/lib/api";
import { digitsOf, matches } from "@/lib/search";
import { useContacts } from "@/features/contacts/api";
import { useContactOpener } from "./store";

/** Nova conversa: escolha um cliente da lista ou digite um telefone (com DDD). */
export function NewConversationModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const opener = useContactOpener();
  // A lista inteira de contatos só é baixada quando a janela abre (ela fica montada o tempo todo na lista de conversas).
  const { contacts } = useContacts({ enabled: open });
  const [term, setTerm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});

  const q = term.trim();
  const found = q ? contacts.filter((c) => matches(`${c.name} ${c.phone} ${c.company ?? ""}`, q)).slice(0, 8) : contacts.slice(0, 8);
  const looksLikePhone = digitsOf(q).length >= 10 && /^[\d\s()+.-]+$/.test(q);

  function close() {
    setTerm("");
    setError("");
    setFields({});
    onClose();
  }

  async function go(run: () => Promise<{ id: string }>) {
    setBusy(true);
    setError("");
    setFields({});
    try {
      const conv = await run();
      close();
      navigate(`/conversas/${conv.id}`);
    } catch (e) {
      setError(errorMessage(e));
      setFields(fieldErrors(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open={open} onClose={close} title="Nova conversa">
      <div className="flex flex-col gap-3">
        {error && <Alert>{fields.phone ?? error}</Alert>}
        <label className="flex items-center gap-2 rounded-lg border border-line bg-elevated px-3 py-2 text-sm focus-within:border-muted">
          <Search className="size-4 shrink-0 text-muted" />
          <input
            autoFocus
            aria-label="Buscar cliente ou digitar telefone"
            placeholder="Nome, empresa ou telefone com DDD"
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted"
          />
        </label>
        <ul className="-mx-1 max-h-72 overflow-y-auto" aria-busy={busy}>
          {looksLikePhone && (
            <li>
              <button
                type="button"
                disabled={busy}
                onClick={() => go(() => opener.openPhone(q))}
                className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-hover disabled:opacity-60"
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent/15 text-brand-ink">
                  <Phone className="size-4" />
                </span>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">Conversar com {q}</span>
                  <span className="block text-xs text-muted">Número novo ou fora da lista</span>
                </span>
              </button>
            </li>
          )}
          {found.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                disabled={busy}
                onClick={() => go(() => (c.conversationId ? Promise.resolve({ id: c.conversationId }) : opener.openContact(c.id)))}
                className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-hover disabled:opacity-60"
              >
                <Avatar name={c.name} src={c.avatarUrl} group={c.kind === "group"} size="list" />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    <Highlight text={c.name} term={q} />
                  </span>
                  <span className="block truncate text-xs text-muted">
                    <Highlight text={[c.phone, c.company].filter(Boolean).join(" · ") || "Telefone não informado"} term={q} />
                  </span>
                </span>
              </button>
            </li>
          ))}
          {!looksLikePhone && found.length === 0 && (
            <li className="px-2 py-6 text-center text-sm text-muted">
              Nenhum cliente encontrado. Para um número novo, digite o telefone com DDD.
            </li>
          )}
        </ul>
      </div>
    </Modal>
  );
}
