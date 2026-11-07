import { useRef, useState } from "react";
import { Check, Pencil, X } from "lucide-react";
import { IconButton } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { cn } from "@/lib/cn";

/**
 * Nome com lápis: clique para editar; Enter salva, Esc cancela. Nome vazio devolve o nome automático do WhatsApp.
 */
export function EditableName({
  value,
  onSave,
  label = "Editar nome",
  className,
}: {
  value: string;
  onSave: (name: string) => Promise<void> | void;
  label?: string;
  className?: string;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const done = useRef(false); // evita salvar duas vezes (Enter + blur)

  function start() {
    done.current = false;
    setDraft(value);
    setError("");
    setEditing(true);
  }

  async function save() {
    if (done.current) return;
    const next = draft.trim();
    if (next === value.trim()) {
      done.current = true;
      setEditing(false);
      return;
    }
    setSaving(true);
    setError("");
    try {
      await onSave(next);
      done.current = true;
      setEditing(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  }

  function cancel() {
    done.current = true;
    setEditing(false);
    setError("");
  }

  if (!editing) {
    return (
      <span className={cn("group inline-flex min-w-0 max-w-full items-center gap-1", className)}>
        <span className="truncate">{value}</span>
        <IconButton label={label} onClick={start} className="size-6 shrink-0 opacity-60 hover:opacity-100 focus-visible:opacity-100">
          <Pencil className="size-3.5" />
        </IconButton>
      </span>
    );
  }
  return (
    <span className={cn("inline-flex min-w-0 max-w-full flex-col items-stretch", className)}>
      <span className="flex items-center gap-1">
        <input
          autoFocus
          aria-label="Nome do contato"
          value={draft}
          maxLength={120}
          disabled={saving}
          placeholder="Nome (vazio = nome do WhatsApp)"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void save();
            if (e.key === "Escape") cancel();
          }}
          className="h-8 min-w-0 flex-1 rounded-lg border border-line bg-elevated px-2.5 text-[13.5px] font-normal outline-none focus:border-accent"
        />
        <IconButton label="Salvar nome" onClick={() => void save()} disabled={saving} className="size-7 shrink-0">
          <Check className="size-4" />
        </IconButton>
        <IconButton label="Cancelar" onClick={cancel} disabled={saving} className="size-7 shrink-0">
          <X className="size-4" />
        </IconButton>
      </span>
      {error && <span role="alert" className="mt-1 text-xs text-danger">{error}</span>}
    </span>
  );
}
