import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Check, Copy, Download, Forward, Info, Pencil, Reply, Trash2 } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Alert, Button, Modal } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatTime } from "@/lib/format";
import { matches } from "@/lib/search";
import { messageCapabilities, copyText } from "./messageCaps";
import { statusLabel } from "./receipts";
import { useConversationActions, useConversations } from "./store";
import type { Message } from "./types";

type MenuProps = {
  message: Message;
  x: number;
  y: number;
  onReply: () => void;
  onForward: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onInfo: () => void;
  onClose: () => void;
};

/** Menu de botão direito de uma mensagem. Posição fixa limitada à tela; fecha com Esc, clique fora ou rolagem. */
export function MessageMenu({ message: m, x, y, onReply, onForward, onEdit, onDelete, onInfo, onClose }: MenuProps) {
  const caps = messageCapabilities(m);
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const [copied, setCopied] = useState(false);
  const openedAt = useRef(0);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const margin = 8;
    setPos({
      left: Math.max(margin, Math.min(x, window.innerWidth - el.offsetWidth - margin)),
      top: Math.max(margin, Math.min(y, window.innerHeight - el.offsetHeight - margin)),
    });
  }, [x, y]);

  useEffect(() => {
    openedAt.current = performance.now();
    // Ao soltar o dedo depois do toque longo, o navegador dispara um "clique" sobre a mensagem: não conta como clique fora.
    const down = (e: MouseEvent) => performance.now() - openedAt.current > 700 && !ref.current?.contains(e.target as Node) && onClose();
    const key = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("mousedown", down);
    document.addEventListener("keydown", key);
    document.addEventListener("scroll", onClose, true);
    window.addEventListener("resize", onClose);
    window.addEventListener("blur", onClose);
    return () => {
      document.removeEventListener("mousedown", down);
      document.removeEventListener("keydown", key);
      document.removeEventListener("scroll", onClose, true);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(copyText(m));
      setCopied(true);
      setTimeout(onClose, 700);
    } catch {
      onClose();
    }
  }

  const item = (icon: React.ReactNode, label: string, onClick: () => void, opts?: { danger?: boolean }) => (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={cn("flex w-full items-center gap-3 px-3.5 py-2 text-left text-[13.5px] hover:bg-hover [&>svg]:size-[18px] [&>svg]:shrink-0", opts?.danger ? "text-danger" : "[&>svg]:text-muted")}
    >
      {icon}
      {label}
    </button>
  );

  return createPortal(
    <div ref={ref} role="menu" aria-label="Ações da mensagem" style={pos} className="fixed z-50 w-60 overflow-hidden rounded-2xl border border-line bg-panel py-1.5 shadow-xl">
      {caps.info && item(<Info />, "Dados da mensagem", () => (onInfo(), onClose()))}
      {caps.reply && item(<Reply />, "Responder", () => (onReply(), onClose()))}
      {caps.copy && item(copied ? <Check /> : <Copy />, copied ? "Copiado" : "Copiar", () => void copy())}
      {caps.forward && item(<Forward />, "Encaminhar", () => (onForward(), onClose()))}
      {caps.download && "url" in m && item(<Download />, "Baixar", () => {
        const a = document.createElement("a");
        a.href = m.url!;
        a.download = m.kind === "document" ? m.fileName : "";
        a.click();
        onClose();
      })}
      {caps.edit && item(<Pencil />, "Editar", () => (onEdit(), onClose()))}
      <div className="my-1 border-t border-line" />
      {item(<Trash2 />, "Apagar", () => (onDelete(), onClose()), { danger: true })}
    </div>,
    document.body,
  );
}

/** Apagar: "para mim" (só some para você) ou "para todos" (no WhatsApp). */
export function DeleteDialog({ message: m, conversationId, onClose }: { message: Message; conversationId: string; onClose: () => void }) {
  const actions = useConversationActions();
  const caps = messageCapabilities(m);
  const [busy, setBusy] = useState<"me" | "everyone" | null>(null);
  const [error, setError] = useState("");

  async function run(scope: "me" | "everyone") {
    setBusy(scope);
    setError("");
    try {
      await actions.deleteMessage(conversationId, m.id, scope);
      onClose();
    } catch (e) {
      setError(errorMessage(e));
      setBusy(null);
    }
  }

  return (
    <Modal open onClose={() => !busy && onClose()} title="Apagar mensagem?">
      <p className="text-sm text-muted">
        {caps.deleteEveryone
          ? "Apagar para todos remove a mensagem também no WhatsApp do cliente. Apagar para mim esconde só para você."
          : "Apagar para mim esconde a mensagem só para você; o cliente e o restante da equipe continuam vendo."}
      </p>
      {error && <Alert>{error}</Alert>}
      <div className="flex flex-col items-stretch gap-2 sm:flex-row-reverse">
        {caps.deleteEveryone && (
          <Button variant="danger" loading={busy === "everyone"} disabled={!!busy} onClick={() => void run("everyone")}>
            Apagar para todos
          </Button>
        )}
        <Button variant="secondary" loading={busy === "me"} disabled={!!busy} onClick={() => void run("me")}>
          Apagar para mim
        </Button>
        <Button variant="ghost" disabled={!!busy} onClick={onClose}>
          Cancelar
        </Button>
      </div>
    </Modal>
  );
}

