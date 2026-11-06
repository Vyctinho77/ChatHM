import { useState } from "react";
import { Copy } from "lucide-react";
import { Alert, Button } from "@/components/ui";
import { AuthLayout } from "./AuthLayout";
import type { HandoverResult } from "./handover";

/** Mostrado uma única vez depois de entregar a instância: o código não é gravado em lugar nenhum. */
export function HandoverDone({ result, onClose }: { result: HandoverResult; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const msg = `Seu ChatHM está pronto. Acesse ${result.url} e use o código de instalação ${result.code} para criar o administrador e conectar o WhatsApp do escritório.`;
  return (
    <AuthLayout title="Instância entregue" subtitle="Tudo foi apagado e o WhatsApp foi desconectado.">
      <div className="flex flex-col gap-4">
        <p className="text-sm text-muted">Envie ao novo cliente o endereço e este código. Ele vale uma única vez e não aparece de novo.</p>
        <div className="rounded-xl border border-line bg-elevated p-3 text-center">
          <p className="text-xs text-muted">{result.url}</p>
          <code className="mt-1 block font-mono text-2xl tracking-widest">{result.code}</code>
        </div>
        <Button type="button" variant="secondary" onClick={() => void navigator.clipboard?.writeText(msg).then(() => setCopied(true))}>
          <Copy className="size-4" /> {copied ? "Mensagem copiada" : "Copiar mensagem pronta"}
        </Button>
        <Alert tone="info">Não conclua a instalação nesta tela: o primeiro a usar o código vira o administrador.</Alert>
        <Button type="button" className="h-10" onClick={onClose}>
          Já copiei, fechar
        </Button>
      </div>
    </AuthLayout>
  );
}
