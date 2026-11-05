import { motion } from "motion/react";
import { Ban, Download, ExternalLink, FileText, Image as ImageIcon, MapPin } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatBytes, formatTime } from "@/lib/format";
import { AudioPlayer } from "./AudioPlayer";
import { receiptDetails } from "./receipts";
import { Ticks } from "./Ticks";
import type { Message } from "./types";

function Meta({ m }: { m: Message }) {
  return (
    <span
      className="ml-auto flex shrink-0 items-center gap-1 self-end pl-2.5 text-[10.5px] tabular-nums opacity-70"
      title={m.direction === "out" ? receiptDetails(m) : undefined}
    >
      {formatTime(m.sentAt)}
      {m.direction === "out" && <Ticks status={m.status} />}
    </span>
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

/** Texto do espaço de uma mídia cujo arquivo ainda não chegou ou não pode ser exibido. */
function mediaStateLabel(m: Message, what: string) {
  if (m.historical) return `${what} do histórico · arquivo não importado`;
  if (m.mediaState === "blocked") return `${what} bloqueada por segurança`;
  if (m.mediaState === "failed") return `Não foi possível baixar ${what.toLowerCase() === "imagem" ? "a imagem" : "o arquivo"}`;
  return `${what} recebida · baixando…`;
}

function ImageBody({ m, showAuthor }: { m: Extract<Message, { kind: "image" }>; showAuthor: boolean }) {
  const author = m.direction === "out" && showAuthor && m.authorName ? m.authorName : "";
  const hasText = !!m.caption || !!author;
  return (
    <div className="w-72 max-w-full">
      {/* Imagem de ponta a ponta: o balão só tem uma borda fina em volta. */}
      <div className="relative overflow-hidden rounded-[5px]">
        {m.url ? (
          <a href={m.url} target="_blank" rel="noopener noreferrer" className="block">
            <img
              src={m.url}
              alt={m.caption ?? "Imagem"}
              width={m.width}
              height={m.height}
              className="block max-h-80 w-full object-cover"
            />
          </a>
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
            {m.caption && <p className="whitespace-pre-wrap break-words">{m.caption}</p>}
            <Meta m={m} />
          </div>
        </div>
      )}
    </div>
  );
}

function DocumentCard({ m }: { m: Extract<Message, { kind: "document" }> }) {
  const isPdf = m.mimeType === "application/pdf";
  const ext = m.fileName.split(".").pop()?.toUpperCase() ?? "ARQ";
  return (
    <div className="flex min-w-60 max-w-80 items-start gap-3 rounded-lg bg-black/10 p-2.5 dark:bg-black/20">
      <span
        className={cn(
          "inline-flex h-11 w-9 shrink-0 items-center justify-center rounded-md text-[10px] font-bold text-white",
          isPdf ? "bg-red-600" : "bg-slate-500",
        )}
      >
        {isPdf ? "PDF" : <FileText className="size-5" />}
      </span>
      <div className="min-w-0 flex-1">
        <p className="break-all text-[13px] font-medium leading-snug">{m.fileName}</p>
        <p className="text-[11px] opacity-70">
          {isPdf ? "PDF" : ext} • {formatBytes(m.sizeBytes)}
          {!m.url && ` • ${m.historical ? "arquivo do histórico não importado" : m.mediaState === "blocked" ? "bloqueado por segurança" : m.mediaState === "failed" ? "download falhou" : "baixando…"}`}
        </p>
      </div>
      {m.url && (
        <a
          href={m.url}
          download={m.fileName}
          aria-label={`Baixar ${m.fileName}`}
          className="rounded-md p-1 opacity-70 hover:opacity-100"
        >
          <Download className="size-4" />
        </a>
      )}
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
      className="block w-72 max-w-full overflow-hidden rounded-lg bg-black/10 dark:bg-black/20"
    >
      <div className="relative flex h-28 items-center justify-center bg-[linear-gradient(90deg,rgb(128_128_128/.12)_1px,transparent_1px),linear-gradient(rgb(128_128_128/.12)_1px,transparent_1px)] bg-[size:16px_16px]">
        <MapPin className="size-8 fill-red-500 text-red-700" />
      </div>
      <div className="p-2.5">
        {m.name && <p className="text-[13px] font-semibold">{m.name}</p>}
        {m.address && <p className="text-[12px] opacity-80">{m.address}</p>}
        <p className="mt-1 flex items-center gap-1 text-xs text-read">
          Abrir no mapa <ExternalLink className="size-3" />
        </p>
      </div>
    </a>
  );
}

export function MessageBubble({ m, showAuthor, animateIn }: { m: Message; showAuthor: boolean; animateIn?: boolean }) {
  const out = m.direction === "out";
  return (
    <div className={cn("flex", out ? "justify-end" : "justify-start")}>
      <motion.div
        // Mensagens novas "brotam" do lado de quem enviou; o histórico aparece sem animação individual.
        initial={animateIn ? { opacity: 0, y: 14, scale: 0.94 } : false}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: "spring", stiffness: 480, damping: 32, mass: 0.8 }}
        style={{ transformOrigin: out ? "100% 100%" : "0% 100%" }}
        className={cn(
          "relative max-w-[min(32rem,80%)] rounded-lg text-[13.5px] leading-[1.4] shadow-sm",
          m.kind === "image" ? "p-[3px]" : "px-2.5 py-1.5",
          out ? "rounded-tr-sm bg-out text-out-fg" : "rounded-tl-sm bg-in text-fg",
          m.status === "failed" && "ring-1 ring-danger",
        )}
      >
        {m.kind !== "image" && out && showAuthor && m.authorName && (
          <p className="text-[12.5px] font-semibold">{m.authorName}:</p>
        )}

        {m.kind === "text" && (
          <div className="flex flex-wrap items-end">
            <p className="whitespace-pre-wrap break-words">{m.text}</p>
            <Meta m={m} />
          </div>
        )}

        {m.kind === "audio" && (
          <div className="flex flex-col">
          <div className="flex items-end">
            <AudioPlayer durationSec={m.durationSec} waveform={m.waveform} url={m.url} outgoing={out} />
            <Meta m={m} />
          </div>
          {m.historical && !m.url && <p className="mt-1 text-[11px] opacity-70">Áudio do histórico · arquivo não importado</p>}
          </div>
        )}

        {m.kind === "document" && (
          <div className="flex flex-col gap-1 pt-1">
            <DocumentCard m={m} />
            <Meta m={m} />
          </div>
        )}

        {m.kind === "image" && <ImageBody m={m} showAuthor={showAuthor} />}

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
      </motion.div>
    </div>
  );
}