/** Quando foi enviada, entregue e lida (e por quem). */
export function MessageInfoDialog({ message: m, onClose }: { message: Message; onClose: () => void }) {
  const rows: [string, string][] = [["Enviada", `${new Date(m.sentAt).toLocaleDateString("pt-BR")} às ${formatTime(m.sentAt)}`]];
  if (m.direction === "out") {
    if (m.authorName) rows.unshift(["Enviada por", m.authorName]);
    if (m.status) rows.push(["Situação", statusLabel[m.status]]);
    if (m.deliveredAt) rows.push(["Entregue", formatTime(m.deliveredAt)]);
    if (m.readAt) rows.push(["Lida", formatTime(m.readAt)]);
  } else if (m.group && m.authorName) {
    rows.unshift(["Enviada por", m.authorName]);
  }
  if (m.editedAt) rows.push(["Editada", formatTime(m.editedAt)]);
  return (
    <Modal open onClose={onClose} title="Dados da mensagem">
      <dl className="flex flex-col gap-2.5 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-4">
            <dt className="text-muted">{k}</dt>
            <dd className="text-right font-medium">{v}</dd>
          </div>
        ))}
      </dl>
    </Modal>
  );
}

/** Escolha de até 5 conversas para encaminhar a mensagem. */
export function ForwardDialog({ message: m, conversationId, onClose }: { message: Message; conversationId: string; onClose: () => void }) {
  const { data } = useConversations();
  const actions = useConversationActions();
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [failures, setFailures] = useState<{ name: string; error: string }[]>([]);

  const options = useMemo(
    () => (data ?? []).filter((c) => c.id !== conversationId && c.status !== "closed" && matches(`${c.contact.name} ${c.contact.phone}`, q)),
    [data, conversationId, q],
  );

  function toggle(id: string) {
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : p.length >= 5 ? p : [...p, id]));
  }

  async function submit() {
    setBusy(true);
    setError("");
    setFailures([]);
    try {
      const res = await actions.forwardMessage(m.id, picked);
      const bad = res.filter((r) => !r.ok);
      if (bad.length === 0) return onClose();
      setFailures(bad.map((r) => ({ name: data?.find((c) => c.id === r.conversationId)?.contact.name ?? "Conversa", error: ("error" in r && r.error) || "Não foi possível encaminhar." })));
      setPicked(bad.map((r) => r.conversationId));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={() => !busy && onClose()} title="Encaminhar para">
      <input
        autoFocus
        aria-label="Buscar conversa"
        placeholder="Buscar conversa"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        className="h-10 rounded-lg border border-line bg-elevated px-3 text-sm outline-none focus:border-accent"
      />
      <ul className="-mx-1 max-h-72 overflow-y-auto">
        {options.length === 0 && <li className="px-2 py-6 text-center text-sm text-muted">Nenhuma conversa encontrada.</li>}
        {options.map((c) => {
          const on = picked.includes(c.id);
          return (
            <li key={c.id}>
              <label className="flex cursor-pointer items-center gap-3 rounded-lg px-2 py-2 hover:bg-hover">
                <input type="checkbox" checked={on} onChange={() => toggle(c.id)} className="size-4 accent-[var(--c-accent)]" />
                <Avatar name={c.contact.name} src={c.contact.avatarUrl} group={c.contact.kind === "group"} size="sm" />
                <span className="min-w-0 flex-1 truncate text-sm">{c.contact.name}</span>
              </label>
            </li>
          );
        })}
      </ul>
      {failures.length > 0 && (
        <Alert>
          {failures.map((f) => (
            <span key={f.name} className="block">{f.name}: {f.error}</span>
          ))}
        </Alert>
      )}
      {error && <Alert>{error}</Alert>}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted">{picked.length} de 5 selecionadas</span>
        <div className="flex gap-2">
          <Button variant="ghost" disabled={busy} onClick={onClose}>Cancelar</Button>
          <Button loading={busy} disabled={picked.length === 0} onClick={() => void submit()}>Encaminhar</Button>
        </div>
      </div>
    </Modal>
  );
}
