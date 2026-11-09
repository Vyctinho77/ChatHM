import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Modal } from "@/components/ui";
import { ApiError, errorMessage, patch } from "@/lib/api";
import { meKey, useMe, type Me } from "@/features/auth/api";

/** Nome que aparece acima das mensagens da pessoa (no app e para o cliente no WhatsApp). */
export function SignatureDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  // O conteúdo só existe com o diálogo aberto, então o campo recomeça do nome salvo a cada abertura.
  return (
    <Modal open={open} onClose={onClose} title="Nome nas mensagens">
      <Editor onClose={onClose} />
    </Modal>
  );
}

function Editor({ onClose }: { onClose: () => void }) {
  const me = useMe().data;
  const qc = useQueryClient();
  const accountName = me?.user.name ?? "";
  const saved = me?.user.signatureName ?? "";
  const [name, setName] = useState(saved);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const shown = name.trim().replace(/\s+/g, " ") || accountName;
  const changed = name.trim().replace(/\s+/g, " ") !== saved;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      const res = await patch<{ signatureName: string }>("/me/preferences", { signatureName: name });
      qc.setQueryData<Me | null>(meKey, (m) => (m ? { ...m, user: { ...m.user, signatureName: res.signatureName } } : m));
      onClose();
    } catch (err) {
      setError(err instanceof ApiError && err.fields.signatureName ? err.fields.signatureName : errorMessage(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      {error && <Alert>{error}</Alert>}
      <p className="text-[13px] leading-relaxed text-muted">
        É o nome em negrito que o cliente vê acima das suas mensagens no WhatsApp (a equipe o vê em “Enviada por”). O nome da sua conta não muda,
        e as mensagens já enviadas continuam como estavam.
      </p>
      <label className="flex flex-col gap-1.5 text-[13px] font-medium">
        Como você quer aparecer
        <input
          value={name}
          autoFocus
          maxLength={40}
          placeholder={accountName}
          onChange={(e) => setName(e.target.value)}
          className="h-10 rounded-lg border border-line bg-elevated px-3 text-[14px] font-normal outline-none placeholder:text-muted/70 focus:border-brand"
        />
      </label>

      <div>
        <p className="mb-2 text-xs font-medium text-muted">Como o cliente vai ver</p>
        <div className="chat-wallpaper rounded-xl border border-line p-4">
          <div className="ml-auto max-w-[80%] rounded-lg bg-out px-3 py-2 text-[13px] text-out-fg">
            <strong>{shown}:</strong>
            <br />
            Bom dia! Já estou verificando para você.
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between gap-2 border-t border-line pt-4">
        <Button type="button" variant="ghost" disabled={saved === "" && name === ""} onClick={() => setName("")}>
          Usar o nome da conta
        </Button>
        <div className="flex gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" loading={saving} disabled={!changed}>
            Salvar
          </Button>
        </div>
      </div>
    </form>
  );
}
