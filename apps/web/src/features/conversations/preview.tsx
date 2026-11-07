import { Ban, FileText, Image, MapPin, Mic, Sticker } from "lucide-react";
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
          <FileText className={icon} /> <span className="truncate">{m.fileName}</span>
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
