import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, KeyRound, Plus, Trash2 } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { api, del, errorMessage, fieldErrors, get, post } from "@/lib/api";
import { cn } from "@/lib/cn";
import { useMe } from "@/features/auth/api";

type AiSettings = { enabled: boolean; maskDocuments: boolean; maskPhones: boolean; allowWrite: boolean };
type AiToken = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expiresAt: string;
  createdAt: string;
  lastUsedAt?: string;
  revoked: boolean;
  expired: boolean;
  owner?: string;
};
type Created = { token: string; info: AiToken };

const settingsKey = ["ai", "settings"] as const;
const tokensKey = (all: boolean) => ["ai", "tokens", all] as const;
const date = (iso: string) => new Date(iso).toLocaleDateString("pt-BR");

function Toggle({ checked, onChange, label, hint, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; hint?: string; disabled?: boolean }) {
  return (
    <label className={cn("flex items-start gap-3 py-1.5", disabled ? "opacity-60" : "cursor-pointer")}>
      <input type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="mt-1 size-4 accent-[var(--c-accent)]" />
      <span>
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-muted">{hint}</span>}
      </span>
    </label>
  );
}

function CopyButton({ text, label = "Copiar" }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  return (
    <Button
      type="button"
      variant="secondary"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setDone(true);
          setTimeout(() => setDone(false), 1800);
        } catch {
          /* sem permissão de área de transferência: a pessoa copia à mão */
        }
      }}
    >
      {done ? <Check className="size-4" /> : <Copy className="size-4" />} {done ? "Copiado" : label}
    </Button>
  );
}

const TABS = [
  ["claude-code", "Claude Code"],
  ["claude-desktop", "Claude Desktop"],
  ["grok", "Grok"],
  ["chatgpt", "ChatGPT"],
  ["outros", "Outros / teste"],
] as const;

function Snippet({ text, copyLabel }: { text: string; copyLabel: string }) {
  return (
    <div className="mt-2">
      <pre className="overflow-x-auto rounded-lg bg-elevated p-3 text-xs">{text}</pre>
      <div className="mt-1.5"><CopyButton text={text} label={copyLabel} /></div>
    </div>
  );
}

