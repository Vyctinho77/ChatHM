import { Fragment, Suspense, lazy, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ArrowDown, ArrowLeft, CheckCheck, ChevronDown, ChevronUp, CircleCheck, Clock3, EllipsisVertical, Search, UserRoundPlus, UsersRound, X } from "lucide-react";
import { useSearchParams } from "react-router";
import { fold, matches } from "@/lib/search";
import { ViewableAvatar } from "@/components/AvatarViewer";
import { Menu, MenuItem } from "@/components/Menu";
import { Alert, IconButton } from "@/components/ui";
import { ChatSkeleton } from "@/components/Skeleton";
import { errorMessage, isRejection } from "@/lib/api";
import { canSupervise, useMe } from "@/features/auth/api";
import { AssigneeChip } from "./AssigneeChip";
import { DropOverlay } from "./DropOverlay";
import { useFileDrop } from "./useFileDrop";
import { cn } from "@/lib/cn";
import { formatDayDivider, formatTime, sameDay } from "@/lib/format";
import { Composer } from "./Composer";
import { getDraft, setDraft } from "./drafts";
import { ImageViewer, type ViewableImage } from "./ImageViewer";
import { MessageBubble } from "./MessageBubble";
import type { ViewablePdf } from "./PdfViewer";

// pdf.js é pesado: só é baixado quando alguém abre um PDF.
const PdfViewer = lazy(() => import("./PdfViewer").then((mod) => ({ default: mod.PdfViewer })));
import { TransferMenu } from "./TransferMenu";
import { endHint, endLabel } from "./closeOptions";
import { useEndFlow } from "./useEndFlow";
import { useTransferFlow } from "./transferFlow";
import { DEMO } from "@/lib/env";
import { AiSuggestions } from "./AiSuggestions";
import { DeleteDialog, ForwardDialog, MessageInfoDialog, MessageMenu } from "./MessageActions";
import { buildOutgoing, capabilities, useConversationActions, useMessages, useSendMessage, type OutgoingDraft } from "./store";
import { statusLabel } from "./receipts";
import { segmentClass, segmentLabel, type Conversation, type Message, type Quoted } from "./types";
import { SnoozeDialog, SnoozeNotice } from "./SnoozeDialog";
import { useTyping } from "./typing";
import { TypingBubble, TypingIndicator } from "./TypingIndicator";

const MESSAGE_PAUSE_MS = 30 * 60 * 1000;

