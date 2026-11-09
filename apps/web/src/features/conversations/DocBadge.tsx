import { FileText } from "lucide-react";
import { cn } from "@/lib/cn";
import type { DocKind } from "./docKind";

const style: Record<Exclude<DocKind, "other">, { bg: string; mark: string }> = {
  pdf: { bg: "bg-red-600", mark: "PDF" },
  word: { bg: "bg-[#2b579a]", mark: "W" },
  excel: { bg: "bg-[#217346]", mark: "X" },
  powerpoint: { bg: "bg-[#c43e1c]", mark: "P" },
};

const box = { sm: "size-5", md: "h-11 w-9", lg: "h-20 w-16" } as const;
const glyph = { sm: "text-[11px]", md: "text-[20px]", lg: "text-[36px]" } as const;
const pdfGlyph = { sm: "text-[6px]", md: "text-[10px]", lg: "text-[18px]" } as const;
const icon = { sm: "size-3", md: "size-5", lg: "size-9" } as const;

/** Ícone do documento: cor e letra de cada aplicativo (estilo próprio, sem usar as marcas registradas). */
export function DocBadge({ kind, size = "md", className }: { kind: DocKind; size?: "sm" | "md" | "lg"; className?: string }) {
  if (kind === "other") {
    return (
      <span aria-hidden className={cn("inline-flex shrink-0 items-center justify-center rounded-md bg-slate-500 text-white", box[size], className)}>
        <FileText className={icon[size]} />
      </span>
    );
  }
  const s = style[kind];
  return (
    <span aria-hidden className={cn("inline-flex shrink-0 select-none items-center justify-center rounded-md font-bold leading-none text-white", s.bg, box[size], className)}>
      <span className={cn(kind === "pdf" ? pdfGlyph[size] : cn("font-extrabold", glyph[size]))}>{s.mark}</span>
    </span>
  );
}
