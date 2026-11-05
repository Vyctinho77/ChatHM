import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { MessageSquareText, Search, UserRound } from "lucide-react";
import { Page, PageHeader } from "@/components/list";
import { Alert, Spinner } from "@/components/ui";
import { get, errorMessage } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { useConversations } from "@/features/conversations/store";
import type { Contact } from "@/features/conversations/types";

type ContactRecord = Contact & { conversationId?: string };

export function ContactsPage() {
  const [search, setSearch] = useState("");
  const demoConversations = useConversations();
  const query = useQuery({
    queryKey: ["contacts"],
    enabled: !DEMO,
    queryFn: async () => (await get<{contacts: ContactRecord[]}>("/contacts")).contacts,
    refetchInterval: 10_000,
  });
  const contacts: ContactRecord[] = DEMO
    ? [...new Map((demoConversations.data ?? []).map((c) => [c.contact.id, {...c.contact, conversationId: c.id}])).values()]
    : query.data ?? [];
  const term = search.trim().toLocaleLowerCase();
  const visible = contacts.filter((c) => `${c.name} ${c.phone} ${c.company ?? ""}`.toLocaleLowerCase().includes(term));
  const pending = DEMO ? demoConversations.isPending : query.isPending;
  const error = DEMO ? demoConversations.error : query.error;

  return (
    <Page>
      <PageHeader title="Clientes" />
      <div className="mx-auto max-w-4xl px-4 py-5 sm:px-6">
        <div className="mb-4 flex flex-wrap items-center gap-3">
          <label className="flex flex-1 items-center gap-2 rounded-lg border border-line bg-elevated px-3 py-2 text-sm">
            <Search className="size-4 shrink-0 text-muted" />
            <input aria-label="Buscar clientes" placeholder="Buscar por nome, telefone ou empresa" value={search} onChange={(e) => setSearch(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none" />
          </label>
          <span className="text-xs text-muted">{contacts.length} contatos</span>
        </div>
        {pending && <Spinner className="mx-auto my-10" />}
        {error && <Alert>{errorMessage(error)}</Alert>}
        {!pending && !error && visible.length === 0 && <p className="py-12 text-center text-sm text-muted">{search ? "Nenhum contato encontrado." : "Os contatos do WhatsApp aparecerão aqui após a sincronização."}</p>}
        <ul className="divide-y divide-line">
          {visible.map((c) => (
            <li key={c.id} className="flex items-center gap-3 py-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-hover text-muted"><UserRound className="size-5" /></span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{c.name}</p>
                <p className="mt-0.5 truncate text-xs text-muted">{[c.phone, c.company].filter(Boolean).join(" · ") || "Telefone não informado"}</p>
              </div>
              {c.conversationId ? <Link to={`/conversas/${c.conversationId}`} className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs text-muted hover:bg-hover hover:text-fg"><MessageSquareText className="size-4" /> <span className="hidden sm:inline">Ver conversa</span></Link> : <span className="text-xs text-muted">Sem histórico recebido</span>}
            </li>
          ))}
        </ul>
      </div>
    </Page>
  );
}
