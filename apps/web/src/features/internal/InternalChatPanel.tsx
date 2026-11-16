import { Fragment, Suspense, lazy, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ArrowDown, ArrowLeft, Bell, BellOff, MessagesSquare, Share2 } from "lucide-react";
import { Alert, IconButton } from "@/components/ui";
import { ChatSkeleton } from "@/components/Skeleton";
import { errorMessage, isRejection } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { updateSoundPreferences, useSoundPreferences } from "@/lib/sounds";
import { cn } from "@/lib/cn";
import { formatDayDivider, formatTime, sameDay } from "@/lib/format";
import { canSupervise, useMe } from "@/features/auth/api";
import { usePeople } from "@/features/team/api";
import { Composer } from "@/features/conversations/Composer";
import { DropOverlay } from "@/features/conversations/DropOverlay";
import { getDraft, setDraft } from "@/features/conversations/drafts";
import { ImageViewer, type ViewableImage } from "@/features/conversations/ImageViewer";
import { MessageBubble } from "@/features/conversations/MessageBubble";
import { MessageMenu } from "@/features/conversations/MessageActions";
import type { ViewablePdf } from "@/features/conversations/PdfViewer";
import { useFileDrop } from "@/features/conversations/useFileDrop";
import type { MentionPerson } from "@/features/conversations/mentions";
import type { OutgoingDraft } from "@/features/conversations/store";
import type { Message, Quoted } from "@/features/conversations/types";
import { AuthorLine } from "./AuthorLine";
import { InternalDeleteDialog } from "./DeleteDialog";
import { MembersDialog } from "./MembersDialog";
import { MentionText } from "./MentionText";
import { roleLabel } from "./roles";
import { ShareConversationDialog } from "./ShareConversationDialog";
import { SharedConversationCard } from "./SharedConversationCard";
import { internalCapabilities } from "./caps";
import {
  buildInternalOutgoing,
  buildSharedConversationMessage,
  useEditInternal,
  useInternalMessages,
  useInternalRoom,
  useLoadOlder,
  useMarkInternalRead,
  useSendInternal,
  type InternalMessage,
} from "./api";

// pdf.js é pesado: só é baixado quando alguém abre um PDF.
const PdfViewer = lazy(() => import("@/features/conversations/PdfViewer").then((mod) => ({ default: mod.PdfViewer })));

/** Chave do rascunho do campo de mensagem deste chat. */
const DRAFT_ID = "internal-chat";
const MESSAGE_PAUSE_MS = 30 * 60 * 1000;

/** Prévia usada para citar a mensagem numa resposta. */
function quotedOf(m: Message): Quoted {
  const text = m.kind === "text" ? m.text : m.kind === "document" ? (m.caption || m.fileName || "Documento") : m.kind === "image" ? (m.caption ?? "") : m.kind === "conversation" ? "Conversa de cliente" : "";
  return { id: m.id, author: m.authorName ?? "", fromMe: m.direction === "out", kind: m.kind, text: text.slice(0, 160) };
}

const renderAuthor = (m: Message) => <AuthorLine m={m} />;

/**
 * O chat interno do escritório. Ocupa o painel das conversas (a lista de contatos continua ao lado) e some ao sair da aba:
 * voltar a Conversas mostra o painel vazio de sempre.
 */
