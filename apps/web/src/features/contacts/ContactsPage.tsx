import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { MessageSquarePlus, MessageSquareText, Search, UserRound } from "lucide-react";
import { Highlight } from "@/components/Highlight";
import { Page, PageHeader } from "@/components/list";
import { Alert, Button, Spinner } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { matches } from "@/lib/search";
import { useContactOpener } from "@/features/conversations/store";
import { useContacts } from "./api";

export function ContactsPage() {
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [failure, setFailure] = useState("");
  const navigate = useNavigate();
  const opener = useContactOpener();
  const { contacts, isPending, error } = useContacts();
  const term = search.trim();
  // Sem acento/maiúscula; com 3+ dígitos casa também pedaços do telefone.
  const visible = contacts.filter((c) => matches(`${c.name} ${c.phone} ${c.company ?? ""}`, term));

  async function start(contactId: string) {
    setBusy(contactId);
    setFailure("");
    try {
      const conv = await opener.openContact(contactId);
      navigate(`/conversas/${conv.id}`);
    } catch (e) {
      setFailure(errorMessage(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <Page>
      <PageHeader title="Clientes" />
      <div className="mx-auto max-w-4xl px-4 py-5 sm:px-6">
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <label className="flex flex-1 items-center gap-2 rounded-lg border border-line bg-elevated px-3 py-2 text-sm">
            <Search className="size-4 shrink-0 text-muted" />
            <input
              aria-label="Buscar clientes"
              placeholder="Buscar por nome, telefone ou empresa"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="min-w-0 flex-1 bg-transparent outline-none"
            />
          </label>
          <span className="text-xs text-muted">
            {term ? `${visible.length} de ${contacts.length}` : contacts.length} contatos
          </span>
        </div>
        {failure && (
          <div className="mb-3">
            <Alert>{failure}</Alert>
          </div>
        )}
        {isPending && <Spinner className="mx-auto my-10" />}
        {error && <Alert>{errorMessage(error)}</Alert>}
        {!isPending && !error && visible.length === 0 && (
          <p className="py-12 text-center text-sm text-muted">
            {search ? "Nenhum contato encontrado." : "Os contatos do WhatsApp aparecerão aqui após a sincronização."}
          </p>
        )}
        <ul className="divide-y divide-line">
          {visible.map((c) => (
            <li key={c.id} className="flex items-center gap-3 py-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-hover text-muted">
                <UserRound className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  <Highlight text={c.name} term={term} />
                </p>
                <p className="mt-0.5 truncate text-xs text-muted">
                  <Highlight text={[c.phone, c.company].filter(Boolean).join(" · ") || "Telefone não informado"} term={term} />
                </p>
              </div>
              {c.conversationId ? (
                <Link
                  to={`/conversas/${c.conversationId}`}
                  className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs text-muted hover:bg-hover hover:text-fg"
                >
                  <MessageSquareText className="size-4" /> <span className="hidden sm:inline">Ver conversa</span>
                </Link>
              ) : (
                <Button variant="secondary" loading={busy === c.id} onClick={() => start(c.id)}>
                  <MessageSquarePlus className="size-4" /> Conversar
                </Button>
              )}
            </li>
          ))}
        </ul>
      </div>
    </Page>
  );
}
