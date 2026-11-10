import { useRef, useState } from "react";
import { ProviderLogo } from "@/components/ProviderLogo";
import { providerLabel } from "@/lib/providers";
import { motion } from "motion/react";
import { Ban, Download, FileText, Image as ImageIcon, LoaderCircle, MapPin, Mic, Sticker } from "lucide-react";
import { cn } from "@/lib/cn";
import { previewUrl } from "@/lib/media";
import { formatBytes, formatTime } from "@/lib/format";
import { AudioPlayer } from "./AudioPlayer";
import { DocBadge } from "./DocBadge";
import { docKind, docKindLabel, officeApp } from "./docKind";
import { receiptDetails } from "./receipts";
import { Highlight } from "@/components/Highlight";
import { RichText } from "@/components/RichText";
import { FormattedText } from "@/components/FormattedText";
import { Ticks } from "./Ticks";
import type { Message, Quoted } from "./types";

function Meta({ m }: { m: Message }) {
  return (
    <span
      className="ml-auto flex shrink-0 items-center gap-1 self-end pl-2.5 text-[10.5px] tabular-nums opacity-70"
      title={m.direction === "out" ? receiptDetails(m) : undefined}
    >
      {m.editedAt && <span className="italic">editada</span>}
      {formatTime(m.sentAt)}
      {m.direction === "out" && <Ticks status={m.status} />}
    </span>
  );
}

const quotedKind: Record<string, { label: string; icon: React.ReactNode }> = {
  sticker: { label: "Figurinha", icon: <Sticker className="size-3.5" /> },
  image: { label: "Foto", icon: <ImageIcon className="size-3.5" /> },
  audio: { label: "Mensagem de voz", icon: <Mic className="size-3.5" /> },
  document: { label: "Documento", icon: <FileText className="size-3.5" /> },
  location: { label: "Localização", icon: <MapPin className="size-3.5" /> },
};

/** Prévia da mensagem respondida; clique leva até a original (quando ela está na conversa). */
function QuotedBlock({ q, onJump }: { q: Quoted; onJump?: (id: string) => void }) {
  const kind = quotedKind[q.kind];
  const body = (
    <>
      <span className="block truncate text-[12px] font-semibold text-brand-ink [.bg-out_&]:text-out-fg">{q.fromMe ? q.author || "Você" : q.author || "Cliente"}</span>
      <span className="flex items-center gap-1 text-[12.5px] opacity-80">
        {kind && q.kind !== "text" && <span className="shrink-0">{kind.icon}</span>}
        <span className="line-clamp-2 break-words"><FormattedText text={q.text || kind?.label || "Mensagem"} /></span>
      </span>
    </>
  );
  const cls = "mb-1 block w-full min-w-0 rounded-md border-l-4 border-brand bg-black/15 px-2 py-1 text-left";
  return q.id && onJump ? (
    <button type="button" aria-label="Ir para a mensagem respondida" onClick={() => onJump(q.id!)} className={cn(cls, "cursor-pointer hover:bg-black/25")}>
      {body}
    </button>
  ) : (
    <div className={cls}>{body}</div>
  );
}

/** Hora e status sobre a imagem, quando não há legenda abaixo dela. */
function MetaOverlay({ m }: { m: Message }) {
  return (
    <span
      className="absolute bottom-1.5 right-1.5 flex items-center gap-1 rounded-full bg-black/50 px-1.5 py-0.5 text-[10.5px] tabular-nums text-white"
      title={m.direction === "out" ? receiptDetails(m) : undefined}
    >
      {formatTime(m.sentAt)}
      {m.direction === "out" && <Ticks status={m.status} className="size-3.5" />}
    </span>
  );
}

/** Situação de um documento sem arquivo: histórico antigo (só metadados), limite de espaço, apagado no WhatsApp, bloqueado ou baixando. */
function historicalDocLabel(m: Message) {
  if (m.mediaState === "blocked") return "bloqueado por segurança";
  if (m.historical && m.mediaState === "failed") return "o WhatsApp não tem mais o arquivo";
  if (m.historical && m.mediaState === "skipped") return "histórico · não importado (limite de espaço)";
  if (m.historical) return "arquivo do histórico não importado";
  return m.mediaState === "failed" ? "download falhou" : "baixando…";
}

