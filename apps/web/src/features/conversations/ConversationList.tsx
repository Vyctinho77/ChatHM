import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { CheckCheck, EllipsisVertical, ListFilter, Search, SquarePen } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { Badge, IconButton, Spinner } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatListTime } from "@/lib/format";
import { MessagePreview } from "./preview";
import { useConversationActions, useConversations } from "./store";
import { Ticks } from "./Ticks";
import { segments, type Conversation, type SegmentKey } from "./types";

type Filter = "all" | "unread" | "mine" | "general";

const normalize = (s: string) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();

export function ConversationList({ selectedId, onSelect }: { selectedId?: string; onSelect: (id: string) => void }) {
  const { data, isPending } = useConversations();
  const actions = useConversationActions();
  const [filter, setFilter] = useState<Filter>("all");
  const [segment, setSegment] = useState<SegmentKey | null>(null);
  const [query, setQuery] = useState("");
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

  const all = useMemo(() => data ?? [], [data]);
  const counts = {
    all: all.length,
    unread: all.filter((c) => c.unreadCount > 0).length,
  };

  const visible = useMemo(() => {
    const q = normalize(query.trim());
    return all.filter((c) => {
      if (filter === "unread" && c.unreadCount === 0) return false;
      if (filter === "mine" && c.assignee?.id !== actions.me.id) return false;
      if (filter === "general" && c.segment !== "geral") return false;
      if (segment && c.segment !== segment) return false;
      if (!q) return true;
      const hay = normalize(
        [c.contact.name, c.contact.phone, c.contact.company ?? "", c.lastMessage.kind === "text" ? c.lastMessage.text : ""].join(" "),
      );
      const digits = q.replace(/\D/g, "");
      return hay.includes(q) || (digits.length >= 3 && c.contact.phone.replace(/\D/g, "").includes(digits));
    });
  }, [all, filter, segment, query, actions.me.id]);

  const chips: { key: Filter; label: string; count?: number }[] = [
    { key: "all", label: "Todas", count: counts.all },
    { key: "unread", label: "Não lidas", count: counts.unread },
    { key: "mine", label: "Minhas" },
    { key: "general", label: "Gerais" },
  ];

  return (
    <section aria-label="Conversas" className="flex h-full min-w-0 flex-col bg-panel">
      <div className="flex items-center gap-0.5 px-3 pt-3">
        <h1 className="mr-auto pl-1 text-base font-semibold">Conversas</h1>
        <IconButton label="Nova conversa (disponível após conectar o WhatsApp)" disabled>
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
              {(Object.keys(segments) as SegmentKey[]).map((k) => (
                <MenuItem key={k} active={segment === k} onClick={() => (setSegment(k), close())}>
                  {segments[k].label}
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
            {c.count !== undefined && c.count > 0 && (
              <Badge
                className={cn(filter === c.key ? "bg-white/25 text-white" : "bg-hover text-muted")}
              >
                {c.count}
              </Badge>
            )}
          </button>
        ))}
      </div>

      {segment && (
        <div className="px-4 pb-2 text-[11.5px] text-muted">
          Segmento: <span className="font-medium text-fg">{segments[segment].label}</span>{" "}
          <button className="text-accent hover:underline" onClick={() => setSegment(null)}>
            limpar
          </button>
        </div>
      )}

      <motion.div layoutScroll className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-2">
        {isPending && <Spinner className="mx-auto mt-8" />}
        {!isPending && visible.length === 0 && (
          <p className="px-4 py-10 text-center text-sm text-muted">Nenhuma conversa encontrada.</p>
        )}
        <ul className="flex flex-col">
          <AnimatePresence initial={false}>
            {visible.map((c) => (
              <ConversationRow key={c.id} c={c} selected={c.id === selectedId} onClick={() => onSelect(c.id)} />
            ))}
          </AnimatePresence>
        </ul>
      </motion.div>
    </section>
  );
}

function ConversationRow({ c, selected, onClick }: { c: Conversation; selected: boolean; onClick: () => void }) {
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
            <span className={cn("ml-auto shrink-0 text-[11px] tabular-nums", unread ? "font-semibold text-unread" : "text-muted")}>
              {formatListTime(last.sentAt)}
            </span>
          </div>
          <div className="mt-px flex items-center gap-1 text-[12.5px] text-muted">
            {last.direction === "out" && <Ticks status={last.status} />}
            <span className="flex min-w-0 items-center gap-1">
              <MessagePreview m={last} />
            </span>
            <AnimatePresence initial={false}>
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
