import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import { CheckCheck, Clock3, EllipsisVertical, ListFilter, Search, SquarePen, StickyNote } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Logo } from "@/components/Logo";
import { Menu, MenuItem } from "@/components/Menu";
import { Badge, IconButton } from "@/components/ui";
import { ListSkeleton } from "@/components/Skeleton";
import { canSupervise, useMe } from "@/features/auth/api";
import { cn } from "@/lib/cn";
import { formatListTime } from "@/lib/format";
import { matches } from "@/lib/search";
import { useDebounced } from "@/lib/useDebounced";
import { NewConversationModal } from "./NewConversation";
import { MessagePreview } from "./preview";
import { SearchResults } from "./SearchResults";
import { useContactOpener, useConversationActions, useConversations, useMessageSearch, useSegmentOptions } from "./store";
import { Ticks } from "./Ticks";
import { TransferMenu } from "./TransferMenu";
import { countsByTab, filterConversations, type ConversationFilter as Filter } from "./conversationFilter";
import { reminderColors, useReminders, type Reminder } from "./reminders";
import { ReminderEditor } from "./ReminderEditor";
import { useTyping } from "./typing";
import { TypingIndicator } from "./TypingIndicator";
import type { ContactRecord, Conversation, MessageHit, SegmentKey, SegmentOption } from "./types";

const contactCountLabel = (count: number) => `${count} ${count === 1 ? "contato" : "contatos"}`;