/** Texto do espaço de uma mídia cujo arquivo ainda não chegou ou não pode ser exibido. */
function mediaStateLabel(m: Message, what: string) {
  if (m.mediaState === "blocked") return `${what} bloqueada por segurança`;
  if (m.historical && m.mediaState === "failed") return `${what} do histórico · o WhatsApp não tem mais o arquivo`;
  if (m.historical && m.mediaState === "skipped") return `${what} do histórico · não importado (limite de espaço)`;
  if (m.historical) return `${what} do histórico · arquivo não importado`;
  if (m.mediaState === "failed") return `Não foi possível baixar ${what.toLowerCase() === "imagem" ? "a imagem" : "o arquivo"}`;
  return `${what} recebida · baixando…`;
}

function StickerBody({ m }: { m: Extract<Message, { kind: "sticker" }> }) {
  const [failedUrl, setFailedUrl] = useState<string>();
  const unavailable = !m.url || failedUrl === m.url;
  return (
    <div className="flex w-56 max-w-full flex-col gap-1">
      {unavailable ? (
        <div role="status" className="flex aspect-square flex-col items-center justify-center gap-2 rounded-xl bg-panel/80 px-4 text-center text-xs text-muted">
          {!m.url && !m.historical && !m.mediaState ? <LoaderCircle className="size-5 animate-spin motion-reduce:animate-none" /> : <Sticker className="size-8" />}
          <span>{failedUrl === m.url && m.url ? "Não foi possível exibir a figurinha" : mediaStateLabel(m, "Figurinha")}</span>
        </div>
      ) : (
        <img src={m.url} alt="Figurinha" draggable={false} onError={() => setFailedUrl(m.url)} className="aspect-square w-full object-contain" />
      )}
      <div className="flex justify-end">
        <span className={cn("flex rounded-full px-2 py-1 shadow-sm", m.direction === "out" ? "bg-out text-out-fg" : "bg-panel text-fg")}>
          <Meta m={m} />
        </span>
      </div>
    </div>
  );
}

/**
 * Como no WhatsApp: a miniatura minúscula (vem junto com a mensagem) aparece desfocada na hora, a versão média carrega só quando o
 * balão chega perto da tela e entra suavemente. O original só é baixado ao abrir a foto.
 */
function ProgressiveImage({ m }: { m: Extract<Message, { kind: "image" }> }) {
  const [loaded, setLoaded] = useState(false);
  const ratio = m.width && m.height ? m.width / m.height : undefined;
  return (
    <span className="relative block w-full overflow-hidden bg-black/10 dark:bg-black/25" style={ratio ? { aspectRatio: String(ratio), maxHeight: 320 } : { minHeight: 120, maxHeight: 320 }}>
      {m.thumb && <img src={m.thumb} alt="" aria-hidden="true" draggable={false} className="absolute inset-0 size-full scale-110 object-cover blur-md" />}
      <img
        ref={(el) => { if (el?.complete && el.naturalWidth > 0) setLoaded(true); }}
        src={previewUrl(m.url)}
        alt={m.caption ?? "Imagem"}
        width={m.width}
        height={m.height}
        loading="lazy"
        decoding="async"
        onLoad={() => setLoaded(true)}
        className={cn("relative block size-full max-h-80 object-cover transition-opacity duration-200", loaded ? "opacity-100" : "opacity-0")}
      />
    </span>
  );
}

function ImageBody({
  m,
  showAuthor,
  onOpen,
  term,
}: {
  m: Extract<Message, { kind: "image" }>;
  showAuthor: boolean;
  onOpen?: (id: string) => void;
  term?: string;
}) {
  const author = m.direction === "in" && m.group && showAuthor && m.authorName ? m.authorName : "";
  const hasText = !!m.caption || !!author;
  return (
    <div className="w-72 max-w-full">
      {/* Imagem de ponta a ponta: o balão só tem uma borda fina em volta. */}
      <div className="relative overflow-hidden rounded-[5px]">
        {m.url ? (
          <button
            type="button"
            onClick={() => (onOpen ? onOpen(m.id) : window.open(m.url, "_blank", "noopener"))}
            aria-label="Abrir imagem"
            className="block w-full cursor-zoom-in"
          >
            <ProgressiveImage m={m} />
          </button>
        ) : (
          <div
            className="flex h-40 w-full flex-col items-center justify-center gap-1.5 bg-black/10 text-[11.5px] opacity-80 dark:bg-black/25"
          >
            <ImageIcon className="size-6" />
            {mediaStateLabel(m, "Imagem")}
          </div>
        )}
        {m.hd && (
          <span className="absolute left-1.5 top-1.5 rounded bg-black/50 px-1 text-[10px] font-semibold text-white">HD</span>
        )}
        {!hasText && <MetaOverlay m={m} />}
      </div>
      {hasText && (
        // w-0 min-w-full: o texto quebra na largura da imagem em vez de alargar o balão.
        <div className="w-0 min-w-full px-2 pb-1 pt-1.5">
          {author && <p className="text-[12.5px] font-semibold">{author}:</p>}
          <div className="flex flex-wrap items-end">
            {m.caption && (
              <p className="whitespace-pre-wrap break-words">
                <RichText text={m.caption} term={term} />
              </p>
            )}
            <Meta m={m} />
          </div>
        </div>
      )}
    </div>
  );
}

