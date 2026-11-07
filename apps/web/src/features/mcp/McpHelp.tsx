import { useEffect, useId, useRef, useState } from "react";
import { Info, X } from "lucide-react";
import { IconButton } from "@/components/ui";
import { safeStorage } from "@/lib/storage";

const seenUsers = new Set<string>();
const seenKey = (userId: string) => `chathm:mcp-help:seen:${userId}`;

export function McpHelp({ userId }: { userId: string }) {
  const [open, setOpen] = useState(() => !seenUsers.has(userId) && safeStorage.getItem(seenKey(userId)) !== "1");
  const [notice, setNotice] = useState("");
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const id = useId();

  useEffect(() => {
    if (!open) return;
    seenUsers.add(userId);
    safeStorage.setItem(seenKey(userId), "1");
    panel.current?.focus({ preventScroll: true });
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setOpen(false);
      trigger.current?.focus({ preventScroll: true });
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [open, userId]);

  function goTo(targetId: string) {
    const target = document.getElementById(targetId) ?? document.getElementById("mcp-access");
    if (!target) { setNotice("As opções ainda estão carregando. Tente novamente em instantes."); return; }
    let details = target.closest("details");
    while (details) {
      details.open = true;
      details = details.parentElement?.closest("details") ?? null;
    }
    setOpen(false);
    if (target instanceof HTMLDetailsElement) target.querySelector("summary")?.focus({ preventScroll: true });
    else target.focus({ preventScroll: true });
    target.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth", block: "start" });
  }

  const sectionLink = (label: string, targetId: string) => <a href={`#${targetId}`} className="font-medium text-brand-ink underline decoration-brand/40 underline-offset-2 hover:decoration-brand focus-visible:rounded focus-visible:outline-2 focus-visible:outline-brand" onClick={event => { event.preventDefault(); goTo(targetId); }}>{label}</a>;

  return <div ref={root} className="relative shrink-0" onBlur={event => {
    if (event.relatedTarget && !root.current?.contains(event.relatedTarget as Node)) setOpen(false);
  }}>
    <button ref={trigger} type="button" aria-label="Como configurar o MCP" title="Como configurar o MCP" aria-haspopup="dialog" aria-expanded={open} aria-controls={open ? id : undefined} onClick={() => { setNotice(""); setOpen(value => !value); }} className="flex size-9 items-center justify-center rounded-full text-muted outline-none transition-colors hover:bg-hover hover:text-brand-ink focus-visible:ring-2 focus-visible:ring-brand">
      <Info className="size-5" />
    </button>
    {open && <div ref={panel} id={id} role="dialog" aria-labelledby={`${id}-title`} tabIndex={-1} className="absolute right-0 top-full z-40 mt-2 max-h-[calc(100dvh-8rem)] w-[min(24rem,calc(100vw-2rem))] overflow-y-auto rounded-2xl border border-line bg-panel p-5 text-fg shadow-xl outline-none">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 id={`${id}-title`} className="text-sm font-semibold">Como configurar o MCP</h2>
          <p className="mt-1 text-xs leading-relaxed text-muted">Conecte seu assistente ao ChatHM em quatro passos.</p>
        </div>
        <IconButton label="Fechar ajuda" className="-mr-1 -mt-1 size-7 shrink-0" onClick={() => { setOpen(false); trigger.current?.focus({ preventScroll: true }); }}><X className="size-4" /></IconButton>
      </div>
      <ol className="space-y-4">
        {[
          { title: "Ative o acesso por IA", text: <>Ligue o {sectionLink("acesso por IA", "mcp-access")}. Se você for atendente, peça a ativação ao administrador.</> },
          { title: "Defina o que o assistente pode acessar", text: <>Revise {sectionLink("Permissões", "mcp-permissions")} e {sectionLink("Privacidade", "mcp-privacy")}. Você pode começar com somente leitura.</> },
          { title: "Escolha a forma de conexão", text: <>Por token: crie um acesso em {sectionLink("Tokens de acesso", "mcp-tokens")} e copie-o para seu assistente. Por login: habilite {sectionLink("Conexão por login", "mcp-login")} e autorize o aplicativo no ChatHM.</> },
          { title: "Teste no seu assistente", text: "Peça um resumo de uma conversa ou uma consulta à fila. Mensagens para clientes continuam dependendo da sua aprovação." },
        ].map((step, index) => <li key={step.title} className="flex items-start gap-3">
          <span aria-hidden="true" className="flex size-6 shrink-0 items-center justify-center rounded-full bg-brand/15 text-xs font-semibold text-brand-ink">{index + 1}</span>
          <div><h3 className="text-[13px] font-medium">{step.title}</h3><p className="mt-1 text-xs leading-relaxed text-muted">{step.text}</p></div>
        </li>)}
      </ol>
      <div className="mt-4 rounded-xl bg-elevated p-3">
        <p className="text-xs font-medium">Experimente pedir ao assistente</p>
        <blockquote className="mt-1.5 text-xs leading-relaxed text-muted">“Resuma as conversas não atribuídas de hoje.”</blockquote>
      </div>
      <p className="mt-4 border-t border-line/60 pt-3 text-xs leading-relaxed text-muted">Em {sectionLink("Como conectar o assistente", "mcp-connect-guide")}, você encontra as instruções e os comandos de cada serviço.</p>
      {notice && <p role="status" className="mt-3 text-xs text-muted">{notice}</p>}
    </div>}
  </div>;
}
