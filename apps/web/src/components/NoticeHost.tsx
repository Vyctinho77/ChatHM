import { Link } from "react-router";
import { X } from "lucide-react";
import { dismissNotice, useNotices } from "@/lib/notices";

/** Avisos curtos no canto da tela (ver lib/notices). Cada um é anunciado pelos leitores de tela e some sozinho. */
export function NoticeHost() {
  const notices = useNotices();
  if (!notices.length) return null;
  return (
    <div role="region" aria-label="Avisos" className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-[min(24rem,calc(100vw-2rem))] flex-col gap-2">
      {notices.map((n) => (
        <div key={n.id} role="status" className="pointer-events-auto flex items-start gap-3 rounded-xl border border-line bg-panel px-3.5 py-3 text-[13px] shadow-xl">
          <span className="min-w-0 flex-1">
            {n.text}
            {n.link && (
              <>
                {" "}
                <Link to={n.link.to} onClick={() => dismissNotice(n.id)} className="font-medium text-brand-ink underline-offset-2 hover:underline">
                  {n.link.label}
                </Link>
              </>
            )}
          </span>
          <button type="button" aria-label="Fechar aviso" onClick={() => dismissNotice(n.id)} className="shrink-0 rounded p-0.5 text-muted hover:text-fg">
            <X className="size-4" />
          </button>
        </div>
      ))}
    </div>
  );
}