/** Baixa o arquivo com o nome certo: o navegador oferece abrir no Word/Excel/PowerPoint instalados no computador. */
function saveFile(url: string, fileName: string) {
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  a.rel = "noopener";
  document.body.appendChild(a);
  a.click();
  a.remove();
}

/** Balão: animado só quando a mensagem é nova; as do histórico são um <div> simples (centenas de componentes animados travavam a abertura). */
function Pop({ animate, children, ...rest }: { animate?: boolean; children: React.ReactNode; style?: React.CSSProperties; className?: string }) {
  if (!animate) return <div {...rest}>{children}</div>;
  return (
    <motion.div initial={{ opacity: 0, y: 14, scale: 0.94 }} animate={{ opacity: 1, y: 0, scale: 1 }} transition={{ type: "spring", stiffness: 480, damping: 32, mass: 0.8 }} {...rest}>
      {children}
    </motion.div>
  );
}

function DocumentCard({ m, onOpen, term }: { m: Extract<Message, { kind: "document" }>; onOpen?: (id: string) => void; term?: string }) {
  const fileName = m.fileName || "Documento";
  const kind = docKind(m.fileName, m.mimeType);
  const isPdf = kind === "pdf";
  const app = officeApp[kind];
  const ext = m.fileName?.includes(".") ? m.fileName.split(".").pop()!.toUpperCase() : "ARQ";
  // PDF abre no visualizador do app; Word/Excel/PowerPoint baixam e abrem no próprio Office do computador.
  const action = m.url ? (isPdf && onOpen ? () => onOpen(m.id) : app ? () => saveFile(m.url!, fileName) : null) : null;
  return (
    <div
      className={cn(
        "flex min-w-60 max-w-80 items-start gap-3 rounded-lg bg-black/10 p-2.5 dark:bg-black/20",
        action && "cursor-pointer hover:bg-black/15 dark:hover:bg-black/30",
      )}
      {...(action && {
        role: "button",
        tabIndex: 0,
        title: isPdf ? "Abrir PDF" : `Abrir no ${app}`,
        onClick: action,
        onKeyDown: (e: React.KeyboardEvent) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            action();
          }
        },
      })}
    >
      <DocBadge kind={kind} />
      <div className="min-w-0 flex-1">
        <p className="break-all text-[13px] font-medium leading-snug">
          <Highlight text={fileName} term={term} />
        </p>
        <p className="text-[11px] opacity-70">
          {docKindLabel(kind, ext)}{m.sizeBytes ? ` • ${formatBytes(m.sizeBytes)}` : ""}
          {!m.url && ` • ${historicalDocLabel(m)}`}
        </p>
      </div>
      {m.url && (
        <a
          href={m.url}
          download={fileName}
          onClick={(e) => e.stopPropagation()}
          aria-label={`Baixar ${fileName}`}
          className="rounded-md p-1 opacity-70 hover:opacity-100"
        >
          <Download className="size-4" />
        </a>
      )}
    </div>
  );
}

const TILE = 256;
const ZOOM = 16;
const MAP_W = 288;
const MAP_H = 144;

