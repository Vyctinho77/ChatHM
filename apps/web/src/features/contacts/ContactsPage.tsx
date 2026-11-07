import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { MessageSquarePlus, MessageSquareText, Search } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { Highlight } from "@/components/Highlight";
import { Page, PageHeader } from "@/components/list";
import { Alert, Button } from "@/components/ui";
import { ListSkeleton } from "@/components/Skeleton";
import { errorMessage } from "@/lib/api";
import { matches } from "@/lib/search";
import { useContactOpener } from "@/features/conversations/store";
import { EditableName } from "@/components/EditableName";
import { useContacts, useRenameContact } from "./api";

export function ContactsPage() {
  const [search, setSearch] = useState("");
  const [tab, setTab] = useState<"person" | "group">("person");
  const [busy, setBusy] = useState<string | null>(null);
  const [failure, setFailure] = useState("");
  const navigate = useNavigate();
  const opener = useContactOpener();
  const rename = useRenameContact();
  const { contacts, isPending, error } = useContacts();
  const term = search.trim();
  // Sem acento/maiúscula; com 3+ dígitos casa também pedaços do telefone.
  const inTab = contacts.filter((c) => (c.kind === "group" ? "group" : "person") === tab);
  const groupCount = contacts.filter((c) => c.kind === "group").length;
  const visible = inTab.filter((c) => matches(`${c.name} ${c.phone} ${c.company ?? ""}`, term));

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
      <PageHeader
        title="Clientes"
        actions={
          <div className="flex min-w-0 items-center gap-3">
            <span className="sr-only whitespace-nowrap text-xs text-muted sm:not-sr-only" aria-live="polite">
              {term ? `${visible.length} de ${inTab.length}` : inTab.length} {tab === "group" ? "grupos" : "contatos"}
            </span>
          <label className="flex h-9 w-[min(52vw,24rem)] items-center gap-2 rounded-lg border border-line bg-elevated px-3 text-[13px] focus-within:border-accent sm:w-80 lg:w-96">
            <Search className="size-4 shrink-0 text-muted" aria-hidden="true" />
            <input
              aria-label="Buscar clientes"
              placeholder="Buscar por nome, telefone ou empresa"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted"
            />
          </label>
          </div>
        }
      />
      <div className="mx-auto max-w-4xl px-4 py-5 sm:px-6">
        <div role="tablist" aria-label="Tipo de contato" className="mb-3 flex gap-1 text-[13px]">
          {([["person", "Pessoas"], ["group", `Grupos${groupCount ? ` (${groupCount})` : ""}`]] as const).map(([key, label]) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={tab === key}
              onClick={() => setTab(key)}
              className={cn("rounded-lg px-3 py-1.5 transition-colors", tab === key ? "bg-selected font-semibold text-brand-ink" : "text-muted hover:bg-hover hover:text-fg")}
            >
              {label}
            </button>
          ))}
        </div>
        {failure && (
          <div className="mb-3">
            <Alert>{failure}</Alert>
          </div>
        )}
        {isPending && <ListSkeleton label="Carregando clientes" />}
        {error && <Alert>{errorMessage(error)}</Alert>}
        {!isPending && !error && visible.length === 0 && (
          <p className="py-12 text-center text-sm text-muted">
            {search ? "Nenhum contato encontrado." : tab === "group" ? "Os grupos aparecem aqui quando alguém escreve neles." : "Os contatos do WhatsApp aparecerão aqui após a sincronização."}
          </p>
        )}
        <ul className="divide-y divide-line">
          {visible.map((c) => (
            <li key={c.id} className="flex items-center gap-3 py-3">
              <Avatar name={c.name} src={c.avatarUrl} group={c.kind === "group"} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">
                  {term ? (
                    <p className="truncate"><Highlight text={c.name} term={term} /></p>
                  ) : (
                    <EditableName value={c.name} label={`Editar nome de ${c.name}`} onSave={(name) => rename(c, name)} />
                  )}
                </div>
                <p className="mt-0.5 truncate text-xs text-muted">
                  <Highlight text={[c.phone, c.company].filter(Boolean).join(" · ") || (c.kind === "group" ? "Grupo do WhatsApp" : "Telefone não informado")} term={term} />
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
