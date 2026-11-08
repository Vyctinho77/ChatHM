import { useState, type CSSProperties } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Modal } from "@/components/ui";
import { errorMessage, patch } from "@/lib/api";
import { cn } from "@/lib/cn";
import {
  appearanceVars,
  presets,
  sameAppearance,
  sanitize,
  setAppearance,
  useAppearance,
  type Appearance,
} from "@/lib/chatAppearance";
import { meKey, type Me } from "@/features/auth/api";

/** Valor efetivo de uma variável do tema atual (para mostrar o padrão nos controles). */
function themeVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function defaults() {
  return {
    wallpaper: Math.round(parseFloat(themeVar("--wallpaper-i") || "0.3") * 100),
    out: themeVar("--c-out") || "#5f6e24",
    in: themeVar("--c-in") || "#ffffff",
  };
}

export function ChatAppearanceDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  // O conteúdo só existe com o diálogo aberto, então o rascunho recomeça da aparência salva a cada abertura.
  return (
    <Modal open={open} onClose={onClose} title="Aparência do chat" className="max-w-lg">
      <Editor onClose={onClose} />
    </Modal>
  );
}

function Editor({ onClose }: { onClose: () => void }) {
  const saved = useAppearance();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Appearance>(saved);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const base = defaults();

  const shown = { wallpaper: draft.wallpaper ?? base.wallpaper, out: draft.out ?? base.out, in: draft.in ?? base.in };
  const previewStyle = appearanceVars(draft) as CSSProperties;

  async function save() {
    setSaving(true);
    setError("");
    const next = sanitize(draft);
    try {
      await patch("/me/preferences", { chatAppearance: next });
      setAppearance(next);
      qc.setQueryData<Me | null>(meKey, (me) => (me ? { ...me, user: { ...me.user, chatAppearance: next } } : me));
      onClose();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  }

  const change = (patch: Appearance) => setDraft((d) => ({ ...d, ...patch }));
  const clear = (key: "out" | "in") =>
    setDraft((d) => {
      const next = { ...d };
      delete next[key];
      return next;
    });

  return (
    <div className="flex flex-col gap-4">
      {error && <Alert>{error}</Alert>}

      {/* Prévia ao vivo: usa o mesmo fundo e as mesmas classes dos balões reais. */}
      <div className="chat-wallpaper flex flex-col gap-2 rounded-xl border border-line p-4" style={previewStyle} aria-label="Prévia">
        <div className="max-w-[75%] self-start rounded-lg bg-in px-3 py-2 text-[13px] text-in-fg">Bom dia! Preciso da guia do imposto.</div>
        <div className="max-w-[75%] self-end rounded-lg bg-out px-3 py-2 text-[13px] text-out-fg">Claro, já envio para você.</div>
      </div>

      <div>
        <p className="mb-2 text-xs font-medium text-muted">Estilos prontos</p>
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
          {presets.map((p) => {
            const active = sameAppearance(sanitize(p.value), sanitize(draft));
            return (
              <button
                key={p.id}
                type="button"
                aria-pressed={active}
                onClick={() => setDraft(p.value)}
                className={cn(
                  "flex flex-col items-center gap-1.5 rounded-lg border p-2 text-[11px] transition-colors hover:bg-hover",
                  active ? "border-brand bg-brand/10 text-brand-ink" : "border-line text-muted",
                )}
              >
                <span className="flex gap-0.5" aria-hidden>
                  <span className="size-4 rounded-full border border-line" style={{ background: p.value.in ?? base.in }} />
                  <span className="size-4 rounded-full border border-line" style={{ background: p.value.out ?? base.out }} />
                </span>
                {p.label}
              </button>
            );
          })}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <ColorField label="Balão que eu envio" value={shown.out} custom={draft.out !== undefined} onChange={(out) => change({ out })} onReset={() => clear("out")} />
        <ColorField label="Balão do cliente" value={shown.in} custom={draft.in !== undefined} onChange={(v) => change({ in: v })} onReset={() => clear("in")} />
      </div>

      <div className="flex flex-col gap-1.5">
        <label htmlFor="wallpaper-range" className="flex items-center justify-between text-xs font-medium text-muted">
          <span>Intensidade do desenho de fundo</span>
          <span className="tabular-nums">{shown.wallpaper === 0 ? "Liso" : `${shown.wallpaper}%`}</span>
        </label>
        <input
          id="wallpaper-range"
          type="range"
          min={0}
          max={100}
          step={1}
          value={shown.wallpaper}
          onChange={(e) => change({ wallpaper: Number(e.target.value) })}
          className="w-full accent-brand"
        />
      </div>

      <p className="text-xs text-muted">A cor do texto nos balões é escolhida automaticamente para ficar legível. Suas escolhas valem em qualquer aparelho em que você entrar.</p>

      <div className="flex items-center justify-between gap-2 border-t border-line pt-4">
        <Button type="button" variant="ghost" onClick={() => setDraft({})} disabled={Object.keys(draft).length === 0}>
          Restaurar padrão
        </Button>
        <div className="flex gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="button" onClick={save} loading={saving} disabled={sameAppearance(sanitize(draft), saved)}>
            Salvar
          </Button>
        </div>
      </div>
    </div>
  );
}

function ColorField({
  label,
  value,
  custom,
  onChange,
  onReset,
}: {
  label: string;
  value: string;
  custom: boolean;
  onChange: (hex: string) => void;
  onReset: () => void;
}) {
  return (
    <div className="flex flex-col gap-1.5 text-xs font-medium text-muted">
      <span>{label}</span>
      <div className="flex items-center gap-2">
        <input
          type="color"
          aria-label={label}
          value={/^#[0-9a-f]{6}$/i.test(value) ? value : "#ffffff"}
          onChange={(e) => onChange(e.target.value)}
          className="h-9 w-12 cursor-pointer rounded-md border border-line bg-transparent p-0.5"
        />
        <span className="font-mono text-[12px] uppercase text-fg">{value}</span>
        {custom && (
          <button type="button" onClick={onReset} className="ml-auto text-[11px] font-normal text-brand-ink hover:underline">
            Usar padrão
          </button>
        )}
      </div>
    </div>
  );
}