export function InternalChatPanel({ onBack }: { onBack?: () => void }) {
  const { data: me } = useMe();
  const myId = me?.user.id ?? "";
  const canModerate = canSupervise(me?.user.role);
  const { data: page, isPending, isError, refetch } = useInternalMessages();
  const { data: room } = useInternalRoom();
  const { data: people } = usePeople();
  const messages = page?.messages;
  const send = useSendInternal();
  const edit = useEditInternal();
  const { mutate: markRead } = useMarkInternalRead();
  const loadOlder = useLoadOlder();

  const scroller = useRef<HTMLDivElement>(null);
  const atBottomRef = useRef(true);
  const [atBottom, setAtBottom] = useState(true);
  const opened = useRef(false);
  const anchor = useRef<{ height: number; top: number } | null>(null);
  const loadingOlder = useRef(false);
  const [loadingOlderUi, setLoadingOlderUi] = useState(false);
  const [openedAt] = useState(() => Date.now()); // mensagens posteriores à abertura entram animadas
  const [visible, setVisible] = useState(() => typeof document === "undefined" || document.visibilityState === "visible");

  const [reply, setReply] = useState<InternalMessage | null>(null);
  const replyConsumed = useRef<string | null>(null);
  const [editing, setEditing] = useState<InternalMessage | null>(null);
  const [composerKey, setComposerKey] = useState(0);
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);
  const [deleting, setDeleting] = useState<InternalMessage | null>(null);
  const [membersOpen, setMembersOpen] = useState(false);
  const [sharing, setSharing] = useState(false);
  const sound = useSoundPreferences();
  const [flashId, setFlashId] = useState<string | null>(null);
  const [viewerId, setViewerId] = useState<string | null>(null);
  const [pdfId, setPdfId] = useState<string | null>(null);
  const closeViewer = useCallback(() => setViewerId(null), []);
  const closePdf = useCallback(() => setPdfId(null), []);
  const closeMenu = useCallback(() => setMenu(null), []);
  const [incoming, setIncoming] = useState<{ id: number; files: File[] } | null>(null);
  const [dropProblem, setDropProblem] = useState("");
  const { dragging, handlers: dropHandlers } = useFileDrop({
    blocked: null,
    onFiles: (files, problem) => {
      setDropProblem(problem ?? "");
      if (files.length) setIncoming((prev) => ({ id: (prev?.id ?? 0) + 1, files }));
    },
  });

  const unread = room?.unread ?? 0;
  const mentions = room?.mentions ?? 0;
  // Quem pode ser mencionado: o escritório todo, com perfil e equipes na lista.
  const mentionPeople = useMemo<MentionPerson[]>(() => (people ?? []).map((p) => ({ id: p.id, name: p.name, subtitle: [roleLabel[p.role], ...p.teams].join(" · ") })), [people]);
  const viewable = useMemo(() => (messages ?? []).filter((m): m is ViewableImage & InternalMessage => m.kind === "image" && !!m.url), [messages]);
  const openPdf = (messages ?? []).find((m): m is ViewablePdf & InternalMessage => m.id === pdfId && m.kind === "document" && !!m.url);
  const menuMsg = menu ? messages?.find((m) => m.id === menu.id) : undefined;

  useEffect(() => {
    const onChange = () => setVisible(document.visibilityState === "visible");
    document.addEventListener("visibilitychange", onChange);
    return () => document.removeEventListener("visibilitychange", onChange);
  }, []);

  // Tudo o que chegou conta como lido quando a pessoa está olhando o fim da conversa.
  useEffect(() => {
    if (!unread || !atBottom || !visible || !messages) return;
    const t = window.setTimeout(() => markRead(), 400);
    return () => window.clearTimeout(t);
  }, [unread, atBottom, visible, messages, markRead]);

  // Ao abrir, vai direto ao fim; depois, rola quando chega mensagem (se a pessoa está no fim ou acabou de enviar).
  // Ao carregar mensagens anteriores, mantém na tela o que a pessoa estava lendo.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el || !messages?.length) return;
    if (anchor.current) {
      const a = anchor.current;
      anchor.current = null;
      el.scrollTop = a.top + (el.scrollHeight - a.height);
      return;
    }
    if (!opened.current) {
      el.scrollTop = el.scrollHeight;
      opened.current = true;
      return;
    }
    if (atBottomRef.current || messages[messages.length - 1]?.direction === "out") {
      el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
    }
  }, [messages]);

  async function older() {
    const el = scroller.current;
    if (!el || loadingOlder.current) return;
    loadingOlder.current = true;
    setLoadingOlderUi(true);
    anchor.current = { height: el.scrollHeight, top: el.scrollTop };
    try {
      if (!(await loadOlder())) anchor.current = null;
    } catch {
      anchor.current = null;
    } finally {
      loadingOlder.current = false;
      setLoadingOlderUi(false);
    }
  }

  function onScroll() {
    const el = scroller.current;
    if (!el) return;
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
    atBottomRef.current = near;
    setAtBottom(near);
    if (el.scrollTop < 300 && page?.hasMore) void older();
  }

  const jumpToLatest = () => scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });

  /** Vai até a mensagem original de uma resposta (se ela está entre as carregadas). */
  function jumpTo(id: string) {
    const el = document.getElementById(`msg-${id}`);
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    setFlashId(id);
    window.setTimeout(() => setFlashId((cur) => (cur === id ? null : cur)), 2500);
  }

  function submit(d: OutgoingDraft) {
    if (!me) return;
    const quote = reply && replyConsumed.current !== reply.id ? quotedOf(reply) : undefined;
    if (reply) replyConsumed.current = reply.id;
    const msg = buildInternalOutgoing(me.user, room?.id ?? DRAFT_ID, d, quote);
    setReply(null);
    send.mutate(
      { msg },
      {
        onError: (err) => {
          // O servidor recusou: o texto volta ao campo, sem perder o que foi digitado.
          if (d.kind === "text" && isRejection(err) && !getDraft(DRAFT_ID)) {
            setDraft(DRAFT_ID, d.text);
            setComposerKey((k) => k + 1);
          }
        },
      },
    );
  }

  function shareConversation(conversationId: string, comment: string) {
    if (!me) return;
    const quote = reply && replyConsumed.current !== reply.id ? quotedOf(reply) : undefined;
    if (reply) replyConsumed.current = reply.id;
    setReply(null);
    send.mutate({ msg: buildSharedConversationMessage(me.user, room?.id ?? DRAFT_ID, conversationId, comment, quote) });
  }

  const subtitle = people ? `Escritório · ${people.length} ${people.length === 1 ? "pessoa" : "pessoas"}` : "Escritório";

  return (
    <section aria-label="Chat interno do escritório" className="chat-wallpaper relative flex h-full min-w-0 flex-col" {...dropHandlers}>
      {dragging && <DropOverlay blocked={null} />}
      <header className="flex h-14 shrink-0 items-center gap-1.5 border-b border-line bg-panel px-2.5">
        {onBack && (
          <IconButton label="Voltar para a lista" onClick={onBack}>
            <ArrowLeft className="size-5" />
          </IconButton>
        )}
        <button
          type="button"
          onClick={() => setMembersOpen(true)}
          aria-label="Ver as pessoas do escritório"
          className="flex min-w-0 items-center gap-2.5 rounded-lg py-1 pl-2 pr-2.5 text-left hover:bg-hover"
        >
          <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-brand/15 text-brand-ink">
            <MessagesSquare className="size-5" aria-hidden />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-[13.5px] font-semibold">Chat interno</span>
            <span className="block truncate text-[11.5px] text-muted">{subtitle}</span>
          </span>
        </button>
        <IconButton
          label={sound.internalChat === "off" ? "Ativar os sons do chat interno" : "Silenciar os sons do chat interno"}
          aria-pressed={sound.internalChat === "off"}
          onClick={() => updateSoundPreferences({ internalChat: sound.internalChat === "off" ? "mentions" : "off" })}
          className="ml-auto"
        >
          {sound.internalChat === "off" ? <BellOff className="size-5" /> : <Bell className="size-5" />}
        </IconButton>
      </header>

      <div className="relative min-h-0 flex-1">
        <div ref={scroller} onScroll={onScroll} className="h-full overflow-y-auto px-3 py-3 sm:px-4">
          {isPending && <ChatSkeleton />}
          {isError && (
            <Alert>
              <span className="flex items-start justify-between gap-3">
                Não foi possível carregar o chat interno.
                <button type="button" className="shrink-0 font-medium underline-offset-2 hover:underline" onClick={() => void refetch()}>
                  Tentar de novo
                </button>
              </span>
            </Alert>
          )}
          {messages && messages.length === 0 && (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center text-muted">
              <MessagesSquare className="size-10 opacity-40" aria-hidden />
              <p className="text-base font-medium text-fg">Nenhuma mensagem ainda</p>
              <p className="max-w-xs text-sm">Aqui a equipe tira dúvidas. Escreva a primeira mensagem para todo o escritório.</p>
            </div>
          )}
          {page?.hasMore && (
            <div className="flex justify-center pb-2">
              <button
                type="button"
                disabled={loadingOlderUi}
                onClick={() => void older()}
                className="rounded-full bg-elevated px-3 py-1 text-[11.5px] text-fg/75 shadow-sm hover:bg-hover disabled:opacity-60"
              >
                {loadingOlderUi ? "Carregando…" : "Carregar mensagens anteriores"}
              </button>
            </div>
          )}
          <div role="log" aria-label="Mensagens do chat interno" className="flex flex-col gap-2">
            {messages?.map((m, i) => {
              const prev = messages[i - 1];
              const mine = m.direction === "out";
              const newDay = !prev || !sameDay(prev.sentAt, m.sentAt);
              const afterPause = !!prev && Date.parse(m.sentAt) - Date.parse(prev.sentAt) >= MESSAGE_PAUSE_MS;
              const startsGroup = !prev || newDay || afterPause || prev.authorId !== m.authorId;
              return (
                <Fragment key={m.id}>
                  {newDay && (
                    <div className="my-3 flex justify-center">
                      <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] text-fg/75 shadow-sm">{formatDayDivider(m.sentAt)}</span>
                    </div>
                  )}
                  {afterPause && !newDay && (
                    <div className="my-3 flex justify-center">
                      <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] tabular-nums text-fg/75 shadow-sm">{formatTime(m.sentAt)}</span>
                    </div>
                  )}
                  <div id={`msg-${m.id}`} className={cn(startsGroup && "mt-2")}>
                    <MessageBubble
                      m={m}
                      me={me?.user.name}
                      showAuthor={!mine && startsGroup}
                      startsGroup={startsGroup}
                      animateIn={new Date(m.sentAt).getTime() > openedAt}
                      emphasis={m.id === flashId}
                      renderAuthor={renderAuthor}
                      renderText={(t) => <MentionText m={t} myId={myId} />}
                      renderCard={(c) => <SharedConversationCard id={c.sharedConversationId} />}
                      onOpenImage={setViewerId}
                      onOpenPdf={setPdfId}
                      onMenu={(msg, x, y) => setMenu({ id: msg.id, x, y })}
                      onJump={jumpTo}
                    />
                    {m.status === "pending" && <p className="mt-0.5 pr-1 text-right text-[11px] text-muted">Enviando…</p>}
                    {m.status === "failed" && (
                      <p className="mt-0.5 pr-1 text-right text-[11px] text-danger">
                        Não enviada ·{" "}
                        <button type="button" onClick={() => send.mutate({ msg: m })} className="font-medium underline-offset-2 hover:underline">
                          Tentar de novo
                        </button>
                      </p>
                    )}
                  </div>
                </Fragment>
              );
            })}
          </div>
        </div>
        <AnimatePresence>
          {!atBottom && (
            <motion.button
              key={unread > 0 ? "new" : "jump"}
              type="button"
              aria-label={mentions > 0 ? "Ir para a menção" : unread > 0 ? "Ir para as mensagens novas" : "Ir para a mensagem mais recente"}
              onClick={jumpToLatest}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: 12 }}
              className={cn(
                "absolute bottom-4 inline-flex items-center justify-center shadow-lg",
                unread > 0
                  ? cn("left-1/2 -translate-x-1/2 gap-1.5 rounded-full px-3.5 py-1.5 text-[13px] font-semibold hover:brightness-110", mentions > 0 ? "bg-amber-400 text-amber-950" : "bg-accent text-accent-fg")
                  : "right-5 size-10 rounded-full border border-line bg-elevated text-muted hover:text-fg",
              )}
            >
              <ArrowDown className={unread > 0 ? "size-4" : "size-5"} aria-hidden />
              {unread > 0 && (mentions > 0 ? (mentions === 1 ? "@ 1 menção" : `@ ${mentions} menções`) : unread === 1 ? "1 nova mensagem" : `${unread} novas mensagens`)}
            </motion.button>
          )}
        </AnimatePresence>
      </div>

      {(dropProblem || send.isError) && (
        <div className="border-t border-line bg-panel px-3 pt-2">
          <Alert>
            <span className="flex items-start justify-between gap-3">
              {dropProblem || errorMessage(send.error)}
              <button
                type="button"
                className="shrink-0 font-medium underline-offset-2 hover:underline"
                onClick={() => (dropProblem ? setDropProblem("") : send.reset())}
              >
                Fechar
              </button>
            </span>
          </Alert>
        </div>
      )}
      <Composer
        key={`${composerKey}:${editing?.id ?? ""}`}
        conversationId={DRAFT_ID}
        reply={reply ? { author: quotedOf(reply).author, text: quotedOf(reply).text || reply.kind } : null}
        onCancelReply={() => setReply(null)}
        edit={editing && editing.kind === "text" ? { id: editing.id, text: editing.text } : null}
        onCancelEdit={() => setEditing(null)}
        onSaveEdit={async (t, mentionRefs) => {
          if (!editing) return;
          await edit.mutateAsync({ id: editing.id, text: t, mentions: mentionRefs });
          setEditing(null);
        }}
        mentionPeople={mentionPeople}
        myId={myId}
        // A demonstração não tem as conversas de clientes de verdade para compartilhar.
        attachExtras={DEMO ? undefined : [{ label: "Conversa de cliente", icon: <Share2 />, onClick: () => setSharing(true) }]}
        incoming={incoming}
        onSend={submit}
      />

      {menu && menuMsg && (
        <MessageMenu
          message={menuMsg}
          x={menu.x}
          y={menu.y}
          caps={internalCapabilities(menuMsg, { meId: myId, canModerate })}
          canDelete={internalCapabilities(menuMsg, { meId: myId, canModerate }).deleteEveryone}
          onReply={() => (setEditing(null), setReply(menuMsg), (replyConsumed.current = null))}
          onForward={() => undefined}
          onEdit={() => (setReply(null), setEditing(menuMsg))}
          onDelete={() => setDeleting(menuMsg)}
          onInfo={() => undefined}
          onClose={closeMenu}
        />
      )}
      {deleting && <InternalDeleteDialog message={deleting} mine={deleting.authorId === myId} onClose={() => setDeleting(null)} />}
      {membersOpen && <MembersDialog myId={myId} onClose={() => setMembersOpen(false)} />}
      {sharing && <ShareConversationDialog onShare={shareConversation} onClose={() => setSharing(false)} />}
      {viewerId && <ImageViewer images={viewable} startId={viewerId} contactName="Chat interno" onClose={closeViewer} />}
      {openPdf && (
        <Suspense fallback={null}>
          <PdfViewer pdf={openPdf} onClose={closePdf} />
        </Suspense>
      )}
    </section>
  );
}
