import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, CircleCheck, EllipsisVertical, PanelRight, Search, X } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { IconButton, Spinner } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatDayDivider, sameDay } from "@/lib/format";
import { Composer } from "./Composer";
import { MessageBubble } from "./MessageBubble";
import { buildOutgoing, useConversationActions, useMessages, useSendMessage } from "./store";
import { segments, type Conversation } from "./types";

const normalize = (s: string) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();

export function ChatPanel({
  conversation: c,
  contactOpen,
  onToggleContact,
  onBack,
}: {
  conversation: Conversation;
  contactOpen: boolean;
  onToggleContact: () => void;
  onBack?: () => void;
}) {
  const { data: messages, isPending } = useMessages(c.id);
  const actions = useConversationActions();
  const send = useSendMessage(c.id);
  const scroller = useRef<HTMLDivElement>(null);
  const [search, setSearch] = useState<string | null>(null);

  useEffect(() => actions.markRead(c.id), [c.id, c.unreadCount, actions]);

  // Rola para o fim ao abrir a conversa e quando chega/sai mensagem.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [c.id, messages?.length]);

  const shown = useMemo(() => {
    if (!messages || !search?.trim()) return messages ?? [];
    const q = normalize(search.trim());
    return messages.filter(
      (m) =>
        (m.kind === "text" && normalize(m.text).includes(q)) ||
        (m.kind === "document" && normalize(m.fileName).includes(q)) ||
        (m.kind === "image" && normalize(m.caption ?? "").includes(q)),
    );
  }, [messages, search]);

  const mine = c.assignee?.id === actions.me.id;
  const lockedByOther = !!c.assignee && !mine && c.status === "in_progress";

  let banner: React.ReactNode;
  if (c.status === "new" || !c.assignee) {
    banner = (
      <>
        Aguardando atendimento · <span className="text-fg">ao responder, você assume esta conversa.</span>
      </>
    );
  } else if (c.status === "closed") {
    banner = <>Atendimento encerrado. Uma nova mensagem do cliente poderá ser reclassificada.</>;
  } else if (mine) {
    banner = (
      <span className="flex items-center gap-2">
        Você está atendendo
        <button className="font-medium text-accent hover:underline" onClick={() => actions.close(c.id)}>
          Encerrar atendimento
        </button>
      </span>
    );
  } else {
    banner = <>Em atendimento por {c.assignee.name}</>;
  }

  return (
    <section aria-label={`Conversa com ${c.contact.name}`} className="flex h-full min-w-0 flex-col">
      <header className="flex h-16 shrink-0 items-center gap-2 border-b border-line bg-panel px-3">
        {onBack && (
          <IconButton label="Voltar para a lista" onClick={onBack}>
            <ArrowLeft className="size-5" />
          </IconButton>
        )}
        <button
          type="button"
          onClick={onToggleContact}
          aria-expanded={contactOpen}
          aria-label="Ver dados do contato"
          className="flex min-w-0 items-center gap-3 rounded-lg py-1 pl-1 pr-3 text-left hover:bg-hover"
        >
          <Avatar name={c.contact.name} src={c.contact.avatarUrl} />
          <span className="min-w-0">
            <span className="block truncate font-semibold">{c.contact.name}</span>
            <span className="flex items-center gap-2 text-xs text-muted">
              <span className={cn("rounded-full px-2 py-0.5 font-medium", segments[c.segment].className)}>
                {segments[c.segment].label}
              </span>
              <span className="hidden truncate sm:inline">{c.contact.phone}</span>
            </span>
          </span>
        </button>
        <div className="ml-auto flex items-center">
          <IconButton
            label="Pesquisar na conversa"
            active={search !== null}
            onClick={() => setSearch((s) => (s === null ? "" : null))}
          >
            <Search className="size-5" />
          </IconButton>
          <span className="hidden md:contents">
            <IconButton
              label={contactOpen ? "Fechar dados do contato" : "Dados do contato"}
              active={contactOpen}
              onClick={onToggleContact}
            >
              <PanelRight className="size-5" />
            </IconButton>
          </span>
          <Menu
            trigger={({ toggle }) => (
              <IconButton label="Mais opções" onClick={toggle}>
                <EllipsisVertical className="size-5" />
              </IconButton>
            )}
          >
            {(close) => (
              <>
                <MenuItem onClick={() => (onToggleContact(), close())}>Dados do contato</MenuItem>
                <MenuItem
                  icon={<CircleCheck />}
                  disabled={!mine || c.status === "closed"}
                  onClick={() => (actions.close(c.id), close())}
                >
                  Encerrar atendimento
                </MenuItem>
              </>
            )}
          </Menu>
        </div>
      </header>

      {search !== null && (
        <div className="flex items-center gap-2 border-b border-line bg-panel px-4 py-2">
          <Search className="size-4 text-muted" />
          <input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setSearch(null)}
            placeholder="Pesquisar mensagens"
            aria-label="Pesquisar mensagens"
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          {search && <span className="text-xs text-muted">{shown.length} resultado(s)</span>}
          <IconButton label="Fechar pesquisa" onClick={() => setSearch(null)} className="size-7">
            <X className="size-4" />
          </IconButton>
        </div>
      )}

      <div ref={scroller} className="chat-wallpaper min-h-0 flex-1 overflow-y-auto px-3 py-4 sm:px-8">
        {isPending && <Spinner className="mx-auto mt-10" />}
        <div className="mx-auto flex max-w-5xl flex-col gap-1.5">
          {shown.map((m, i) => {
            const prev = shown[i - 1];
            const newDay = !prev || !sameDay(prev.sentAt, m.sentAt);
            const showAuthor = m.direction === "out" && (!prev || newDay || prev.direction !== "out" || prev.authorName !== m.authorName);
            return (
              <Fragment key={m.id}>
                {newDay && (
                  <div className="my-3 flex justify-center">
                    <span className="rounded-lg bg-elevated px-3 py-1 text-xs text-fg/80 shadow-sm">
                      {formatDayDivider(m.sentAt)}
                    </span>
                  </div>
                )}
                <div className={cn(showAuthor || (prev && prev.direction !== m.direction) ? "mt-1.5" : "")}>
                  <MessageBubble m={m} showAuthor={showAuthor} />
                </div>
              </Fragment>
            );
          })}
        </div>
      </div>

      {!lockedByOther && <div className="border-t border-line bg-panel px-4 py-1.5 text-xs text-muted">{banner}</div>}
      <Composer
        key={c.id}
        conversationId={c.id}
        disabled={lockedByOther}
        disabledReason={lockedByOther ? `${c.assignee?.name} está atendendo esta conversa.` : undefined}
        onSend={(d) => send.mutate({ msg: buildOutgoing(c.id, actions.me.name, d) })}
      />
    </section>
  );
}
