import { cn } from "@/lib/cn";

export function TypingIndicator({ className, announce = false }: { className?: string; announce?: boolean }) {
  return <span role={announce ? "status" : "img"} aria-label="Digitando" className={cn("inline-flex items-baseline whitespace-nowrap font-medium", className)}>
    <span aria-hidden="true" className="typing-gradient">Digitando</span>
    <span aria-hidden="true" className="ml-0.5 inline-flex text-muted">
      {[0, 1, 2].map((dot) => <span key={dot} className="typing-dot" style={{ animationDelay: `${dot * 180}ms` }}>.</span>)}
    </span>
  </span>;
}

/** Bolha de "digitando" no fim das mensagens, como no WhatsApp. O aviso falado já vem do cabeçalho, por isso aqui é decorativa. */
export function TypingBubble() {
  return (
    <div aria-hidden="true" className="mt-2 flex" data-testid="typing-bubble">
      <span className="inline-flex items-center gap-1 rounded-2xl rounded-bl-md bg-in px-3.5 py-3 text-in-fg shadow-sm">
        {[0, 1, 2].map((dot) => (
          <span key={dot} className="typing-bubble-dot size-1.5 rounded-full bg-current" style={{ animationDelay: `${dot * 180}ms` }} />
        ))}
      </span>
    </div>
  );
}
