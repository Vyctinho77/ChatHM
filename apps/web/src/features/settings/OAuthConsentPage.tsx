import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Bot, EyeOff, ShieldCheck } from "lucide-react";
import { Alert, Button, Spinner } from "@/components/ui";
import { errorMessage, get, post } from "@/lib/api";

type Preview = { client: { id: string; name: string }; redirectHost: string; levels: ("read" | "write" | "send")[] };

const LEVELS: Record<string, { title: string; text: string }> = {
  read: { title: "Só ler (recomendado)", text: "Consultar conversas, mensagens, contatos, filas, etiquetas e agenda que você já enxerga." },
  write: { title: "Ler, alterar e deixar rascunhos", text: "Também etiquetar, acrescentar observações, adiar, lembretes, agenda, transferir e deixar rascunhos de resposta para você revisar." },
  send: { title: "Tudo isso e pedir envios (você aprova)", text: "Pode pedir o envio de mensagens a clientes; elas só saem quando você clicar em Enviar na conversa." },
};

/** Tela aberta pelo ChatGPT / claude.ai (via /oauth/authorize): a pessoa vê o que o aplicativo recebe e autoriza ou nega. */
export function OAuthConsentPage() {
  const query = new URLSearchParams(window.location.search);
  const raw = query.toString();
  const [level, setLevel] = useState<"read" | "write" | "send">("read");
  const [error, setError] = useState("");

  const preview = useQuery({
    queryKey: ["ai", "oauth-preview", raw],
    retry: false,
    queryFn: () => get<Preview>(`/ai/oauth/preview?${raw}`),
  });

  const decide = useMutation({
    mutationFn: (approve: boolean) =>
      post<{ redirectTo: string }>("/ai/oauth/authorize", {
        clientId: query.get("client_id"),
        redirectUri: query.get("redirect_uri"),
        codeChallenge: query.get("code_challenge"),
        codeChallengeMethod: query.get("code_challenge_method"),
        state: query.get("state") ?? "",
        scope: query.get("scope") ?? "",
        resource: query.get("resource") ?? "",
        level,
        approve,
      }),
    onSuccess: (r) => window.location.assign(r.redirectTo),
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto flex max-w-lg flex-col gap-5 px-4 py-10">
        <h1 className="flex items-center gap-2 text-xl font-semibold"><Bot className="size-6 text-brand-ink" /> Conectar aplicativo de IA</h1>
        {preview.isPending && <Spinner className="mx-auto my-8" />}
        {preview.isError && <Alert>{errorMessage(preview.error)}</Alert>}
        {preview.data && (
          <>
            <p className="text-sm">
              <strong>{preview.data.client.name}</strong> quer acessar o ChatHM <strong>em seu nome</strong>. Ele só vê e faz o que você já pode ver e fazer, e nunca envia mensagens sozinho.
            </p>

            <fieldset className="flex flex-col gap-2.5 text-sm">
              <legend className="mb-1 font-medium">O que ele pode fazer</legend>
              {preview.data.levels.map((l) => (
                <label key={l} className="flex cursor-pointer items-start gap-2.5">
                  <input type="radio" name="level" checked={level === l} onChange={() => setLevel(l)} className="mt-1 accent-[var(--c-accent)]" />
                  <span><span className="font-medium">{LEVELS[l]?.title}</span><span className="block text-xs text-muted">{LEVELS[l]?.text}</span></span>
                </label>
              ))}
            </fieldset>

            <div className="rounded-xl bg-elevated p-3.5 text-xs text-muted">
              <p className="flex items-center gap-1.5 font-medium text-fg"><EyeOff className="size-4" /> O que o aplicativo NÃO recebe</p>
              <ul className="mt-1.5 list-disc pl-5">
                <li>Telefones, e-mails, CEPs e documentos (CPF, CNPJ, RG, CNH…) dos clientes: aparecem ocultos.</li>
                <li>Senhas, códigos, chaves de acesso, cartões, contas bancárias, chave PIX e boletos.</li>
                <li>Observações internas dos contatos, fotos, áudios e arquivos.</li>
              </ul>
            </div>

            <p className="flex items-start gap-1.5 text-xs text-muted">
              <ShieldCheck className="mt-0.5 size-4 shrink-0" />
              Depois de autorizar, você volta para <strong>{preview.data.redirectHost}</strong>. Você pode desconectar o aplicativo quando quiser na aba MCP.
            </p>
            {error && <Alert>{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" disabled={decide.isPending} onClick={() => (setError(""), decide.mutate(false))}>Cancelar</Button>
              <Button type="button" loading={decide.isPending && decide.variables === true} disabled={decide.isPending} onClick={() => (setError(""), decide.mutate(true))}>Autorizar</Button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
