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
          "relative max-w-[min(32rem,80%)] rounded-lg px-2.5 py-1.5 text-[13.5px] leading-[1.4] shadow-sm",
          out ? "rounded-tr-sm bg-out text-out-fg" : "rounded-tl-sm bg-in text-fg",
          m.status === "failed" && "ring-1 ring-danger",
        )}
      >
        {out && showAuthor && m.authorName && <p className="text-[12.5px] font-semibold">{m.authorName}:</p>}

        {m.kind === "text" && (
          <div className="flex flex-wrap items-end">
            <p className="whitespace-pre-wrap break-words">{m.text}</p>
            <Meta m={m} />
          </div>
        )}

        {m.kind === "audio" && (
          <div className="flex items-end">
            <AudioPlayer durationSec={m.durationSec} waveform={m.waveform} url={m.url} outgoing={out} />
            <Meta m={m} />
          </div>
        )}

        {m.kind === "document" && (
          <div className="flex flex-col gap-1 pt-1">
            <DocumentCard m={m} />
            <Meta m={m} />
          </div>
        )}

        {m.kind === "image" && (
          <div className="flex flex-col gap-1 pt-1">
            {m.url ? (
              <a href={m.url} target="_blank" rel="noopener noreferrer" className="block overflow-hidden rounded-lg">
                <img
                  src={m.url}
                  alt={m.caption ?? "Imagem"}
                  width={m.width}
                  height={m.height}
                  className="max-h-80 w-72 max-w-full object-cover"
                />
              </a>
            ) : (
              <div
                className="flex h-36 w-64 max-w-full flex-col items-center justify-center gap-1.5 rounded-lg bg-black/10 text-[11.5px] opacity-80 dark:bg-black/25"
                title="O download de mídias chega na Fase 4"
              >
                <ImageIcon className="size-6" />
                Imagem recebida
              </div>
            )}
            {m.hd && <span className="text-[11px] font-semibold opacity-70">HD</span>}
            <div className="flex flex-wrap items-end">
              {m.caption && <p className="whitespace-pre-wrap break-words">{m.caption}</p>}
              <Meta m={m} />
            </div>
          </div>
        )}

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