/** Quantas mensagens recentes aparecem de início (e a cada rolagem para cima). */
const WINDOW = 40;

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
  const typing = useTyping(c.id);
  const contactTitle = c.contact.kind === "group" || c.contact.saved ? c.contact.name : c.contact.phone || c.contact.name;
  const send = useSendMessage(c.id);
  const { data: me } = useMe();
  const isAdmin = canSupervise(me?.user.role); // administrador ou gestor
  const [snoozeOpen, setSnoozeOpen] = useState(false);
  const [incoming, setIncoming] = useState<{ id: number; files: File[] } | null>(null);
  const [dropProblem, setDropProblem] = useState("");
  const [statusExpanded, setStatusExpanded] = useState(true);
  // Muda ao devolver um texto recusado ao campo de mensagem (remonta o Composer).
  const [composerKey, setComposerKey] = useState(0);
  const scroller = useRef<HTMLDivElement>(null);
  const startIdxRef = useRef(0);
  const [search, setSearch] = useState<string | null>(null);
  // Posição escolhida na lista de ocorrências; vale só para o termo atual (ao digitar de novo volta para a mais recente).
  const [nav, setNav] = useState<{ term: string; idx: number }>({ term: "", idx: -1 });
  const [flashDone, setFlashDone] = useState<string | null>(null);
  const [params] = useSearchParams();
  const targetMsg = params.get("msg");
  const handledTarget = useRef<string | null>(null);
  const loadedAll = useRef(false);
  // Mensagens com horário posterior à abertura da conversa entram animadas.
  // (O componente é remontado a cada conversa: ver key no ConversationsPage.)
  const [openedAt] = useState(() => Date.now());
  const atBottom = useRef(true);
  const scrolledOnce = useRef(false);
  const [showJump, setShowJump] = useState(false);
  // Última mensagem vista com a rolagem no fim: o que chegou depois, com a pessoa lendo acima, vira o aviso "novas mensagens".
  const [seenId, setSeenId] = useState<string | null>(null);
  // Imagem aberta no visualizador em tela cheia (navega por todas as imagens da conversa).
  const [viewerId, setViewerId] = useState<string | null>(null);
  const closeViewer = useCallback(() => setViewerId(null), []);
  const [pdfId, setPdfId] = useState<string | null>(null);
  const closePdf = useCallback(() => setPdfId(null), []);
  const openPdf = (messages ?? []).find((m): m is ViewablePdf => m.id === pdfId && m.kind === "document" && !!m.url);
  const viewable = useMemo(
    () => (messages ?? []).filter((m): m is ViewableImage => m.kind === "image" && !!m.url),
    [messages],
  );

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

  // A bolha de "digitando" aparece no fim: acompanha se a pessoa já está lendo o fim da conversa.
  useEffect(() => {
    if (typing && atBottom.current) scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });
  }, [typing]);

  function onScroll() {
    const el = scroller.current;
    if (!el) return;
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
    if (el.scrollTop < 500 && startIdxRef.current > 0 && !anchorHeight.current) {
      anchorHeight.current = el.scrollHeight;
      setWindowSize((n) => n + WINDOW);
    }
    atBottom.current = near;
    setShowJump(!near);
    if (near) setSeenId(messages?.[messages.length - 1]?.id ?? null);
  }

  const newCount = useMemo(() => {
    if (!showJump || !seenId || !messages) return 0;
    const i = messages.findIndex((m) => m.id === seenId);
    return i < 0 ? 0 : messages.slice(i + 1).filter((m) => m.direction === "in").length;
  }, [showJump, seenId, messages]);
  const jumpToLatest = () => scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: "smooth" });

  const lastOutId = useMemo(() => {
    for (let i = (messages?.length ?? 0) - 1; i >= 0; i--) {
      const m = messages![i]!;
      if (m.direction === "out") return m.id;
    }
    return undefined;
  }, [messages]);

  // A busca destaca e navega entre as ocorrências, sem esconder o resto da conversa.
  const term = search?.trim() ?? "";
  // Só as mensagens mais recentes entram na tela de início: montar centenas de balões é o que fazia a conversa demorar.
  // Rolar para cima mostra mais; busca, link para uma mensagem e "ir até a original" mostram tudo.
  const [windowSize, setWindowSize] = useState(WINDOW);
  const [expanded, setExpanded] = useState(false);
  const all = messages ?? [];
  const renderAll = expanded || term.length > 0 || !!targetMsg;
  const startIdx = renderAll ? 0 : Math.max(0, all.length - windowSize);
  const shown = startIdx > 0 ? all.slice(startIdx) : all;
  const anchorHeight = useRef(0);
  useLayoutEffect(() => {
    startIdxRef.current = startIdx;
  });
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && anchorHeight.current) {
      el.scrollTop += el.scrollHeight - anchorHeight.current; // mantém na tela o que a pessoa estava lendo
      anchorHeight.current = 0;
    }
  }, [windowSize]);
  const matchIds = useMemo(() => {
    if (term.length < 1 || !messages) return [];
    return messages
      .filter((m) => {
        const text = m.kind === "text" ? m.text : m.kind === "document" ? (m.fileName ?? "") : m.kind === "image" ? (m.caption ?? "") : m.kind === "unsupported" ? (m.text ?? "") : "";
        return !!text && (fold(text).includes(fold(term)) || matches(text, term));
      })
      .map((m) => m.id);
  }, [messages, term]);
  const matchIdx = matchIds.length ? (nav.term === term && nav.idx >= 0 ? Math.min(nav.idx, matchIds.length - 1) : matchIds.length - 1) : 0;
  const currentMatch = matchIds.length ? matchIds[matchIdx] : undefined;

  // Menu de botão direito e ações das mensagens.
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);
  const [reply, setReply] = useState<Message | null>(null);
  const [editing, setEditing] = useState<Message | null>(null);
  const [deleting, setDeleting] = useState<Message | null>(null);
  const [forwarding, setForwarding] = useState<Message | null>(null);
  const [infoFor, setInfoFor] = useState<Message | null>(null);
  const menuMsg = menu ? messages?.find((m) => m.id === menu.id) : undefined;
  const openMenu = useCallback((m: Message, x: number, y: number) => setMenu({ id: m.id, x, y }), []);
  const closeMenu = useCallback(() => setMenu(null), []);

  const scrollToMessage = useCallback((id: string) => {
    requestAnimationFrame(() => document.getElementById(`msg-${id}`)?.scrollIntoView({ block: "center", behavior: "smooth" }));
  }, []);

  useEffect(() => {
    if (currentMatch) scrollToMessage(currentMatch);
  }, [currentMatch, scrollToMessage]);

  function step(dir: 1 | -1) {
    if (!matchIds.length) return;
    setNav({ term, idx: (matchIdx + dir + matchIds.length) % matchIds.length });
  }

  // Link para uma mensagem (?msg=): rola até ela e a destaca por alguns segundos; se estiver além das mais recentes,
  // carrega o histórico todo.
  const targetLoaded = !!targetMsg && !!messages?.some((m) => m.id === targetMsg);
  const flashId = targetLoaded && flashDone !== targetMsg ? targetMsg : null;
  useEffect(() => {
    if (!targetMsg || !messages || handledTarget.current === targetMsg) return;
    if (targetLoaded) {
      handledTarget.current = targetMsg;
      scrollToMessage(targetMsg);
      const t = setTimeout(() => setFlashDone(targetMsg), 3500);
      return () => clearTimeout(t);
    }
    if (!loadedAll.current) {
      loadedAll.current = true;
      void actions.loadAllMessages(c.id);
    }
  }, [targetMsg, messages, targetLoaded, actions, c.id, scrollToMessage]);

  const mine = c.assignee?.id === actions.me.id;
  const closed = c.status === "closed";
  // Outra pessoa já assumiu: só ela responde (o servidor também garante isso).
  const participating = !!c.participants?.some((p) => p.id === actions.me.id); // incluída no atendimento: responde sem assumir
  const lockedByOther = !!c.assignee && !mine && !participating && !closed;
  // O responsável encerra; administradores e gestores também encerram qualquer atendimento que enxergam.
  const canClose = capabilities.close && !closed && (mine || isAdmin);
  // Encerrar: para todos ou só a própria parte (ver closeOptions e useEndFlow).
  const endFlow = useEndFlow(c, actions, isAdmin);
  const { end, canEnd, startEnd } = endFlow;
  const showAttendanceStatus = !lockedByOther && !closed && !c.snoozedUntil;

  const canTransfer = !closed && (mine || isAdmin || !c.assignee);
  const [transferAt, setTransferAt] = useState<{ x: number; y: number } | null>(null);
  const transferFlow = useTransferFlow(actions);
  const [participantsAt, setParticipantsAt] = useState<{ x: number; y: number } | null>(null);
  const canManageParticipants = !closed && (mine || participating || isAdmin);
  // Por que não dá para enviar arquivos agora (ou null): o mesmo que o campo de mensagem avisa.
  const dropBlocked = !capabilities.send
    ? (capabilities.sendUnavailableReason ?? "O envio está indisponível.")
    : closed
      ? "Atendimento encerrado."
      : lockedByOther
        ? `${c.assignee?.name} está atendendo esta conversa.`
        : c.snoozedUntil
          ? "Atendimento adiado. Retome para responder."
          : !capabilities.media
            ? (capabilities.mediaUnavailableReason ?? "O envio de arquivos está indisponível.")
            : null;
  const { dragging, handlers: dropHandlers } = useFileDrop({
    blocked: dropBlocked,
    onFiles: (files, problem) => {
      setDropProblem(problem ?? "");
      if (files.length) setIncoming((prev) => ({ id: (prev?.id ?? 0) + 1, files }));
    },
  });
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
      </span>
    );
  } else if (mine) {
    banner = <>Você está atendendo</>;
  } else {
    banner = <>Em atendimento por {c.assignee.name}</>;
  }

  /** Prévia usada para citar a mensagem numa resposta. */
  function quotedOf(m: Message): Quoted {
    const text = m.kind === "text" ? m.text : m.kind === "document" ? (m.caption || m.fileName || "Documento") : m.kind === "image" ? (m.caption ?? "") : m.kind === "sticker" ? "Figurinha" : "";
    const out = m.direction === "out";
    return { id: m.id, author: out ? (m.authorName ?? actions.me.name) : (m.authorName ?? c.contact.name), fromMe: out, kind: m.kind, text: text.slice(0, 160) };
  }

  /** Vai até a mensagem original de uma resposta (carrega o histórico se ela estiver além das mais recentes). */
  async function jumpTo(id: string) {
    if (!document.getElementById(`msg-${id}`)) {
      setExpanded(true);
      await actions.loadAllMessages(c.id);
      await new Promise<void>((r) => requestAnimationFrame(() => r()));
    }
    scrollToMessage(id);
  }

  // Vários anexos de uma vez: só o primeiro leva a citação.
  const replyConsumed = useRef<string | null>(null);
  const chooseReply = (m: Message | null) => {
    replyConsumed.current = null;
    setReply(m);
  };

  function submit(d: OutgoingDraft) {
    const quote = reply && replyConsumed.current !== reply.id ? quotedOf(reply) : undefined;
    if (reply) replyConsumed.current = reply.id;
    const msg = buildOutgoing(c.id, actions.me.name, d, quote);
    setReply(null);
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
    <section aria-label={`Conversa com ${c.contact.name}`} className="chat-wallpaper relative flex h-full min-w-0 flex-col" {...dropHandlers}>
      {dragging && <DropOverlay blocked={dropBlocked} />}
      <header className="relative flex h-14 shrink-0 items-center gap-1.5 border-b border-line bg-panel px-2.5">
        {onBack && (
          <IconButton label="Voltar para a lista" onClick={onBack}>
            <ArrowLeft className="size-5" />
          </IconButton>
        )}
        <div className="flex min-w-0 items-center gap-1 pl-1">
        <ViewableAvatar name={c.contact.name} src={c.contact.avatarUrl} group={c.contact.kind === "group"} />
        <button
          type="button"
          onClick={() => { if (!contactOpen) onToggleContact(); }}
          aria-expanded={contactOpen}
          aria-label="Ver dados do contato"
          className="flex min-w-0 items-center gap-2.5 rounded-lg py-1 pl-1.5 pr-2.5 text-left hover:bg-hover"
        >
          <span className="min-w-0">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate text-[13.5px] font-semibold">{contactTitle}</span>
              <span
                title={
                  c.classification === "conflict"
                    ? `${segmentLabel(c)}: a mensagem combinou com mais de um segmento, sem uma equipe definida`
                    : c.segment === "geral"
                      ? `${segmentLabel(c)}: nenhuma equipe identificada para este atendimento`
                      : undefined
                }
                className={cn(
                  "shrink-0 rounded-full px-1.5 py-px text-[10.5px] font-medium leading-4",
                  segmentClass(c.segment),
                )}
              >
                {segmentLabel(c)}
              </span>
              {c.assignee && !closed && <AssigneeChip name={c.assignee.name} mine={mine} className="max-w-[9rem]" />}
              {!!c.participants?.length && !closed && (
                <span title={`Também no atendimento: ${c.participants.map((p) => p.name).join(", ")}`} className="max-w-[10rem] shrink-0 truncate text-[11px] text-muted">
                  + {c.participants.map((p) => (p.id === actions.me.id ? "você" : p.name.trim().split(/\s+/)[0])).join(", ")}
                </span>
              )}
            </span>
            {typing && <TypingIndicator announce className="text-[11.5px]" />}
          </span>
        </button>
        </div>
        <div className="ml-auto flex items-center">
          {showAttendanceStatus && !statusExpanded && (
            <IconButton label="Mostrar status do atendimento" aria-expanded={false} onClick={() => setStatusExpanded(true)}>
              <ChevronDown className="size-4" />
            </IconButton>
          )}
          {!closed && (
            <IconButton
              label="Transferir conversa"
              onClick={(e) => {
                const r = e.currentTarget.getBoundingClientRect();
                setTransferAt({ x: r.right - 256, y: r.bottom + 4 });
              }}
            >
              <UserRoundPlus className="size-5" />
            </IconButton>
          )}
          {!closed && (
            <IconButton
              label="Participantes do atendimento"
              active={!!c.participants?.length}
              onClick={(e) => {
                const r = e.currentTarget.getBoundingClientRect();
                setParticipantsAt({ x: r.right - 256, y: r.bottom + 4 });
              }}
            >
              <UsersRound className="size-5" />
            </IconButton>
          )}
          {canEnd && (end.actions.length === 1 ? (
            <IconButton label={end.direct ? "Encerrar atendimento" : endLabel[end.actions[0]!]} onClick={() => startEnd(end.actions[0]!)}>
              <CircleCheck className="size-5" />
            </IconButton>
          ) : (
            <Menu
              className="w-72"
              trigger={({ toggle }) => (
                <IconButton label="Encerrar atendimento" onClick={toggle}>
                  <CircleCheck className="size-5" />
                </IconButton>
              )}
            >
              {(close) => end.actions.map((kind) => (
                <MenuItem key={kind} icon={<CircleCheck />} onClick={() => (close(), startEnd(kind))}>
                  <span className="min-w-0">
                    <span className="block">{endLabel[kind]}</span>
                    <span className="block text-[11px] font-normal text-muted">{endHint(kind, end.others)}</span>
                  </span>
                </MenuItem>
              ))}
            </Menu>
          ))}
          {canClose && (
            <IconButton
              label={c.snoozedUntil ? "Alterar adiamento" : "Adiar atendimento"}
              active={!!c.snoozedUntil || snoozeOpen}
              onClick={() => setSnoozeOpen(true)}
            >
              <Clock3 className="size-5" />
            </IconButton>
          )}
          <IconButton
            label="Pesquisar na conversa"
            active={search !== null}
            onClick={() => setSearch((s) => (s === null ? "" : null))}
          >
            <Search className="size-5" />
          </IconButton>
          <Menu
            trigger={({ toggle }) => (
              <IconButton label="Mais opções" onClick={toggle}>
                <EllipsisVertical className="size-5" />
              </IconButton>
            )}
          >
            {(close) => (
              <>
                <MenuItem
                  icon={<UserRoundPlus />}
                  disabled={closed}
                  onClick={() => {
                    close();
                    setTransferAt({ x: window.innerWidth - 280, y: 56 });
                  }}
                >
                  Transferir…
                </MenuItem>
                <MenuItem
                  icon={<UsersRound />}
                  disabled={closed}
                  onClick={() => {
                    close();
                    setParticipantsAt({ x: window.innerWidth - 280, y: 56 });
                  }}
                >
                  Incluir pessoa no atendimento…
                </MenuItem>
                {canEnd ? end.actions.map((kind) => (
                  <MenuItem key={kind} icon={<CircleCheck />} onClick={() => (close(), startEnd(kind))}>
                    {end.direct ? "Encerrar atendimento" : endLabel[kind]}
                  </MenuItem>
                )) : (
                  <MenuItem icon={<CircleCheck />} disabled>
                    Encerrar atendimento
                  </MenuItem>
                )}
              </>
            )}
          </Menu>
        </div>
        <AnimatePresence initial={false}>
          {showAttendanceStatus && statusExpanded && (
            <motion.button
              key="attendance-status"
              type="button"
              aria-label="Recolher status do atendimento"
              aria-expanded={true}
              onClick={() => setStatusExpanded(false)}
              initial={{ opacity: 0, y: -10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              transition={{ duration: 0.16 }}
              className="absolute right-3 top-full z-20 mt-2 flex min-h-9 max-w-[min(20rem,calc(100%-1.5rem))] items-center justify-between gap-3 rounded-lg border border-line bg-elevated px-3 py-2 text-left text-[11.5px] leading-4 text-muted shadow-sm hover:bg-hover"
            >
              <span aria-hidden className="absolute -top-1.5 right-8 size-2.5 rotate-45 border-l border-t border-line bg-elevated" />
              <span className={cn("relative", mine && "font-medium text-brand-ink")}>{banner}</span>
              <ChevronDown aria-hidden className="size-3.5 shrink-0" />
            </motion.button>
          )}
        </AnimatePresence>
      </header>

      {c.snoozedUntil && <SnoozeNotice conversation={c} canManage={canClose} onEdit={() => setSnoozeOpen(true)} />}
      {snoozeOpen && <SnoozeDialog conversation={c} onClose={() => setSnoozeOpen(false)} />}

      {search !== null && (
        <div className="flex items-center gap-2 border-b border-line bg-panel px-4 py-2">
          <Search className="size-4 text-muted" />
          <input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") setSearch(null);
              if (e.key === "Enter") {
                e.preventDefault();
                step(e.shiftKey ? -1 : 1);
              }
            }}
            placeholder="Pesquisar mensagens"
            aria-label="Pesquisar mensagens"
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          {term && (
            <>
              <span className="text-xs tabular-nums text-muted">
                {matchIds.length ? `${matchIdx + 1} de ${matchIds.length}` : "Nenhum resultado"}
              </span>
              <IconButton label="Resultado anterior" onClick={() => step(-1)} disabled={matchIds.length < 2} className="size-7">
                <ChevronUp className="size-4" />
              </IconButton>
              <IconButton label="Próximo resultado" onClick={() => step(1)} disabled={matchIds.length < 2} className="size-7">
                <ChevronDown className="size-4" />
              </IconButton>
            </>
          )}
          <IconButton label="Fechar pesquisa" onClick={() => setSearch(null)} className="size-7">
            <X className="size-4" />
          </IconButton>
        </div>
      )}

      <div className="relative min-h-0 flex-1">
      <div ref={scroller} onScroll={onScroll} className="h-full overflow-y-auto px-3 py-3 sm:px-4">
        {isPending && <ChatSkeleton />}
        <motion.div
          className="flex flex-col gap-2"
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: messages ? 1 : 0, y: messages ? 0 : 8 }}
          transition={{ duration: 0.25, ease: "easeOut" }}
        >
          {shown.map((m, i) => {
            const prev = shown[i - 1];
            const newDay = !prev || !sameDay(prev.sentAt, m.sentAt);
            const newAttendance = !!prev && prev.conversationId !== m.conversationId;
            const afterPause = !!prev && Date.parse(m.sentAt) - Date.parse(prev.sentAt) >= MESSAGE_PAUSE_MS;
            const startsGroup = !prev || newDay || newAttendance || afterPause || prev.direction !== m.direction || prev.authorName !== m.authorName || prev.kind === "sticker" || m.kind === "sticker";
            const showAuthor = (m.direction === "out" || !!m.group) && startsGroup;
            return (
              <Fragment key={m.id}>
                {newDay && (
                  <div className="my-3 flex justify-center">
                    <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] text-fg/75 shadow-sm">
                      {formatDayDivider(m.sentAt)}
                    </span>
                  </div>
                )}
                {newAttendance && (
                  <div className="my-3 flex justify-center">
                    <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] font-medium text-fg/75 shadow-sm">
                      Novo atendimento
                    </span>
                  </div>
                )}
                {afterPause && !newDay && !newAttendance && (
                  <div className="my-3 flex justify-center">
                    <span className="rounded-md bg-elevated px-2.5 py-0.5 text-[11px] tabular-nums text-fg/75 shadow-sm">
                      {formatTime(m.sentAt)}
                    </span>
                  </div>
                )}
                <div id={`msg-${m.id}`} className={cn(startsGroup && "mt-2")}>
                  <MessageBubble
                    m={m}
                    contact={c.contact}
                    me={actions.me.name}
                    showAuthor={showAuthor}
                    startsGroup={startsGroup}
                    animateIn={new Date(m.sentAt).getTime() > openedAt}
                    term={term}
                    emphasis={m.id === flashId || m.id === currentMatch}
                    onOpenImage={setViewerId}
                    onOpenPdf={setPdfId}
                    onMenu={openMenu}
                    onJump={(id) => void jumpTo(id)}
                  />
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
          {typing && !search && !closed && <TypingBubble />}
        </motion.div>
      </div>
      <AnimatePresence>
        {showJump && newCount > 0 && (
          <motion.button
            key="new-messages"
            type="button"
            onClick={jumpToLatest}
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 12 }}
            className="absolute bottom-4 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-accent px-3.5 py-1.5 text-[13px] font-semibold text-accent-fg shadow-lg hover:brightness-110"
          >
            <ArrowDown className="size-4" aria-hidden="true" />
            {newCount === 1 ? "1 nova mensagem" : `${newCount} novas mensagens`}
          </motion.button>
        )}
        {showJump && newCount === 0 && (
          <motion.button
            key="jump"
            type="button"
            aria-label="Ir para a mensagem mais recente"
            onClick={jumpToLatest}
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

      {dropProblem && (
        <div className="border-t border-line bg-panel px-3 pt-2">
          <Alert>
            <span className="flex items-start justify-between gap-3">
              {dropProblem}
              <button type="button" className="shrink-0 font-medium underline-offset-2 hover:underline" onClick={() => setDropProblem("")}>
                Fechar
              </button>
            </span>
          </Alert>
        </div>
      )}
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
      {!DEMO && (
        <AiSuggestions
          conversationId={c.id}
          canSend={!lockedByOther && !closed && !c.snoozedUntil && capabilities.send}
          sendBlockedReason={closed ? "Atendimento encerrado." : c.snoozedUntil ? "Atendimento adiado." : lockedByOther ? `${c.assignee?.name} está atendendo.` : capabilities.sendUnavailableReason}
          onUse={(text) => {
            setDraft(c.id, text);
            setComposerKey((k) => k + 1);
          }}
        />
      )}
      <Composer
        key={`${c.id}:${composerKey}:${editing?.id ?? ""}`}
        conversationId={c.id}
        reply={reply ? { author: quotedOf(reply).author, text: quotedOf(reply).text || reply.kind } : null}
        onCancelReply={() => setReply(null)}
        edit={editing && editing.kind === "text" ? { id: editing.id, text: editing.text } : null}
        onCancelEdit={() => setEditing(null)}
        onSaveEdit={async (t) => {
          if (!editing) return;
          await actions.editMessage(c.id, editing.id, t);
          setEditing(null);
        }}
        disabled={lockedByOther || closed || !!c.snoozedUntil || !capabilities.send}
        disabledReason={
          !capabilities.send
            ? capabilities.sendUnavailableReason
            : closed
              ? "Atendimento encerrado. Uma nova mensagem do cliente abre um novo atendimento."
              : lockedByOther
                ? `${c.assignee?.name} está atendendo esta conversa.${canClose ? "" : ""}`
                : c.snoozedUntil ? "Atendimento adiado. Retome para responder." : undefined
        }
        mediaDisabledReason={capabilities.media ? undefined : capabilities.mediaUnavailableReason}
        incoming={incoming}
        onSend={submit}
      />
      {menu && menuMsg && (
        <MessageMenu
          message={menuMsg}
          x={menu.x}
          y={menu.y}
          onReply={() => (setEditing(null), chooseReply(menuMsg))}
          onForward={() => setForwarding(menuMsg)}
          onEdit={() => (setReply(null), setEditing(menuMsg))}
          onDelete={() => setDeleting(menuMsg)}
          onInfo={() => setInfoFor(menuMsg)}
          onClose={closeMenu}
        />
      )}
      {deleting && <DeleteDialog message={deleting} conversationId={c.id} onClose={() => setDeleting(null)} />}
      {forwarding && <ForwardDialog message={forwarding} conversationId={c.id} onClose={() => setForwarding(null)} />}
      {infoFor && <MessageInfoDialog message={infoFor} onClose={() => setInfoFor(null)} />}
      {viewerId && (
        <ImageViewer images={viewable} startId={viewerId} contactName={c.contact.name} onClose={closeViewer} />
      )}
      {openPdf && (
        <Suspense fallback={null}>
          <PdfViewer pdf={openPdf} onClose={closePdf} />
        </Suspense>
      )}
      {participantsAt && (
        <TransferMenu
          variant="participants"
          conversation={c}
          x={participantsAt.x}
          y={participantsAt.y}
          myId={actions.me.id}
          canTransfer={canManageParticipants}
          onPick={(p) => actions.toggleParticipant(c.id, p)}
          onClose={() => setParticipantsAt(null)}
        />
      )}
      {transferAt && (
        <TransferMenu
          conversation={c}
          x={transferAt.x}
          y={transferAt.y}
          myId={actions.me.id}
          canTransfer={canTransfer}
          onPick={(p) => transferFlow.request(c, p)}
          onClose={() => setTransferAt(null)}
        />
      )}
      {transferFlow.dialog}
      {endFlow.dialog}
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
  } else if (m.kind === "audio" && m.playedAt) {
    content = (
      <span className="inline-flex items-center gap-1 text-read">
        <CheckCheck className="size-3.5" /> Ouvida às {formatTime(m.playedAt)}
      </span>
    );
  } else if (status === "read" && m.readAt) {
    content = (
      <span className="inline-flex items-center gap-1 text-read">
        <CheckCheck className="size-3.5" /> Lida às {formatTime(m.readAt)}{m.kind === "audio" ? " · ainda não ouvida" : ""}
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
