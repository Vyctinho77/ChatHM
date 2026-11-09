import { Bot } from "lucide-react";
import { cn } from "@/lib/cn";
import { providerLabel } from "@/lib/providers";

// Marcas simplificadas próprias, só para identificar de qual IA veio a mensagem (uso interno).
const BRANDS: Record<string, { label: string; bg: string; fg: string; glyph: string }> = {
  claude: { label: "Claude", bg: "#D97757", fg: "#fff", glyph: "✳" },
  chatgpt: { label: "ChatGPT", bg: "#10A37F", fg: "#fff", glyph: "✺" },
  grok: { label: "Grok", bg: "#111111", fg: "#fff", glyph: "∕" },
  gemini: { label: "Gemini", bg: "#4B6BFB", fg: "#fff", glyph: "✦" },
};

export function ProviderLogo({ provider, className, title }: { provider?: string; className?: string; title?: string }) {
  const b = BRANDS[provider ?? ""];
  return (
    <span
      role="img"
      aria-label={title ?? providerLabel(provider)}
      title={title ?? providerLabel(provider)}
      className={cn("inline-flex size-4 shrink-0 select-none items-center justify-center rounded-full text-[10px] leading-none", !b && "bg-elevated text-muted ring-1 ring-line", className)}
      style={b ? { background: b.bg, color: b.fg } : undefined}
    >
      {b ? b.glyph : <Bot className="size-2.5" />}
    </span>
  );
}