/** Mapinha como no WhatsApp: usa a miniatura enviada pelo celular; senão monta os blocos do OpenStreetMap (via servidor). */
function LocationMap({ lat, lng, thumb }: { lat: number; lng: number; thumb?: string }) {
  const [failed, setFailed] = useState(false);
  const n = 2 ** ZOOM;
  const fx = ((lng + 180) / 360) * n;
  const rad = (lat * Math.PI) / 180;
  const fy = ((1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI) / 2) * n;
  const tx = Math.floor(fx);
  const ty = Math.floor(fy);
  const left = MAP_W / 2 - (fx - tx) * TILE;
  const top = MAP_H / 2 - (fy - ty) * TILE;
  return (
    <div className="relative h-36 w-full overflow-hidden bg-[rgb(128_128_128/.18)]">
      {thumb ? (
        <img src={thumb} alt="" className="size-full object-cover" />
      ) : (
        !failed &&
        [-1, 0, 1].flatMap((dy) =>
          [-1, 0, 1].map((dx) => (
            <img
              key={`${dx},${dy}`}
              src={`/api/v1/map-tiles/${ZOOM}/${tx + dx}/${ty + dy}`}
              alt=""
              draggable={false}
              loading="lazy"
              onError={() => setFailed(true)}
              className="absolute max-w-none select-none dark:[filter:invert(1)_hue-rotate(180deg)_brightness(.85)_contrast(.9)]"
              style={{ width: TILE, height: TILE, left: left + dx * TILE, top: top + dy * TILE }}
            />
          )),
        )
      )}
      <MapPin className="absolute left-1/2 top-1/2 size-8 -translate-x-1/2 -translate-y-full fill-red-500 text-red-700 drop-shadow" />
    </div>
  );
}

function LocationCard({ m }: { m: Extract<Message, { kind: "location" }> }) {
  const href = `https://www.google.com/maps?q=${m.lat},${m.lng}`;
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`Abrir no mapa${m.name ? `: ${m.name}` : ""}`}
      className="block w-72 max-w-full"
    >
      <div className="overflow-hidden rounded-lg">
        <LocationMap lat={m.lat} lng={m.lng} thumb={m.thumb} />
      </div>
      {(m.name || m.address) && (
        <div className="px-0.5 pt-1.5">
          {m.name && <p className="text-[13px] font-semibold">{m.name}</p>}
          {m.address && <p className="text-[12px] opacity-80">{m.address}</p>}
        </div>
      )}
    </a>
  );
}