export function ConversationList({ selectedId, onSelect }: { selectedId?: string; onSelect: (id: string) => void }) {
  const { data, isPending } = useConversations();
  const actions = useConversationActions();
  const { data: meData } = useMe();
  const isAdmin = canSupervise(meData?.user.role); // administrador ou gestor
  const [ctx, setCtx] = useState<{ id: string; x: number; y: number } | null>(null);
  // O filtro de fila fica na URL (?fila=folha): a tela Filas / Segmentos abre a lista já filtrada.
  const [params, setParams] = useSearchParams();
  const requestedFilter = params.get("vista");
  const filter: Filter = requestedFilter === "mine" || requestedFilter === "general" || requestedFilter === "groups" || requestedFilter === "snoozed" ? requestedFilter : "all";
  const reminders = useReminders(filter === "mine");
  const [editingReminder, setEditingReminder] = useState<Conversation | null>(null);
  const setFilter = (nextFilter: Filter) => setParams((p) => {
    const next = new URLSearchParams(p);
    if (nextFilter === "all") next.delete("vista");
    else next.set("vista", nextFilter);
    return next;
  }, { replace: true });
  const segment: SegmentKey | null = params.get("fila");
  const setSegment = (k: SegmentKey | null) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p);
        if (k) next.set("fila", k);
        else next.delete("fila");
        return next;
      },
      { replace: true },
    );
  const { data: segmentData } = useSegmentOptions();
  const segmentOptions: SegmentOption[] = [...(segmentData ?? []), { key: "geral", name: "Não atribuídos" }];
  const [query, setQuery] = useState("");
  const [newOpen, setNewOpen] = useState(false);
  const navigate = useNavigate();
  const opener = useContactOpener();
  const debounced = useDebounced(query.trim(), 250);
  const server = useMessageSearch(debounced);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Atendimentos encerrados saem da lista; um novo contato do cliente abre outro.
  const all = useMemo(() => (data ?? []).filter((c) => c.status !== "closed"), [data]);
  const tabCounts = useMemo(() => countsByTab(all, actions.me.id, segment), [all, actions.me.id, segment]);

  const visible = useMemo(() => {
    const q = query.trim();
    return filterConversations(all, filter, actions.me.id, segment).filter((c) => {
      if (!q) return true;
      const last = c.lastMessage;
      const text = last?.kind === "text" ? last.text : last?.kind === "document" ? (last.fileName ?? "") : "";
      return matches(`${c.contact.name} ${c.contact.phone} ${c.contact.company ?? ""} ${text}`, q);
    });
  }, [all, filter, segment, query, actions.me.id]);

  const openMenu = useCallback((id: string, x: number, y: number) => setCtx({ id, x, y }), []);

  async function openContact(ct: ContactRecord) {
    try {
      const id = ct.conversationId ?? (await opener.openContact(ct.id)).id;
      navigate({ pathname: `/conversas/${id}` });
    } catch {
      /* o aviso aparece ao abrir pela lista de Clientes; aqui só não navega */
    }
  }

  async function openHit(h: MessageHit) {
    try {
      // Mensagem de um atendimento encerrado: o histórico aparece no atendimento aberto do cliente.
      const id = h.closed ? (h.openConversationId ?? (await opener.openContact(h.contactId)).id) : h.conversationId;
      navigate({ pathname: `/conversas/${id}`, search: `?msg=${h.id}` });
    } catch {
      /* sem acesso: nada a abrir */
    }
  }

  const chips: { key: Filter; label: string; showCount?: boolean }[] = [
    { key: "mine", label: "Minhas", showCount: true },
    { key: "general", label: "Não atribuídos", showCount: true },
    { key: "groups", label: "Grupos" },
    { key: "all", label: "Todas" },
    { key: "snoozed", label: "Adiadas", showCount: true },
  ];
  const ctxConv = ctx ? all.find((c) => c.id === ctx.id) : undefined;

  return (
    <section aria-label="Conversas" className="flex h-full min-w-0 flex-col bg-panel">
      <div className="flex items-center gap-0.5 px-3 pt-3">
        <h1 className="mr-auto pl-1 text-base font-semibold">
          <span className="lg:hidden">Conversas</span>
          <span className="hidden lg:inline-flex"><Logo className="[&_img]:h-7" /></span>
        </h1>
        <IconButton label="Nova conversa" onClick={() => setNewOpen(true)}>
          <SquarePen className="size-5" />
        </IconButton>
        <Menu
          trigger={({ toggle }) => (
            <IconButton label="Filtrar por segmento" onClick={toggle} active={!!segment}>
              <ListFilter className="size-5" />
            </IconButton>
          )}
        >
          {(close) => (
            <>
              <MenuItem active={!segment} onClick={() => (setSegment(null), close())}>
                Todos os segmentos
              </MenuItem>
              {segmentOptions.map((o) => (
                <MenuItem key={o.key} active={segment === o.key} onClick={() => (setSegment(o.key), close())}>
                  {o.name}
                </MenuItem>
              ))}
            </>
          )}
        </Menu>
        <Menu
          trigger={({ toggle }) => (
            <IconButton label="Mais opções" onClick={toggle}>
              <EllipsisVertical className="size-5" />
            </IconButton>
          )}
        >
          {(close) => (
            <MenuItem icon={<CheckCheck />} onClick={() => (actions.markAllRead(), close())}>
              Marcar todas como lidas
            </MenuItem>
          )}
        </Menu>
      </div>

      <div className="px-3 pt-2.5">
        <label className="flex h-8 items-center gap-2 rounded-lg border border-line bg-elevated px-2.5 focus-within:border-muted">
          <Search className="size-3.5 shrink-0 text-muted" />
          <input
            ref={searchRef}
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Pesquisar conversas"
            aria-label="Pesquisar conversas"
            className="min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-muted"
          />
          <kbd className="hidden rounded border border-line px-1 font-sans text-[10px] text-muted sm:block">
            Ctrl + K
          </kbd>
        </label>
      </div>

      <div role="tablist" aria-label="Filtros" className="flex gap-0.5 overflow-x-auto px-2 py-2.5 [scrollbar-width:none]">
        {chips.map((c) => (
          <button
            key={c.key}
            role="tab"
            aria-selected={filter === c.key}
            aria-label={isPending ? c.label : `${c.label}: ${contactCountLabel(tabCounts[c.key].contacts)}${tabCounts[c.key].unread ? `, ${tabCounts[c.key].unread} com mensagens não lidas` : ""}`}
            onClick={() => setFilter(c.key)}
            className={cn(
              "inline-flex h-7 shrink-0 items-center gap-1 rounded-full px-2 text-xs transition-colors",
              filter === c.key ? "font-semibold text-brand-ink" : "text-muted hover:bg-hover hover:text-fg",
            )}
          >
            {c.label}
            {!isPending && c.showCount && <span aria-hidden="true" title={contactCountLabel(tabCounts[c.key].contacts)} className="text-[10px] tabular-nums text-muted">{tabCounts[c.key].contacts}</span>}
            {!isPending && tabCounts[c.key].unread > 0 && <span aria-hidden="true" title={`${contactCountLabel(tabCounts[c.key].unread)} com mensagens novas`} className="size-1.5 shrink-0 rounded-full bg-unread" />}
          </button>
        ))}
      </div>

      {segment && (
        <div className="px-4 pb-2 text-[11.5px] text-muted">
          Segmento: <span className="font-medium text-fg">{segmentOptions.find((o) => o.key === segment)?.name ?? segment}</span>{" "}
          <button className="text-brand-ink hover:underline" onClick={() => setSegment(null)}>
            limpar
          </button>
        </div>
      )}

      <motion.div layoutScroll className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-2">
        {filter === "mine" && reminders.isError && <p role="status" className="px-3 py-2 text-xs text-muted">Não foi possível carregar os lembretes. <button type="button" className="text-brand-ink underline" onClick={() => void reminders.refetch()}>Tentar novamente</button></p>}
        {isPending && <ListSkeleton rows={6} label="Carregando conversas" />}
        {!isPending && visible.length === 0 && !(debounced.length >= 2 && debounced === query.trim() && ((server.data?.messages.length ?? 0) > 0 || (server.data?.contacts.length ?? 0) > 0 || server.isFetching)) && (
          <p className="px-4 py-10 text-center text-sm text-muted">Nenhuma conversa encontrada.</p>
        )}
        <ul className="flex flex-col">
          {visible.map((c) => (
            <ConversationRow
              key={c.id}
              c={c}
              myId={actions.me.id}
              selected={c.id === selectedId}
              showReminder={filter === "mine" && c.assignee?.id === actions.me.id}
              reminder={reminders.data?.[c.id]}
              reminderReady={reminders.isSuccess}
              onEditReminder={setEditingReminder}
              onSelect={onSelect}
              onMenu={openMenu}
            />
          ))}
        </ul>
        {debounced.length >= 2 && debounced === query.trim() && (
          <SearchResults
            term={debounced}
            loading={server.isFetching}
            contacts={(server.data?.contacts ?? []).filter((ct) => !visible.some((c) => c.contact.id === ct.id))}
            messages={server.data?.messages ?? []}
            onOpenMessage={openHit}
            onOpenContact={openContact}
          />
        )}
      </motion.div>
      <NewConversationModal open={newOpen} onClose={() => setNewOpen(false)} />
      {editingReminder && <ReminderEditor key={editingReminder.id} conversationId={editingReminder.id} contactName={editingReminder.contact.name} reminder={reminders.data?.[editingReminder.id]} onClose={() => setEditingReminder(null)} />}
      {ctx && ctxConv && (
        <TransferMenu
          conversation={ctxConv}
          x={ctx.x}
          y={ctx.y}
          myId={actions.me.id}
          canTransfer={!ctxConv.assignee || ctxConv.assignee.id === actions.me.id || isAdmin}
          onPick={(p) => actions.transfer(ctxConv.id, p)}
          onClose={() => setCtx(null)}
        />
      )}
    </section>
  );
}

