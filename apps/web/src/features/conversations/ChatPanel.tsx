import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ArrowDown, ArrowLeft, CheckCheck, CircleCheck, EllipsisVertical, PanelRight, Search, X } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { Alert, IconButton, Spinner } from "@/components/ui";
import { errorMessage, isRejection } from "@/lib/api";
import { useMe } from "@/features/auth/api";
import { cn } from "@/lib/cn";
import { formatDayDivider, formatTime, sameDay } from "@/lib/format";
import { Composer } from "./Composer";
import { getDraft, setDraft } from "./drafts";
import { MessageBubble } from "./MessageBubble";
import { buildOutgoing, capabilities, useConversationActions, useMessages, useSendMessage, type OutgoingDraft } from "./store";
import { statusLabel } from "./receipts";
import { segmentClass, segmentLabel, type Conversation, type Message } from "./types";

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
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  // Muda ao devolver um texto recusado ao campo de mensagem (remonta o Composer).
  const [composerKey, setComposerKey] = useState(0);
  const scroller = useRef<HTMLDivElement>(null);
  const [search, setSearch] = useState<string | null>(null);
  // Mensagens com horário posterior à abertura da conversa entram animadas.
  // (O componente é remontado a cada conversa: ver key no ConversationsPage.)
  const [openedAt] = useState(() => Date.now());
  const atBottom = useRef(true);
  const scrolledOnce = useRef(false);
  const [showJump, setShowJump] = useState(false);

  useEffect(() => actions.markRead(c.id), [c.id, c.unreadCount, actions]);

  // Ao abrir: vai direto ao fim. Depois: rolagem suave quando chega mensagem,
  // desde que a pessoa esteja perto do fim (ou tenha acabado de enviar).
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el || !messages?.length) return;
    if (!scrolledOnce.current) {
      el.scrollTop = el.scrollHeight;
      scrolledOnce.current = true;
      return;
    }
    const last = messages[messages.length - 1];
    if (atBottom.current || last?.direction === "out") {
      el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
    }
  }, [messages]);

  function onScroll() {
    const el = scroller.current;
    if (!el) return;
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
    atBottom.current = near;
    setShowJump(!near);
  }

  const lastOutId = useMemo(() => {
    for (let i = (messages?.length ?? 0) - 1; i >= 0; i--) {
      const m = messages![i]!;
      if (m.direction === "out") return m.id;
    }
    return undefined;
  }, [messages]);

  const shown = useMemo(() => {
    if (!messages || !search?.trim()) return messages ?? [];
    const q = normalize(search.trim());
    return messages.filter(
      (m) =>
        (m.kind === "text" && normalize(m.text).includes(q)) ||
        (m.kind === "document" && normalize(m.fileName).includes(q)) ||
        (m.kind === "image" && normalize(m.caption ?? "").includes(q)) ||
        (m.kind === "unsupported" && normalize(m.text ?? "").includes(q)),
    );
  }, [messages, search]);

  const mine = c.assignee?.id === actions.me.id;
  const closed = c.status === "closed";
  // Outra pessoa já assumiu: só ela responde (o servidor também garante isso).
  const lockedByOther = !!c.assignee && !mine && !closed;
  // O responsável encerra; administradores também podem encerrar qualquer atendimento.
  const canClose = capabilities.close && !closed && (mine || isAdmin);

  const closeButton = canClose && (
    <button className="font-medium text-accent hover:underline" onClick={() => actions.close(c.id)}>
      Encerrar atendimento
    </button>
  );
  let banner: React.ReactNode;
  if (closed) {
    banner = <>Atendimento encerrado. Uma nova mensagem do cliente abre um novo atendimento.</>;
  } else if (!c.assignee) {
    banner = (
      <span className="flex items-center gap-2">
        <span>
          Aguardando atendimento ·{" "}
          <span className="text-fg">{capabilities.send ? "ao responder, você assume esta conversa." : "fila " + segmentLabel(c)}</span>
        </span>
        {closeButton}
      </span>
    );
  } else if (mine) {
    banner = (
      <span className="flex items-center gap-2">
        Você está atendendo {closeButton}
      </span>
    );
  } else {
    banner = (
      <span className="flex items-center gap-2">
        Em atendimento por {c.assignee.name} {closeButton}
      </span>
    );
  }

  function submit(d: OutgoingDraft) {
    const msg = buildOutgoing(c.id, actions.me.name, d);
    send.mutate(
      { msg },
      {
        onError: (err) => {
          // O servidor recusou (ex.: outra pessoa assumiu): o texto volta ao campo, sem perder o que foi digitado.
          if (d.kind === "text" && isRejection(err) && !getDraft(c.id)) {
            setDraft(c.id, d.text);
            setComposerKey((k) => k + 1);
          }
        },
      },
    );
  }

  return (
    <section aria-label={`Conversa com ${c.contact.name}`} className="flex h-full min-w-0 flex-col">
      <header className="flex h-14 shrink-0 items-center gap-1.5 border-b border-line bg-panel px-2.5">
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
          className="flex min-w-0 items-center gap-2.5 rounded-lg py-1 pl-1 pr-2.5 text-left hover:bg-hover"
        >
          <Avatar name={c.contact.name} src={c.contact.avatarUrl} />
          <span className="min-w-0">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[13.5px] font-semibold">{c.contact.name}</span>
              <span
                title={
                  c.classification === "conflict"
                    ? "Fila geral: a mensagem combinou com mais de um segmento"
                    : c.segment === "geral"
                      ? "Fila geral: sem correspondência com um segmento"
                      : undefined
                }
                className={cn(
                  "shrink-0 rounded-full px-1.5 py-px text-[10.5px] font-medium leading-4",
                  segmentClass(c.segment),
                )}
              >
                {segmentLabel(c)}
              </span>
            </span>
            {c.contact.phone !== c.contact.name && (
              <span className="block truncate text-[11.5px] text-muted">{c.contact.phone}</span>
            )}
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
                  disabled={!canClose}
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

      <div className="relative min-h-0 flex-1">
      <div ref={scroller} onScroll={onScroll} className="chat-wallpaper h-full overflow-y-auto px-3 py-3 sm:px-4">
        {isPending && <Spinner className="mx-auto mt-10" />}
        <motion.div
          className="flex flex-col gap-1"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: messages ? 1 : 0, y: messages ? 0 : 8 }}
          transition={{ duration: 0.25, ease: "easeOut" }}
        >
          {shown.map((m, i) => {
            const prev = shown[i - 1];
            const newDay = !prev || !sameDay(prev.sentAt, m.sentAt);
            const showAuthor = m.direction === "out" && (!prev || newDay || prev.direction !== "out" || prev.authorName !== m.authorName);
            return (
              <Fragment key={m.id}>
                {newDay && (
                  <div className="my-3 flex justify-center">
                    <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] text-fg/75 shadow-sm">
                      {formatDayDivider(m.sentAt)}
                    </span>
                  </div>
                )}
                <div className={cn(showAuthor || (prev && prev.direction !== m.direction) ? "mt-1.5" : "")}>
                  <MessageBubble m={m} showAuthor={showAuthor} animateIn={new Date(m.sentAt).getTime() > openedAt} />
                </div>
                {m.id === lastOutId && !search && (
                  <ReceiptLine
                    m={m}
                    onRetry={() => send.mutate({ msg: m })}
                  />
                )}
              </Fragment>
            );
          })}
        </motion.div>
      </div>
      <AnimatePresence>
        {showJump && (
          <motion.button
            type="button"
            aria-label="Ir para a mensagem mais recente"
            onClick={() => scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" })}
            initial={{ opacity: 0, scale: 0.8, y: 8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.8, y: 8 }}
            className="absolute bottom-4 right-5 inline-flex size-10 items-center justify-center rounded-full border border-line bg-elevated text-muted shadow-lg hover:text-fg"
          >
            <ArrowDown className="size-5" />
          </motion.button>
        )}
      </AnimatePresence>
      </div>

      {send.isError && (
        <div className="border-t border-line bg-panel px-3 pt-2">
          <Alert>
            <span className="flex items-start justify-between gap-3">
              {errorMessage(send.error)}
              <button type="button" className="shrink-0 font-medium underline-offset-2 hover:underline" onClick={() => send.reset()}>
                Fechar
              </button>
            </span>
          </Alert>
        </div>
      )}
      {!lockedByOther && !closed && (
        <div className="border-t border-line bg-panel px-4 py-1 text-[11.5px] text-muted">{banner}</div>
      )}
      <Composer
        key={`${c.id}:${composerKey}`}
        conversationId={c.id}
        disabled={lockedByOther || closed || !capabilities.send}
        disabledReason={
          !capabilities.send
            ? capabilities.sendUnavailableReason
            : closed
              ? "Atendimento encerrado. Uma nova mensagem do cliente abre um novo atendimento."
              : lockedByOther
                ? `${c.assignee?.name} está atendendo esta conversa.${canClose ? "" : ""}`
                : undefined
        }
        mediaDisabledReason={capabilities.media ? undefined : capabilities.mediaUnavailableReason}
        onSend={submit}
      />
    </section>
  );
}

