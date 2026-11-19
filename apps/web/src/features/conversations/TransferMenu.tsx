import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Check } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { usePeople, type Person } from "@/features/team/api";
import { useOnline } from "@/features/team/presence";
import { Spinner } from "@/components/ui";
import type { Conversation } from "./types";

/**
 * Lista todo o escritório para transferir a conversa (clique direito na lista ou botão no chat).
 * Posição fixa, limitada à tela; fecha com Esc, clique fora, rolagem ou redimensionamento.
 */
export function TransferMenu({
  conversation: c,
  x,
  y,
  myId,
  canTransfer,
  onPick,
  onClose,
  variant = "transfer",
}: {
  conversation: Conversation;
  x: number;
  y: number;
  myId: string;
  canTransfer: boolean;
  onPick: (p: Person) => void;
  onClose: () => void;
  /** participants: incluir/tirar pessoas do mesmo atendimento, sem trocar o responsável. */
  variant?: "transfer" | "participants";
}) {
  const participants = variant === "participants";
  const { data: people, isPending } = usePeople();
  const online = useOnline();
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const m = 8;
    setPos({
      left: Math.max(m, Math.min(x, window.innerWidth - el.offsetWidth - m)),
      top: Math.max(m, Math.min(y, window.innerHeight - el.offsetHeight - m)),
    });
  }, [x, y, people]);

  useEffect(() => {
    const down = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && onClose();
    const key = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("mousedown", down);
    document.addEventListener("keydown", key);
    window.addEventListener("resize", onClose);
    window.addEventListener("blur", onClose);
    return () => {
      document.removeEventListener("mousedown", down);
      document.removeEventListener("keydown", key);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  return createPortal(
    <div
      ref={ref}
      role="menu"
      aria-label={participants ? `Participantes do atendimento de ${c.contact.name}` : `Transferir conversa de ${c.contact.name}`}
      style={pos}
      className="fixed z-50 max-h-[70vh] w-64 overflow-y-auto rounded-xl border border-line bg-panel py-1 shadow-xl"
    >
      <p className="flex items-center justify-between gap-2 px-3 pb-1 pt-1.5 text-[11px] font-medium uppercase tracking-wide text-muted">
        <span>{participants ? "Pessoas neste atendimento" : "Transferir para"}</span>
        <span className="flex items-center gap-1 text-[10.5px] font-normal normal-case tracking-normal"><span aria-hidden className="size-2 rounded-full bg-emerald-500" />{people ? people.filter((p) => online.has(p.id)).length : 0} online</span>
      </p>
      {participants && canTransfer && <p className="px-3 pb-1.5 text-[12px] text-muted">Quem for incluída vê a conversa e responde, sem tirar {c.assignee ? c.assignee.name : "o responsável"}. Clique de novo para tirar.</p>}
      {!canTransfer && (
        <p className="px-3 pb-2 text-[12px] text-muted">
          {c.status === "closed"
            ? "Atendimento encerrado."
            : participants
              ? `Só ${c.assignee?.name ?? "o responsável"}, quem já participa ou um gestor/administrador pode mudar os participantes.`
              : `${c.assignee?.name ?? "Outra pessoa"} está atendendo; só ela (ou um gestor/administrador) pode transferir.`}
        </p>
      )}
      {isPending && <Spinner className="mx-auto my-3" />}
      {people?.map((p) => {
        const isOwner = c.assignee?.id === p.id;
        const included = !!c.participants?.some((x) => x.id === p.id);
        const current = participants ? included : isOwner;
        return (
          <button
            key={p.id}
            type="button"
            role="menuitem"
            disabled={!canTransfer || (participants && isOwner)}
            onClick={() => {
              if (participants || !current) onPick(p);
              onClose();
            }}
            className="flex w-full items-center gap-2.5 px-3 py-1.5 text-left hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            <span className="relative shrink-0">
              <Avatar name={p.name} size="sm" />
              {online.has(p.id) && <span aria-hidden data-testid={`online-${p.id}`} className="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full bg-emerald-500 ring-2 ring-panel" />}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[13px]">
                {p.name}
                {p.id === myId && <span className="text-muted"> (você)</span>}
                {online.has(p.id) && <span className="sr-only"> (online)</span>}
              </span>
              {p.teams.length > 0 && <span className="block truncate text-[11px] text-muted">{p.teams.join(", ")}</span>}
            </span>
            {participants && isOwner && <span className="text-[11px] text-muted">Responsável</span>}
            {current && <Check className="size-4 text-brand-ink" aria-label={participants ? "Participa do atendimento" : "Atendendo agora"} />}
          </button>
        );
      })}
    </div>,
    document.body,
  );
}
