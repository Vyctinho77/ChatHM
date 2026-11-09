import { Ban, Image, MapPin, Mic, Sticker } from "lucide-react";
import { DocBadge } from "./DocBadge";
import { docKind } from "./docKind";
import { formatDuration } from "@/lib/format";
import type { Message } from "./types";

/** Resumo de uma mensagem para a lista de conversas. */
export function MessagePreview({ m }: { m: Message }) {
  const icon = "size-3.5 shrink-0";
  switch (m.kind) {
    case "sticker":
      return <><Sticker className={icon} /> <span className="truncate">Figurinha</span></>;
    case "text":
      return <span className="truncate">{m.text}</span>;
    case "audio":
      return (
        <>
          <Mic className={icon} /> <span>{formatDuration(m.durationSec)}</span>
        </>
      );
    case "document":
      return (
        <>
          <DocBadge kind={docKind(m.fileName, m.mimeType)} size="sm" className="rounded-[3px]" /> <span className="truncate">{m.fileName}</span>
        </>
      );
    case "image":
      return (
        <>
          <Image className={icon} /> <span className="truncate">{m.caption || "Foto"}</span>
        </>
      );
    case "unsupported":
      return (
        <>
          <Ban className={icon} /> <span className="truncate">{m.label}</span>
        </>
      );
    case "location":
      return (
        <>
          <MapPin className={icon} /> <span className="truncate">{m.name ?? "Localização"}</span>
        </>
      );
  }
}
