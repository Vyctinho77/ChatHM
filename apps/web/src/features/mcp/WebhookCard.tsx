import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, KeyRound, Send } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { api, errorMessage, fieldErrors, get, post } from "@/lib/api";
import { Skeleton } from "@/components/Skeleton";
import { CopyButton, Toggle } from "./AiAccessCard";

type Webhook = {
  enabled: boolean;
  url: string;
  onlyAiReplies: boolean;
  includePreview: boolean;
  hasSecret: boolean;
  consecutiveFailures: number;
  lastDeliveryAt?: string;
  lastStatus?: string;
  lastError?: string;
  disabledReason?: string;
  pending: number;
};
type Saved = { webhook: Webhook; secret?: string };
type TestResult = { ok: boolean; status?: number; error?: string; durationMs: number };

const key = ["ai", "webhook"] as const;
const when = (iso: string) => new Date(iso).toLocaleString("pt-BR");

/** Mostra o segredo uma única vez: depois disso ele só existe cifrado no servidor. */
function SecretModal({ secret, onClose }: { secret: string; onClose: () => void }) {
  return (
    <Modal open onClose={onClose} title="Segredo do webhook" className="max-w-lg">
      <div className="space-y-4 text-[13px] leading-relaxed">
        <p>
          Guarde este segredo agora: <strong>ele não será mostrado de novo</strong>. Quem recebe os avisos usa o segredo para conferir a assinatura de cada um.
        </p>
        <div className="flex items-center gap-2 rounded-lg border border-line bg-elevated p-2.5">
          <code className="min-w-0 flex-1 break-all text-xs">{secret}</code>
          <CopyButton text={secret} label="Copiar segredo" />
        </div>
        <div className="flex justify-end">
          <Button onClick={onClose}>Já guardei</Button>
        </div>
      </div>
    </Modal>
  );
}

export function WebhookCard() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: key, queryFn: () => get<Webhook>("/ai/webhook") });
  const settings = useQuery({ queryKey: ["ai", "settings"], queryFn: () => get<{ enabled: boolean }>("/ai/settings") });
  const [secret, setSecret] = useState("");
  const [confirmRotate, setConfirmRotate] = useState(false);
  const [error, setError] = useState("");
  const [test, setTest] = useState<TestResult | null>(null);

  if (!q.data) {
    return (
      <section id="aviso-automatico" className="scroll-mt-6">
        {q.isPending ? <Skeleton className="h-24 w-full" /> : <Alert>Não foi possível carregar o aviso automático. {errorMessage(q.error)}</Alert>}
      </section>
    );
  }
  const w = q.data;

  return (
    <section id="aviso-automatico" className="scroll-mt-6">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <BellRing className="size-4" aria-hidden /> Aviso automático (webhook)
      </h2>
      <p className="mt-2 text-xs leading-relaxed text-muted">
        Quando o cliente responde a uma mensagem enviada por IA, o ChatHM avisa o endereço abaixo, sem esperar o agente perguntar. O aviso diz <strong className="font-medium">quem
        respondeu, em qual conversa e fila</strong>; o texto da mensagem não vai (o agente lê pelo MCP, com todas as proteções de privacidade, usando{" "}
        <code>list_pending_replies</code>).
      </p>
      {settings.data && !settings.data.enabled && <div className="mt-3"><Alert tone="info">O acesso por IA está desligado: nenhum aviso é enviado enquanto ele não for ligado acima.</Alert></div>}
      {w.disabledReason && <div className="mt-3"><Alert>{w.disabledReason}</Alert></div>}
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}

      <WebhookForm
        key={`${w.url}|${w.enabled}|${w.onlyAiReplies}|${w.includePreview}`}
        webhook={w}
        onSaved={(r) => {
          setError("");
          setTest(null);
          qc.setQueryData(key, r.webhook);
          if (r.secret) setSecret(r.secret);
        }}
        onError={(m) => setError(m)}
      />

      {w.hasSecret && (
        <div className="mt-5 space-y-3 border-t border-line/60 pt-4">
          <div className="flex flex-wrap items-center gap-2">
            <TestButton
              disabled={!w.url}
              onResult={(r) => {
                setTest(r);
                void qc.invalidateQueries({ queryKey: key });
              }}
              onError={(m) => setError(m)}
            />
            <Button type="button" variant="ghost" onClick={() => setConfirmRotate(true)}>
              <KeyRound className="size-4" /> Trocar segredo
            </Button>
          </div>
          {test && (
            <p role="status" className="text-xs text-muted">
              {test.ok ? `Teste entregue (resposta ${test.status}, ${test.durationMs} ms). Confira a assinatura no receptor.` : `Teste falhou: ${test.error || "sem resposta"}${test.status ? ` (resposta ${test.status})` : ""}.`}
            </p>
          )}
          <p className="text-xs leading-relaxed text-muted">
            {w.lastDeliveryAt ? `Último aviso: ${when(w.lastDeliveryAt)} · resposta ${w.lastStatus || "—"}` : "Nenhum aviso enviado ainda"}
            {w.pending > 0 ? ` · ${w.pending} aguardando envio` : ""}
            {w.lastError ? ` · último erro: ${w.lastError}` : ""}
          </p>
        </div>
      )}

      <details className="mt-4 text-xs leading-relaxed text-muted">
        <summary className="cursor-pointer text-fg">Como conferir a assinatura no receptor</summary>
        <div className="mt-2 space-y-2">
          <p>
            Cada aviso é um POST em JSON com o cabeçalho <code>X-ChatHM-Signature: t=&lt;unix&gt;,v1=&lt;hex&gt;</code>. Calcule o HMAC-SHA256 do texto{" "}
            <code>&lt;t&gt;.&lt;corpo exato&gt;</code> com o segredo e compare com <code>v1</code>; recuse se <code>t</code> for de mais de 5 minutos atrás.
          </p>
          <p>
            Outros cabeçalhos: <code>X-ChatHM-Event</code> (<code>message.received</code> ou <code>webhook.test</code>) e <code>X-ChatHM-Delivery</code> (id, para ignorar repetidos). Responda 2xx
            para confirmar; senão o ChatHM tenta de novo (10 s, 1 min, 5 min, 30 min e 2 h). Respostas em rajada chegam juntas num só aviso, com a contagem.
          </p>
        </div>
      </details>

      {secret && <SecretModal secret={secret} onClose={() => setSecret("")} />}
      {confirmRotate && (
        <RotateModal
          onClose={() => setConfirmRotate(false)}
          onRotated={(s) => {
            setConfirmRotate(false);
            setSecret(s);
          }}
          onError={(m) => {
            setConfirmRotate(false);
            setError(m);
          }}
        />
      )}
    </section>
  );
}

