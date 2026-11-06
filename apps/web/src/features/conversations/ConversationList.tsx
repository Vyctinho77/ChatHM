import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import { CheckCheck, EllipsisVertical, ListFilter, Search, SquarePen } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { Badge, IconButton, Spinner } from "@/components/ui";
import { useMe } from "@/features/auth/api";
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
import type { ContactRecord, Conversation, MessageHit, SegmentKey, SegmentOption } from "./types";

type Filter = "mine" | "general" | "all";

export function ConversationList({ selectedId, onSelect }: { selectedId?: string; onSelect: (id: string) => void }) {
  const { data, isPending } = useConversations();
  const actions = useConversationActions();
  const [filter, setFilter] = useState<Filter>("all");
  const { data: meData } = useMe();
  const isAdmin = meData?.user.role === "admin";
  const [ctx, setCtx] = useState<{ id: string; x: number; y: number } | null>(null);
  // O filtro de fila fica na URL (?fila=folha): a tela Filas / Segmentos abre a lista já filtrada.
  const [params, setParams] = useSearchParams();
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
  const segmentOptions: SegmentOption[] = [...(segmentData ?? []), { key: "geral", name: "Geral" }];
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

  const visible = useMemo(() => {
    const q = query.trim();
    return all.filter((c) => {
      if (filter === "mine" && c.assignee?.id !== actions.me.id) return false;
      if (filter === "general" && c.segment !== "geral") return false;
      if (segment && c.segment !== segment) return false;
      if (!q) return true;
      const last = c.lastMessage;
      const text = last?.kind === "text" ? last.text : last?.kind === "document" ? last.fileName : "";
      return matches(`${c.contact.name} ${c.contact.phone} ${c.contact.company ?? ""} ${text}`, q);
    });
  }, [all, filter, segment, query, actions.me.id]);

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

  const chips: { key: Filter; label: string }[] = [
    { key: "mine", label: "Minhas" },
    { key: "general", label: "Geral" },
    { key: "all", label: "Todas" },
  ];
  const ctxConv = ctx ? all.find((c) => c.id === ctx.id) : undefined;

  return (
    <section aria-label="Conversas" className="flex h-full min-w-0 flex-col bg-panel">
      <div className="flex items-center gap-0.5 px-3 pt-3">
        <h1 className="mr-auto pl-1 text-base font-semibold">Conversas</h1>
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
        <label className="flex h-8 items-center gap-2 rounded-lg border border-line bg-elevated px-2.5 focus-within:border-accent">
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

      <div role="tablist" aria-label="Filtros" className="flex flex-wrap gap-1 px-3 py-2.5">
        {chips.map((c) => (
          <button
            key={c.key}
            role="tab"
            aria-selected={filter === c.key}
            onClick={() => setFilter(c.key)}
            className={cn(
              "inline-flex h-7 shrink-0 items-center gap-1 rounded-full border px-2.5 text-xs transition-colors",
              filter === c.key
                ? "border-accent bg-accent text-accent-fg"
                : "border-line text-fg/85 hover:bg-hover",
            )}
          >
            {c.label}
          </button>
        ))}
      </div>

      {segment && (
        <div className="px-4 pb-2 text-[11.5px] text-muted">
          Segmento: <span className="font-medium text-fg">{segmentOptions.find((o) => o.key === segment)?.name ?? segment}</span>{" "}
          <button className="text-accent hover:underline" onClick={() => setSegment(null)}>
            limpar
          </button>
        </div>
      )}

      <motion.div layoutScroll className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-2">
        {isPending && <Spinner className="mx-auto mt-8" />}
        {!isPending && visible.length === 0 && !(debounced.length >= 2 && debounced === query.trim() && ((server.data?.messages.length ?? 0) > 0 || (server.data?.contacts.length ?? 0) > 0 || server.isFetching)) && (
          <p className="px-4 py-10 text-center text-sm text-muted">Nenhuma conversa encontrada.</p>
        )}
        <ul className="flex flex-col">
          <AnimatePresence initial={false}>
            {visible.map((c) => (
              <ConversationRow
                key={c.id}
                c={c}
                myId={actions.me.id}
                selected={c.id === selectedId}
                onClick={() => onSelect(c.id)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setCtx({ id: c.id, x: e.clientX, y: e.clientY });
                }}
              />
            ))}
          </AnimatePresence>
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

function ConversationRow({
  c,
  myId,
  selected,
  onClick,
  onContextMenu,
}: {
  c: Conversation;
  myId: string;
  selected: boolean;
  onClick: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}) {
  const unread = c.unreadCount > 0;
  const last = c.lastMessage;
  return (
    <motion.li
      layout="position"
      initial={{ opacity: 0, height: 0 }}
      animate={{ opacity: 1, height: "auto" }}
      exit={{ opacity: 0, height: 0 }}
      transition={{ type: "spring", stiffness: 500, damping: 40 }}
      className="overflow-hidden"
    >
      <button
        type="button"
        onClick={onClick}
        onContextMenu={onContextMenu}
        aria-current={selected ? "true" : undefined}
        className={cn(
          "relative flex w-full gap-2.5 rounded-lg px-2.5 py-2 text-left transition-colors",
          !selected && "hover:bg-hover",
        )}
      >
        {selected && (
          <motion.span
            layoutId="conversation-selected"
            className="absolute inset-0 rounded-lg bg-selected"
            transition={{ type: "spring", stiffness: 500, damping: 40 }}
            aria-hidden
          />
        )}
        <span className="relative">
        <Avatar name={c.contact.name} src={c.contact.avatarUrl} size="list" />
        </span>
        <div className="relative min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <span className="truncate text-[13.5px] font-medium">{c.contact.name}</span>
            {last && (
              <span className={cn("ml-auto shrink-0 text-[11px] tabular-nums", unread ? "font-semibold text-unread" : "text-muted")}>
                {formatListTime(last.sentAt)}
              </span>
            )}
          </div>
          <div className="mt-px flex items-center gap-1 text-[12.5px] text-muted">
            {last?.direction === "out" && <Ticks status={last.status} />}
            <span className="flex min-w-0 items-center gap-1">
              {last ? <MessagePreview m={last} /> : <span className="italic">Nova conversa — escreva a primeira mensagem</span>}
            </span>
            <AnimatePresence initial={false}>
              {!unread && c.assignee && c.assignee.id !== myId && (
                <span key="assignee" className="ml-auto max-w-[40%] shrink-0 truncate text-[11px]" title={`Atendendo: ${c.assignee.name}`}>
                  {c.assignee.name.split(" ")[0]}
                </span>
              )}
              {unread && (
                <motion.span
                  key="badge"
                  className="ml-auto"
                  initial={{ scale: 0 }}
                  animate={{ scale: 1 }}
                  exit={{ scale: 0 }}
                  transition={{ type: "spring", stiffness: 600, damping: 22 }}
                >
                  <Badge className="bg-unread text-white">{c.unreadCount}</Badge>
                </motion.span>
              )}
            </AnimatePresence>
          </div>
          {(c.tags.length > 0 || c.status === "closed") && (
            <div className="mt-1 flex flex-wrap gap-1">
              {c.status === "closed" && (
                <span className="rounded-full bg-hover px-1.5 py-px text-[9.5px] font-semibold uppercase tracking-wide text-muted">
                  Encerrada
                </span>
              )}
              {c.tags.map((t) => (
                <span
                  key={t}
                  className="rounded-full border border-line px-1.5 py-px text-[9.5px] font-semibold uppercase tracking-wide text-fg/75"
                >
                  {t}
                </span>
              ))}
            </div>
          )}
        </div>
      </button>
    </motion.li>
  );
}
