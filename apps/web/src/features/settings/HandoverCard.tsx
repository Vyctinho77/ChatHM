import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { InfoTip } from "@/components/InfoTip";
import { errorMessage, fieldErrors, post } from "@/lib/api";
import { useMe, useSetMe } from "@/features/auth/api";
import { CodeInput } from "@/features/auth/CodeInput";
import { setHandover } from "@/features/auth/handover";

/** Zona de perigo: entrega a instância a um novo cliente (apaga tudo e libera a instalação com um código único). */
export function HandoverCard() {
  const { data: me } = useMe();
  const qc = useQueryClient();
  const setMe = useSetMe();
  const [open, setOpen] = useState(false);
  const [confirm, setConfirm] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  function close() {
    setOpen(false);
    setConfirm("");
    setCode("");
    setError("");
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const r = await post<{ installCode: string }>("/office/handover", { confirm, code });
      setHandover({ code: r.installCode, url: window.location.origin });
      void qc.invalidateQueries({ queryKey: ["auth", "options"] });
      setMe(null); // a sessão foi encerrada; a tela de entrada mostra o código
    } catch (err) {
      setError(fieldErrors(err).confirm ?? errorMessage(err));
      setCode("");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="border-t border-line py-6">
      <div className="flex items-center gap-1.5">
        <h2 className="flex items-center gap-2 font-semibold text-danger">
          <TriangleAlert className="size-4" /> Entregar instância
        </h2>
        <InfoTip title="Entregar instância">
          Para passar este sistema a um novo cliente: desconecta o WhatsApp atual, apaga <strong>todos</strong> os dados e gera um código de instalação de uso único.
          Quem receber o endereço e o código vira o administrador do próprio escritório.
        </InfoTip>
      </div>
      <div className="mt-4">
        {me?.user.mfaEnabled ? (
          <Button variant="danger" onClick={() => setOpen(true)}>
            Entregar instância…
          </Button>
        ) : (
          <p className="text-sm text-muted">Requer o autenticador ativo na sua conta.</p>
        )}
      </div>

      <Modal open={open} onClose={close} title="Entregar instância">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <Alert>
            <strong>Apaga tudo e não pode ser desfeito:</strong> usuários, equipes, conversas, contatos, anexos e a sessão do WhatsApp
            (o aparelho será desvinculado).
          </Alert>
          {error && <Alert>{error}</Alert>}
          <Field
            label="Digite ENTREGAR para confirmar"
            autoComplete="off"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className="[&_input]:font-mono [&_input]:uppercase"
          />
          <CodeInput value={code} onChange={setCode} />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" variant="danger" loading={loading} disabled={confirm.trim().toUpperCase() !== "ENTREGAR" || code.length !== 6}>
              Apagar tudo e entregar
            </Button>
          </div>
        </form>
      </Modal>
    </section>
  );
}
