import { cn } from "@/lib/cn";

export function TypingIndicator({ className, announce = false }: { className?: string; announce?: boolean }) {
  return <span role={announce ? "status" : "img"} aria-label="Digitando" className={cn("inline-flex items-baseline whitespace-nowrap font-medium", className)}>
    <span aria-hidden="true" className="typing-gradient">Digitando</span>
    <span aria-hidden="true" className="ml-0.5 inline-flex text-muted">
      {[0, 1, 2].map((dot) => <span key={dot} className="typing-dot" style={{ animationDelay: `${dot * 180}ms` }}>.</span>)}
    </span>
  </span>;
}
