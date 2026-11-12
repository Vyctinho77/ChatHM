import { useState } from "react";
import { Alert, Button, Modal } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import type { Message } from "@/features/conversations/types";
import { useDeleteInternal } from "./api";

/** Apagar sempre vale para todos: o conteúdo e o anexo somem. Quem apaga a mensagem de outra pessoa fica registrado na auditoria (sem o texto). */
export function InternalDeleteDialog({ message: m, mine, onClose }: { message: Message; mine: boolean; onClose: () => void }) {
  const del = useDeleteInternal();
  const [error, setError] = useState("");
  async function run() {
    setError("");
    try {
      await del.mutateAsync(m.id);
      onClose();
    } catch (e) {
      setError(errorMessage(e));
    }
  }
  return (
    <Modal open onClose={() => !del.isPending && onClose()} title="Apagar mensagem?">
      <p className="text-sm text-muted">
        A mensagem some para todo o escritório.
        {!mine && <> Como você é gestor ou administrador, isso fica registrado na auditoria (sem o conteúdo da mensagem).</>}
      </p>
      {error && <Alert>{error}</Alert>}
      <div className="flex flex-col items-stretch gap-2 sm:flex-row-reverse">
        <Button variant="danger" loading={del.isPending} onClick={() => void run()}>Apagar para todos</Button>
        <Button variant="secondary" disabled={del.isPending} onClick={onClose}>Cancelar</Button>
      </div>
    </Modal>
  );
}