export function MessageBubble({
  m,
  showAuthor,
  startsGroup = false,
  animateIn,
  onOpenImage,
  onOpenPdf,
  term,
  emphasis,
  onMenu,
  onJump,
}: {
  m: Message;
  showAuthor: boolean;
  /** A primeira mensagem de cada sequência ganha uma cauda no lado do remetente. */
  startsGroup?: boolean;
  /** Abre o menu de ações com o botão direito. */
  onMenu?: (m: Message, x: number, y: number) => void;
  /** Vai até a mensagem original de uma resposta. */
  onJump?: (id: string) => void;
  animateIn?: boolean;
  onOpenImage?: (id: string) => void;
  onOpenPdf?: (id: string) => void;
  /** Trecho buscado, destacado no texto. */
  term?: string;
  /** Mensagem em foco (resultado atual da busca ou destino de um link). */
  emphasis?: boolean;
}) {
  const out = m.direction === "out";
  const sticker = m.kind === "sticker" && !m.deleted;
  // Toque (celular/tablet): clicar e segurar abre o mesmo menu do botão direito.
  const press = useRef<{ timer: number; x: number; y: number } | null>(null);
  const longPressed = useRef(false);
  const cancelPress = () => {
    if (press.current) window.clearTimeout(press.current.timer);
    press.current = null;
  };
  return (
    <div
      className={cn("msg-row flex", out ? "justify-end" : "justify-start")}
      onPointerDown={onMenu ? (e) => {
        if (e.pointerType === "mouse" || (e.target as HTMLElement).closest("a, button, input, textarea, audio")) return;
        cancelPress();
        longPressed.current = false;
        const { clientX: x, clientY: y } = e;
        press.current = {
          x,
          y,
          timer: window.setTimeout(() => {
            longPressed.current = true;
            press.current = null;
            navigator.vibrate?.(12);
            onMenu(m, x, y);
          }, 450),
        };
      } : undefined}
      onPointerMove={(e) => {
        const p = press.current;
        if (p && Math.hypot(e.clientX - p.x, e.clientY - p.y) > 10) cancelPress(); // virou rolagem
      }}
      onPointerUp={cancelPress}
      onPointerCancel={cancelPress}
      onPointerLeave={cancelPress}
      onContextMenu={onMenu ? (e) => {
        if (longPressed.current) {
          // O toque longo já abriu o nosso menu: o navegador não mostra o dele.
          e.preventDefault();
          longPressed.current = false;
          return;
        }
        // Links, textos selecionados e imagens mantêm o menu do navegador.
        if ((e.target as HTMLElement).closest("a, button, input, textarea") || window.getSelection()?.toString()) return;
        e.preventDefault();
        onMenu(m, e.clientX, e.clientY);
      } : undefined}
    >
      <Pop
        // Mensagens novas "brotam" do lado de quem enviou; o histórico é um <div> comum (sem componente animado por balão).
        animate={animateIn}
        style={{ transformOrigin: out ? "100% 100%" : "0% 100%" }}
        // A cauda fica dentro dos limites da camada animada, evitando recorte ao escalar.
        className={cn("relative -mx-2 max-w-[min(29rem,calc(75%+1rem))] px-2", !sticker && "drop-shadow-sm")}
      >
      <div
        className={cn(
          "group/msg relative rounded-lg text-[13.5px] leading-[1.4] transition-shadow duration-300",
          sticker ? "p-0" : m.kind === "image" ? "p-[3px]" : "px-2.5 py-[5px]",
          !sticker && (startsGroup ? "bubble-with-tail" : out ? "bg-out" : "bg-in"),
          !sticker && startsGroup && (out ? "bubble-with-tail-out" : "bubble-with-tail-in"),
          startsGroup && (out ? "rounded-tr-none" : "rounded-tl-none"),
          // A busca destaca o balão sem alterar o contraste do texto.
          sticker ? "text-fg" : out ? "text-out-fg" : "text-in-fg",
          emphasis && "ring-2 ring-brand-ink ring-offset-2 ring-offset-chat",
          m.status === "failed" && "ring-1 ring-danger",
        )}
      >
        {m.deleted ? (
          <div className="flex flex-wrap items-end">
            <p className="flex items-center gap-1.5 italic opacity-80">
              <Ban className="size-3.5" /> {out ? "Você apagou esta mensagem" : "Esta mensagem foi apagada"}
            </p>
            <Meta m={m} />
          </div>
        ) : (
          <>
        {out && m.automated && (
          <p className="mb-0.5 flex items-center gap-1.5 text-[12.5px] font-semibold">
            <ProviderLogo provider={m.aiProvider} title={`Enviado por ${m.aiApp || providerLabel(m.aiProvider)}`} />
            {m.aiApp || providerLabel(m.aiProvider)}
            <span className="font-normal opacity-70">{m.aiMode === "attend" ? "· atendimento por IA" : "· envio automático"}</span>
          </p>
        )}
        {m.kind !== "image" && !out && m.group && showAuthor && m.authorName && (
          <p className="text-[12.5px] font-semibold">{m.authorName}</p>
        )}
        {m.quoted && <div className={m.kind === "image" ? "px-1.5 pt-1.5" : ""}><QuotedBlock q={m.quoted} onJump={onJump} /></div>}

        {m.kind === "text" && (
          <div className="flex flex-wrap items-end">
            <p className="whitespace-pre-wrap break-words">
                <RichText text={m.text} term={term} />
            </p>
            <Meta m={m} />
          </div>
        )}

        {m.kind === "audio" && (
          <div className="flex flex-col">
            <AudioPlayer durationSec={m.durationSec} waveform={m.waveform} url={m.url} outgoing={out} metadata={<Meta m={m} />} />
          {m.historical && !m.url && <p className="mt-1 text-[11px] opacity-70">Áudio do histórico · arquivo não importado</p>}
          </div>
        )}

        {m.kind === "document" && (
          <div className="flex flex-col gap-1 pt-1">
            <DocumentCard m={m} onOpen={onOpenPdf} term={term} />
            {m.caption && <p className="whitespace-pre-wrap break-words pt-1"><RichText text={m.caption} term={term} /></p>}
            <Meta m={m} />
          </div>
        )}

        {m.kind === "image" && <ImageBody m={m} showAuthor={showAuthor} onOpen={onOpenImage} term={term} />}
        {m.kind === "sticker" && <StickerBody m={m} />}

        {m.kind === "unsupported" && (
          <div className="flex flex-wrap items-end">
            <p className="flex items-center gap-1.5 italic opacity-80">
              <Ban className="size-3.5" /> {m.label} — ainda não exibido no ChatHM
            </p>
            {m.text && <p className="w-full whitespace-pre-wrap break-words">{m.text}</p>}
            <Meta m={m} />
          </div>
        )}

        {m.kind === "location" && (
          <div className="flex flex-col gap-1 pt-1">
            <LocationCard m={m} />
            <Meta m={m} />
          </div>
        )}
          </>
        )}
      </div>
      </Pop>
    </div>
  );
}