function WebhookForm({ webhook: w, onSaved, onError }: { webhook: Webhook; onSaved: (r: Saved) => void; onError: (m: string) => void }) {
  const [url, setUrl] = useState(w.url);
  const [enabled, setEnabled] = useState(w.enabled);
  const [onlyAi, setOnlyAi] = useState(w.onlyAiReplies);
  const [preview, setPreview] = useState(w.includePreview);
  const save = useMutation({
    mutationFn: () => api<Saved>("PUT", "/ai/webhook", { enabled, url: url.trim(), onlyAiReplies: onlyAi, includePreview: preview }),
    onSuccess: onSaved,
    onError: (e) => {
      if (!fieldErrors(e).url) onError(errorMessage(e));
    },
  });
  const fields = fieldErrors(save.error);
  const dirty = url !== w.url || enabled !== w.enabled || onlyAi !== w.onlyAiReplies || preview !== w.includePreview;

  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
      className="mt-3 divide-y divide-line/60"
    >
      <Toggle checked={enabled} onChange={setEnabled} label="Avisar quando o cliente responder" hint="Desligado, nenhum aviso é enviado e os que estavam esperando são descartados." />
      <div className="py-3">
        <Field
          label="Endereço que recebe os avisos"
          type="url"
          inputMode="url"
          placeholder="https://meu-servidor.com.br/chathm"
          value={url}
          error={fields.url}
          hint="Precisa ser https e público (endereços da rede interna são recusados)."
          onChange={(e) => setUrl(e.target.value)}
        />
      </div>
      <Toggle checked={onlyAi} onChange={setOnlyAi} label="Só respostas a mensagens enviadas por IA" hint="Ligado (recomendado): ignora conversas em que nenhuma IA falou. Desligado: avisa toda mensagem de cliente." />
      <Toggle
        checked={preview}
        onChange={setPreview}
        label="Incluir um trecho da mensagem"
        hint="Desligado por padrão. Ligado, o aviso leva até 160 caracteres, com senhas, documentos, telefones e e-mails ocultados; o trecho sai do ChatHM para o receptor."
      />
      <div className="flex justify-end gap-2 py-3">
        <Button type="submit" loading={save.isPending} disabled={!dirty || (enabled && !url.trim())}>
          Salvar
        </Button>
      </div>
    </form>
  );
}

function TestButton({ disabled, onResult, onError }: { disabled: boolean; onResult: (r: TestResult) => void; onError: (m: string) => void }) {
  const test = useMutation({ mutationFn: () => post<TestResult>("/ai/webhook/test"), onSuccess: onResult, onError: (e) => onError(errorMessage(e)) });
  return (
    <Button type="button" variant="secondary" loading={test.isPending} disabled={disabled} onClick={() => test.mutate()}>
      <Send className="size-4" /> Enviar teste
    </Button>
  );
}

function RotateModal({ onClose, onRotated, onError }: { onClose: () => void; onRotated: (secret: string) => void; onError: (m: string) => void }) {
  const rotate = useMutation({
    mutationFn: () => post<{ secret: string }>("/ai/webhook/secret"),
    onSuccess: (r) => onRotated(r.secret),
    onError: (e) => onError(errorMessage(e)),
  });
  return (
    <Modal open onClose={() => { if (!rotate.isPending) onClose(); }} title="Trocar o segredo?" className="max-w-md">
      <div className="space-y-4 text-[13px] leading-relaxed">
        <p>O segredo atual para de valer na hora: quem confere a assinatura precisa passar a usar o novo. Os avisos já enviados não são afetados.</p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" disabled={rotate.isPending} onClick={onClose}>Cancelar</Button>
          <Button type="button" loading={rotate.isPending} onClick={() => rotate.mutate()}>Trocar segredo</Button>
        </div>
      </div>
    </Modal>
  );
}
