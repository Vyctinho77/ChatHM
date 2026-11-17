import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CircleCheck, ScanSearch } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { errorMessage, fieldErrors, get, post } from "@/lib/api";
import { describeReport, plural, type DedupeReport } from "./dedupeReport";

/**
 * Corrige dados repetidos (mensagens, arquivos e contatos), efeito de reconectar o WhatsApp: o mesmo chat chegou por dois endereços.
 * Primeiro mostra o que seria feito (sem gravar nada); só aplica depois de a pessoa digitar CORRIGIR.
 */
export function DuplicatesCard() {
  const qc = useQueryClient();
  const [report, setReport] = useState<DedupeReport | null>(null);
  const [done, setDone] = useState<DedupeReport | null>(null);
  const [checking, setChecking] = useState(false);
  const [open, setOpen] = useState(false);
  const [confirm, setConfirm] = useState("");
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");

  async function check() {
    setChecking(true);
    setError("");
    setDone(null);
    try {
      setReport(await get<DedupeReport>("/maintenance/duplicates"));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setChecking(false);
    }
  }

  function close() {
    setOpen(false);
    setConfirm("");
    setError("");
  }

  async function apply() {
    setApplying(true);
    setError("");
    try {
      const r = await post<DedupeReport>("/maintenance/duplicates", { confirm: confirm.trim().toUpperCase() });
      setDone(r);
      setReport(null);
      close();
      void qc.invalidateQueries(); // conversas, contatos e mensagens mudaram
    } catch (e) {
      setError(fieldErrors(e).confirm ?? errorMessage(e));
    } finally {
      setApplying(false);
    }
  }

  const lines = report ? describeReport(report) : [];

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="secondary" loading={checking} onClick={() => void check()}>
          <ScanSearch className="size-4" /> {report || done ? "Verificar de novo" : "Verificar duplicatas"}
        </Button>
        {report && lines.length > 0 && (
          <Button variant="danger" onClick={() => setOpen(true)}>
            Corrigir agora…
          </Button>
        )}
      </div>

      {error && !open && <Alert>{error}</Alert>}

      {report && lines.length === 0 && (
        <p className="flex items-center gap-2 text-sm text-muted">
          <CircleCheck className="size-4 text-brand-ink" /> Nenhuma duplicata encontrada.
        </p>
      )}
      {report && lines.length > 0 && (
        <div className="flex flex-col gap-2 text-sm">
          <p className="font-medium">Encontrado:</p>
          <ul className="list-disc space-y-1 pl-5 text-muted">
            {lines.map((l) => (
              <li key={l}>{l}</li>
            ))}
          </ul>
          {report.contactsSkipped > 0 && (
            <Alert tone="info">
              {plural(report.contactsSkipped, "contato não pôde", "contatos não puderam")} ser unido{report.contactsSkipped === 1 ? "" : "s"} automaticamente (conversa aberta nos dois lados) e precisa{report.contactsSkipped === 1 ? "" : "m"} de revisão manual.
            </Alert>
          )}
        </div>
      )}
      {done && (
        <Alert tone="info">
          <strong>Pronto.</strong> {describeReport(done).join(" ") || "Nada precisou ser corrigido."}
        </Alert>
      )}

      <Modal open={open} onClose={close} title="Corrigir duplicatas">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void apply();
          }}
          className="flex flex-col gap-4"
        >
          <Alert>
            <strong>Apaga as cópias e não pode ser desfeito.</strong> Fica uma cópia de cada mensagem (a que tem o autor, o arquivo e a hora original); os contatos
            repetidos são unidos. Se puder, faça um backup do banco antes.
          </Alert>
          {report && (
            <ul className="list-disc space-y-1 pl-5 text-sm text-muted">
              {describeReport(report).map((l) => (
                <li key={l}>{l}</li>
              ))}
            </ul>
          )}
          {error && <Alert>{error}</Alert>}
          <Field label="Digite CORRIGIR para confirmar" autoComplete="off" value={confirm} onChange={(e) => setConfirm(e.target.value)} className="[&_input]:font-mono [&_input]:uppercase" />
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" variant="danger" loading={applying} disabled={confirm.trim().toUpperCase() !== "CORRIGIR"}>
              Corrigir
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
