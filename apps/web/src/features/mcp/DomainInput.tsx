import { useId, useRef, useState } from "react";
import { LoaderCircle, Plus, X } from "lucide-react";
import { errorMessage, fieldErrors } from "@/lib/api";

export function DomainInput({ domains, disabled, onSave }: {
  domains: string[];
  disabled?: boolean;
  onSave: (domains: string[]) => Promise<unknown>;
}) {
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const busy = useRef(false);
  const id = useId();
  const locked = disabled || saving;

  async function save(next: string[], clearDraft: boolean) {
    if (locked || busy.current) return;
    busy.current = true;
    setSaving(true); setError("");
    try {
      await onSave(next);
      if (clearDraft) setDraft("");
    } catch (e) {
      setError(fieldErrors(e).oauthRedirectHosts ?? errorMessage(e));
    } finally {
      busy.current = false;
      setSaving(false);
      requestAnimationFrame(() => input.current?.focus());
    }
  }

  function add(value = draft) {
    const entries = value.toLowerCase().split(/[\s,;]+/).filter(Boolean);
    if (!entries.length) return;
    const next = [...new Set([...domains, ...entries])];
    if (next.length === domains.length) { setDraft(""); setError(""); return; }
    void save(next, true);
  }

  return <div className="pb-1">
    <div className="rounded-xl border border-line bg-elevated p-2 transition-colors focus-within:border-brand">
      {domains.length > 0 && <ul aria-label="Domínios autorizados" className="flex flex-wrap gap-2">
        {domains.map(domain => <li key={domain} className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-brand/20 bg-brand/10 py-1 pl-3 pr-1 text-xs text-brand-ink">
          <span className="min-w-0 break-all">{domain}</span>
          <button type="button" disabled={locked} aria-label={`Remover domínio ${domain}`} onClick={() => void save(domains.filter(d => d !== domain), false)} className="flex size-6 shrink-0 items-center justify-center rounded-full outline-none hover:bg-brand/15 focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-40"><X className="size-3.5" /></button>
        </li>)}
      </ul>}
      <div className="flex items-center gap-2">
        <input ref={input} value={draft} disabled={locked} aria-label="Adicionar domínio" aria-invalid={!!error} aria-describedby={`${id}-hint${error ? ` ${id}-error` : ""}`} placeholder="Adicionar domínio…" autoComplete="off" autoCapitalize="none" spellCheck={false}
          onChange={e => { setDraft(e.target.value); setError(""); }}
          onKeyDown={e => {
            if (e.nativeEvent.isComposing) return;
            if (["Enter", ",", ";", " "].includes(e.key)) { e.preventDefault(); add(); }
          }}
          onPaste={e => {
            const pasted = e.clipboardData.getData("text");
            if (pasted.trim().split(/[\s,;]+/).length > 1) { e.preventDefault(); const value = `${draft} ${pasted}`; setDraft(value); add(value); }
          }}
          className="h-10 min-w-0 flex-1 bg-transparent px-1 text-sm outline-none placeholder:text-muted disabled:opacity-50" />
        <span className="text-[11px] tabular-nums text-muted">{domains.length}/10</span>
        <button type="button" disabled={locked || !draft.trim()} aria-label="Adicionar domínio à lista" onClick={() => add()} className="flex size-8 shrink-0 items-center justify-center rounded-lg text-brand-ink outline-none hover:bg-brand/10 focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-40">{saving ? <LoaderCircle className="size-4 animate-spin" /> : <Plus className="size-4" />}</button>
      </div>
    </div>
    <p id={`${id}-hint`} className="mt-2 text-xs leading-relaxed text-muted">Enter ou vírgula para adicionar. Até 10 domínios, sem https:// ou caminhos.</p>
    {error && <p id={`${id}-error`} role="alert" className="mt-2 text-xs text-danger">{error}</p>}
  </div>;
}