/** Instruções de conexão por cliente de IA. Com `token`, já vêm preenchidas; sem ele, mostram SEU_TOKEN. */
function ConnectGuide({ token }: { token?: string }) {
  const [tab, setTab] = useState<(typeof TABS)[number][0]>("claude-code");
  const url = `${window.location.origin}/mcp`;
  const t = token ?? "SEU_TOKEN";
  const claude = `claude mcp add --transport http chathm ${url} --header "Authorization: Bearer ${t}"`;
  const desktop = JSON.stringify(
    { mcpServers: { chathm: { command: "npx", args: ["-y", "mcp-remote", url, "--header", "Authorization:${AUTH_HEADER}"], env: { AUTH_HEADER: `Bearer ${t}` } } } },
    null,
    2,
  );
  const grok = JSON.stringify({ type: "mcp", server_label: "chathm", server_url: url, authorization: t }, null, 2);
  const curl = `curl -s ${url} \\\n  -H "Authorization: Bearer ${t}" \\\n  -H "Content-Type: application/json" \\\n  -H "Accept: application/json, text/event-stream" \\\n  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'`;
  return (
    <div className="text-sm">
      <div role="tablist" aria-label="Cliente de IA" className="flex flex-wrap gap-1">
        {TABS.map(([key, label]) => (
          <button
            key={key}
            type="button"
            role="tab"
            aria-selected={tab === key}
            onClick={() => setTab(key)}
            className={cn("rounded-lg px-2.5 py-1 text-xs transition-colors", tab === key ? "bg-selected font-semibold text-brand-ink" : "text-muted hover:bg-hover hover:text-fg")}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="mt-2">
        {tab === "claude-code" && (
          <>
            <p className="text-xs text-muted">Rode no terminal. Os prompts prontos aparecem como comandos, ex.: <code>/mcp__chathm__resumir_atendimento</code>.</p>
            <Snippet text={claude} copyLabel="Copiar comando" />
          </>
        )}
        {tab === "claude-desktop" && (
          <>
            <p className="text-xs text-muted">Edite o arquivo de configuração do Claude Desktop (Settings → Developer → Edit Config). Precisa do Node.js instalado; usa a ponte <code>mcp-remote</code>. Reinicie o aplicativo depois.</p>
            <Snippet text={desktop} copyLabel="Copiar configuração" />
          </>
        )}
        {tab === "grok" && (
          <>
            <p className="text-xs text-muted">Coloque em <code>tools</code> da requisição à API da xAI (Responses API). Use <code>allowed_tools</code> para limitar o que o modelo enxerga.</p>
            <Snippet text={grok} copyLabel="Copiar configuração" />
          </>
        )}
        {tab === "chatgpt" && (
          <p className="rounded-lg bg-elevated p-3 text-xs text-muted">
            O ChatGPT cadastra servidores MCP como “conector personalizado” e costuma pedir login por OAuth, que o ChatHM ainda não oferece. Isso entra na etapa 5 do plano; por enquanto use Claude Code, Claude Desktop ou Grok.
          </p>
        )}
        {tab === "outros" && (
          <>
            <p className="text-xs text-muted">Qualquer cliente MCP que aceite Streamable HTTP com cabeçalho Bearer. Para testar a conexão pelo terminal:</p>
            <Snippet text={curl} copyLabel="Copiar teste" />
          </>
        )}
      </div>
      <div className="mt-4 rounded-lg bg-elevated p-3 text-xs text-muted">
        <p className="font-medium text-fg">Prompts prontos</p>
        <ul className="mt-1 list-disc pl-4">
          <li><code>resumir_atendimento</code>: resume o que o cliente pediu, a situação e os próximos passos.</li>
          <li><code>sugerir_resposta</code>: escreve uma sugestão para você revisar e enviar (nada é enviado pela IA).</li>
          <li><code>triagem_da_fila</code>: mostra o que precisa de atenção agora e a ordem sugerida.</li>
        </ul>
      </div>
    </div>
  );
}

function NewToken({ onClose, allowWrite }: { onClose: () => void; allowWrite: boolean }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [write, setWrite] = useState(false);
  const [error, setError] = useState("");
  const [nameError, setNameError] = useState("");
  const [created, setCreated] = useState<Created | null>(null);
  const create = useMutation({
    mutationFn: () => post<Created>("/ai/tokens", { name, expiresInDays: days, scopes: write ? ["read", "write"] : ["read"] }),
    onSuccess: (c) => {
      setCreated(c);
      void qc.invalidateQueries({ queryKey: ["ai", "tokens"] });
    },
    onError: (e) => {
      setNameError(fieldErrors(e).name ?? fieldErrors(e).expiresInDays ?? fieldErrors(e).scopes ?? "");
      setError(errorMessage(e));
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setNameError("");
    create.mutate();
  }

  return (
    <Modal open onClose={onClose} title={created ? "Token criado" : "Novo token de acesso"} className="max-w-xl">
      {created ? (
        <div className="flex flex-col gap-4">
          <Alert tone="info">Copie agora: por segurança, este token <strong>não será mostrado de novo</strong>.</Alert>
          <div>
            <code className="block break-all rounded-lg bg-elevated p-3 text-xs">{created.token}</code>
            <div className="mt-1.5"><CopyButton text={created.token} label="Copiar token" /></div>
          </div>
          <ConnectGuide token={created.token} />
          <div className="flex justify-end"><Button onClick={onClose}>Concluir</Button></div>
        </div>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Nome do token" placeholder="Ex.: Claude no meu notebook" value={name} maxLength={80} autoFocus error={nameError} onChange={(e) => setName(e.target.value)} />
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="font-medium">Validade</span>
            <select value={days} onChange={(e) => setDays(Number(e.target.value))} className="h-10 rounded-lg border border-line bg-elevated px-3 text-sm outline-none focus:border-accent">
              <option value={30}>30 dias</option>
              <option value={90}>90 dias (recomendado)</option>
              <option value={365}>1 ano</option>
            </select>
          </label>
          <fieldset className="flex flex-col gap-2 text-sm">
            <legend className="mb-1 font-medium">O que o token pode fazer</legend>
            <label className="flex cursor-pointer items-start gap-2.5">
              <input type="radio" name="scope" checked={!write} onChange={() => setWrite(false)} className="mt-1 accent-[var(--c-accent)]" />
              <span><span className="font-medium">Só ler (recomendado)</span><span className="block text-xs text-muted">Conversas, mensagens, contatos, filas, etiquetas e agenda que <strong>você</strong> já enxerga.</span></span>
            </label>
            <label className={cn("flex items-start gap-2.5", allowWrite ? "cursor-pointer" : "opacity-60")}>
              <input type="radio" name="scope" checked={write} disabled={!allowWrite} onChange={() => setWrite(true)} className="mt-1 accent-[var(--c-accent)]" />
              <span>
                <span className="font-medium">Ler e fazer alterações</span>
                <span className="block text-xs text-muted">
                  {allowWrite
                    ? "Também etiqueta, acrescenta observações, renomeia contatos, adia, cria lembretes e compromissos e transfere conversas, sempre dentro do que você pode fazer."
                    : "Um administrador precisa liberar alterações pela IA primeiro."}
                </span>
              </span>
            </label>
          </fieldset>
          <p className="text-xs text-muted">Em nenhum caso o token envia mensagens a clientes.</p>
          {error && !nameError && <Alert>{error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>Cancelar</Button>
            <Button type="submit" loading={create.isPending} disabled={!name.trim()}>Criar token</Button>
          </div>
        </form>
      )}
    </Modal>
  );
}

/** Acesso por IA (MCP): liga/desliga (administrador), tokens pessoais e instruções de conexão. */
export function AiAccessCard() {
  const { data: me } = useMe();
  const qc = useQueryClient();
  const admin = me?.user.role === "admin";
  const [showAll, setShowAll] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const settings = useQuery({ queryKey: settingsKey, queryFn: () => get<AiSettings>("/ai/settings") });
  const enabled = !!settings.data?.enabled;
  const tokens = useQuery({
    queryKey: tokensKey(showAll),
    enabled,
    queryFn: async () => (await get<{ tokens: AiToken[] }>(`/ai/tokens${showAll ? "?all=1" : ""}`)).tokens,
  });
  const save = useMutation({
    mutationFn: (next: AiSettings) => api<AiSettings>("PUT", "/ai/settings", next),
    onSuccess: (s) => qc.setQueryData(settingsKey, s),
    onError: (e) => setError(errorMessage(e)),
  });
  const revoke = useMutation({
    mutationFn: (id: string) => del<void>(`/ai/tokens/${id}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["ai", "tokens"] }),
    onError: (e) => setError(errorMessage(e)),
  });

  if (!settings.data) return null;
  const s = settings.data;
  const change = (patch: Partial<AiSettings>) => (setError(""), save.mutate({ ...s, ...patch }));

  return (
    <section className="border-t border-line py-6">
      <h2 className="font-semibold">Integrações (IA)</h2>
      <p className="mt-0.5 text-sm text-muted">
        Deixe um assistente de IA (Claude, Grok e outros que usam MCP) consultar o ChatHM em seu nome (por padrão só para leitura). Ele enxerga e faz apenas o que você já pode ver e fazer, e nunca envia mensagens.
      </p>

      <div className="mt-3">
        {admin ? (
          <>
            <Toggle checked={s.enabled} disabled={save.isPending} onChange={(v) => change({ enabled: v })} label="Permitir acesso por IA" hint="Desligado, nenhum token funciona. Os dados vão para o provedor de IA que a pessoa escolher: avise a equipe." />
            {s.enabled && (
              <div className="ml-7 border-l border-line pl-3">
                <Toggle checked={s.maskDocuments} disabled={save.isPending} onChange={(v) => change({ maskDocuments: v })} label="Ocultar CPF e CNPJ" hint="Recomendado. A IA vê “[CPF oculto]” no lugar." />
                <Toggle checked={s.maskPhones} disabled={save.isPending} onChange={(v) => change({ maskPhones: v })} label="Ocultar telefones" hint="Mostra só os 4 últimos dígitos." />
                <Toggle checked={s.allowWrite} disabled={save.isPending} onChange={(v) => change({ allowWrite: v })} label="Permitir que a IA faça alterações" hint="Etiquetar, acrescentar observações, renomear contatos, adiar, lembretes, agenda e transferir, sempre como a pessoa do token. Desligado, tokens de escrita deixam de alterar na hora. Nunca envia mensagens." />
              </div>
            )}
          </>
        ) : (
          <p className="text-sm">{s.enabled ? "O acesso por IA está ligado neste escritório." : "O acesso por IA está desligado. Peça a um administrador para ligar."}</p>
        )}
      </div>

      {error && <div className="mt-3"><Alert>{error}</Alert></div>}

      {enabled && (
        <div className="mt-5">
          <div className="flex items-center justify-between gap-3">
            <h3 className="flex items-center gap-2 text-sm font-medium"><KeyRound className="size-4" /> Tokens de acesso</h3>
            <Button type="button" variant="secondary" onClick={() => setCreating(true)}><Plus className="size-4" /> Novo token</Button>
          </div>
          {admin && (
            <label className="mt-2 flex cursor-pointer items-center gap-2 text-xs text-muted">
              <input type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} className="size-3.5 accent-[var(--c-accent)]" /> Ver tokens de toda a equipe
            </label>
          )}
          <details className="mt-3 rounded-lg border border-line px-3 py-2">
            <summary className="cursor-pointer text-sm font-medium">Como conectar o assistente</summary>
            <div className="mt-3"><ConnectGuide /></div>
          </details>
          <ul className="mt-2 divide-y divide-line">
            {tokens.data?.length === 0 && <li className="py-4 text-sm text-muted">Nenhum token ainda.</li>}
            {tokens.data?.map((t) => {
              const inactive = t.revoked || t.expired;
              return (
                <li key={t.id} className="flex items-center gap-3 py-2.5">
                  <div className={cn("min-w-0 flex-1", inactive && "opacity-60")}>
                    <p className="truncate text-sm font-medium">{t.name}{t.owner && <span className="font-normal text-muted"> · {t.owner}</span>}</p>
                    <p className="text-xs text-muted">
                      <code>{t.prefix}…</code> · {t.scopes.includes("write") ? "leitura e escrita" : "só leitura"} · {t.revoked ? "revogado" : t.expired ? "expirado" : `vale até ${date(t.expiresAt)}`} · {t.lastUsedAt ? `usado em ${date(t.lastUsedAt)}` : "nunca usado"}
                    </p>
                  </div>
                  {!inactive && (
                    <Button type="button" variant="ghost" disabled={revoke.isPending} aria-label={`Revogar ${t.name}`} onClick={() => revoke.mutate(t.id)}>
                      <Trash2 className="size-4" /> Revogar
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      )}
      {creating && <NewToken allowWrite={s.allowWrite} onClose={() => setCreating(false)} />}
    </section>
  );
}
