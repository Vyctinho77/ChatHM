import { FileText, Image, MapPin, Mic } from "lucide-react";
import { formatDuration } from "@/lib/format";
import type { Message } from "./types";

/** Resumo de uma mensagem para a lista de conversas. */
export function MessagePreview({ m }: { m: Message }) {
  const icon = "size-4 shrink-0";
  switch (m.kind) {
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
    case "location":
      return (
        <>
          <MapPin className={icon} /> <span className="truncate">{m.name ?? "Localização"}</span>
        </>
      );
  }
}