/** Recibo sob a última mensagem enviada: "Lida às 08:53", "Entregue às 08:51"… */
function ReceiptLine({ m, onRetry }: { m: Message; onRetry: () => void }) {
  const status = m.status ?? "sent";
  let content: React.ReactNode;
  if (status === "failed") {
    content = (
      <span className="text-danger">
        Não enviada ·{" "}
        <button type="button" onClick={onRetry} className="font-medium underline-offset-2 hover:underline">
          Tentar de novo
        </button>
      </span>
    );
  } else if (status === "read" && m.readAt) {
    content = (
      <span className="inline-flex items-center gap-1 text-read">
        <CheckCheck className="size-3.5" /> Lida às {formatTime(m.readAt)}
      </span>
    );
  } else if (status === "delivered" && m.deliveredAt) {
    content = <>Entregue às {formatTime(m.deliveredAt)}</>;
  } else {
    content = <>{statusLabel[status]}{status === "pending" ? "…" : ""}</>;
  }

  return (
    <div className="flex justify-end pr-1" aria-live="polite">
      <AnimatePresence mode="wait" initial={false}>
        <motion.span
          key={status}
          className="text-[11px] text-muted"
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 4 }}
          transition={{ duration: 0.18 }}
        >
          {content}
        </motion.span>
      </AnimatePresence>
    </div>
  );
}