const ConversationRow = memo(function ConversationRow({
  c,
  myId,
  selected,
  onSelect,
  onMenu,
  showReminder,
  reminder,
  reminderReady,
  onEditReminder,
}: {
  c: Conversation;
  myId: string;
  selected: boolean;
  onSelect: (id: string) => void;
  onMenu: (id: string, x: number, y: number) => void;
  showReminder: boolean;
  reminder?: Reminder;
  reminderReady: boolean;
  onEditReminder: (c: Conversation) => void;
}) {
  const unread = c.unreadCount > 0;
  const typing = useTyping(c.id);
  const last = c.lastMessage;
  return (
    // Sem animação de entrada/saída por linha: trocar de aba monta centenas de linhas e as molas travavam a tela.
    // content-visibility deixa o navegador pular o layout/pintura das linhas fora da área visível.
    <li className="[contain-intrinsic-size:auto_60px] [content-visibility:auto]">
      <button
        type="button"
        onClick={() => onSelect(c.id)}
        onContextMenu={(e) => {
          e.preventDefault();
          onMenu(c.id, e.clientX, e.clientY);
        }}
        aria-current={selected ? "true" : undefined}
        className={cn(
          "relative flex w-full gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors",
          !selected && "hover:bg-hover",
        )}
      >
        {selected && <span className="absolute inset-0 rounded-lg bg-hover" aria-hidden />}
        <span className="relative">
        <Avatar name={c.contact.name} src={c.contact.avatarUrl} group={c.contact.kind === "group"} size="list" />
        </span>
        <div className="relative min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <div className="flex min-w-0 flex-1 items-center gap-1.5">
              <span className="truncate text-[13.5px] font-medium">{c.contact.name}</span>
              {c.tags[0] && <span title={c.tags[0]} className="max-w-[45%] shrink-0 truncate rounded-full border border-line px-1.5 py-px text-[9.5px] font-semibold uppercase tracking-wide text-fg/75">{c.tags[0]}</span>}
              {c.tags.length > 1 && <span title={c.tags.slice(1).join(", ")} aria-label={`Outras etiquetas: ${c.tags.slice(1).join(", ")}`} className="shrink-0 text-[10px] font-medium text-muted">+{c.tags.length - 1}</span>}
            </div>
            {last && (
              <span className={cn("ml-auto shrink-0 text-[11px] tabular-nums", unread ? "font-semibold text-unread" : "text-muted")}>
                {formatListTime(last.sentAt)}
              </span>
            )}
          </div>
          <div className="mt-px flex items-center gap-1 text-[12.5px] text-muted">
            {!typing && last?.direction === "out" && <Ticks status={last.status} />}
            <span className="flex min-w-0 items-center gap-1">
              {typing ? <TypingIndicator /> : last ? <MessagePreview m={last} /> : <span className="italic">Nova conversa — escreva a primeira mensagem</span>}
            </span>
            {/* Embaixo da hora: quem está atendendo (nomes grandes de cliente não escondem o responsável). */}
            {c.assignee && c.status !== "closed" && (
              <span
                className={cn("ml-auto max-w-[42%] shrink-0 truncate text-[11px]", c.assignee.id === myId ? "font-medium text-brand-ink" : "text-muted")}
                title={c.assignee.id === myId ? "Você está atendendo" : `Atendendo: ${c.assignee.name}`}
              >
                <span className="sr-only">Atendendo: </span>
                {c.assignee.id === myId ? "Você" : c.participants?.some((p) => p.id === myId) ? `${c.assignee.name.trim().split(/\s+/)[0]} + você` : c.assignee.name.trim().split(/\s+/)[0]}
              </span>
            )}
            <AnimatePresence initial={false}>
              {unread && (
                <motion.span
                  key="badge"
                  className={cn(!c.assignee || c.status === "closed" ? "ml-auto" : "")}
                  initial={{ scale: 0 }}
                  animate={{ scale: 1 }}
                  exit={{ scale: 0 }}
                  transition={{ type: "spring", stiffness: 600, damping: 22 }}
                >
                  <Badge className="bg-unread text-accent-fg">{c.unreadCount}</Badge>
                </motion.span>
              )}
            </AnimatePresence>
          </div>
          {c.status === "closed" && (
            <div className="mt-1 flex flex-wrap gap-1">
              {c.status === "closed" && (
                <span className="rounded-full bg-hover px-1.5 py-px text-[9.5px] font-semibold uppercase tracking-wide text-muted">
                  Encerrada
                </span>
              )}
            </div>
          )}
          {c.snoozedUntil && <p className="mt-1 flex items-center gap-1 text-[11px] text-muted"><Clock3 className="size-3 shrink-0" />Retorna {new Date(c.snoozedUntil).toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" })}</p>}
        </div>
      </button>
      {showReminder && <div className="-mt-1 pb-2.5 pl-14 pr-2.5">
        <button type="button" disabled={!reminderReady} onClick={() => onEditReminder(c)} aria-label={`${reminder ? "Editar" : "Adicionar"} lembrete de ${c.contact.name}`} title={reminder?.text} className={cn("flex w-full items-start gap-1.5 rounded-md px-2 py-1.5 text-left text-xs leading-[17px] transition-colors disabled:opacity-50", reminder ? reminderColors[reminder.color].style : "text-muted hover:bg-hover hover:text-fg")}>
          <StickyNote className="mt-0.5 size-[13px] shrink-0" />
          <span className="min-w-0 break-words line-clamp-2">{reminder?.text ?? (reminderReady ? "Adicionar lembrete" : "Carregando lembrete…")}</span>
        </button>
      </div>}
    </li>
  );
});
