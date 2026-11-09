import { useId, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, Copy, KeyRound, LogIn, Plus, Settings2, ShieldCheck, Trash2 } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { api, del, errorMessage, fieldErrors, get, post } from "@/lib/api";
import { cn } from "@/lib/cn";
import { Skeleton } from "@/components/Skeleton";
import { useMe } from "@/features/auth/api";
import { DomainInput } from "./DomainInput";

type AiSettings = { enabled: boolean; maskDocuments: boolean; maskPhones: boolean; allowWrite: boolean; allowSend: boolean; allowAutomation?: boolean; maskEmails: boolean; strictPrivacy: boolean; allowOauth: boolean; oauthRedirectHosts: string[] };
type Grant = { id: string; client: string; level: "read" | "write" | "send"; createdAt: string; lastUsedAt?: string; owner?: string };
type AiToken = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  provider?: string;
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
  const hintId = useId();
  return (
    <label className={cn("flex items-center justify-between gap-6 py-3", disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer")}>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{label}</span>
        {hint && <span id={hintId} className="mt-1 block text-xs leading-relaxed text-muted">{hint}</span>}
      </span>
      <span className="relative shrink-0">
        <input type="checkbox" role="switch" aria-label={label} aria-describedby={hint ? hintId : undefined} checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="peer sr-only" />
        <span aria-hidden="true" className="block h-6 w-10 rounded-full bg-muted/30 transition-colors peer-checked:bg-brand peer-focus-visible:ring-2 peer-focus-visible:ring-brand peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-panel peer-checked:[&>span]:translate-x-4 motion-reduce:transition-none">
          <span className="absolute left-0.5 top-0.5 size-5 rounded-full bg-white shadow-sm transition-transform motion-reduce:transition-none" />
        </span>
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
  ["claude-ai", "claude.ai"],
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
          <div className="text-xs text-muted">
            <p>O ChatGPT conecta por login (OAuth), sem token. Um administrador precisa ligar <strong>“Permitir conectar aplicativos por login”</strong> acima.</p>
            <ol className="mt-2 list-decimal pl-5">
              <li>No ChatGPT, abra Configurações → Conectores (modo desenvolvedor) e crie um conector personalizado.</li>
              <li>Em URL do servidor MCP, use <code>{url}</code> e escolha autenticação <strong>OAuth</strong>.</li>
              <li>Conecte: o ChatGPT abre o ChatHM, você entra (com o autenticador) e escolhe o que ele pode fazer.</li>
            </ol>
            <p className="mt-2">Os nomes e a disponibilidade desses menus variam conforme o plano e o administrador do seu espaço no ChatGPT; confira a ajuda da OpenAI.</p>
          </div>
        )}
        {tab === "claude-ai" && (
          <div className="text-xs text-muted">
            <p>O claude.ai também conecta por login (OAuth). Mesmo requisito: o administrador liga a conexão por login acima.</p>
            <ol className="mt-2 list-decimal pl-5">
              <li>No claude.ai, abra Configurações → Conectores e adicione um conector personalizado.</li>
              <li>Use o endereço <code>{url}</code> e conclua o login no ChatHM.</li>
            </ol>
            <p className="mt-2">A disponibilidade depende do seu plano; confira a ajuda da Anthropic.</p>
          </div>
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

function NewToken({ onClose, allowWrite, allowSend, allowAutomation }: { onClose: () => void; allowWrite: boolean; allowSend: boolean; allowAutomation: boolean }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [level, setLevel] = useState<"read" | "write" | "send" | "automate">("read");
  const [provider, setProvider] = useState("claude");
  const [error, setError] = useState("");
  const [nameError, setNameError] = useState("");
  const [created, setCreated] = useState<Created | null>(null);
  const create = useMutation({
    mutationFn: () => post<Created>("/ai/tokens", { name, expiresInDays: days, scopes: level === "automate" ? ["read", "write", "send", "automate"] : level === "send" ? ["read", "write", "send"] : level === "write" ? ["read", "write"] : ["read"], provider: level === "automate" ? provider : undefined }),
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
              <input type="radio" name="scope" checked={level === "read"} onChange={() => setLevel("read")} className="mt-1 accent-[var(--c-accent)]" />
              <span><span className="font-medium">Só ler (recomendado)</span><span className="block text-xs text-muted">Conversas, mensagens, contatos, filas, etiquetas e agenda que <strong>você</strong> já enxerga.</span></span>
            </label>
            <label className={cn("flex items-start gap-2.5", allowWrite ? "cursor-pointer" : "opacity-60")}>
              <input type="radio" name="scope" checked={level === "write"} disabled={!allowWrite} onChange={() => setLevel("write")} className="mt-1 accent-[var(--c-accent)]" />
              <span>
                <span className="font-medium">Ler, alterar e deixar rascunhos</span>
                <span className="block text-xs text-muted">
                  {allowWrite
                    ? "Também etiqueta, acrescenta observações, renomeia contatos, adia, cria lembretes e compromissos, transfere conversas e deixa rascunhos de resposta para você revisar."
                    : "Um administrador precisa liberar alterações pela IA primeiro."}
                </span>
              </span>
            </label>
            <label className={cn("flex items-start gap-2.5", allowSend ? "cursor-pointer" : "opacity-60")}>
              <input type="radio" name="scope" checked={level === "send"} disabled={!allowSend} onChange={() => setLevel("send")} className="mt-1 accent-[var(--c-accent)]" />
              <span>
                <span className="font-medium">Tudo isso e pedir envios (você aprova)</span>
                <span className="block text-xs text-muted">
                  {allowSend ? "A IA pode pedir o envio de uma mensagem ao cliente, mas ela só sai quando você clicar em Enviar na conversa." : "Um administrador precisa liberar pedidos de envio primeiro."}
                </span>
              </span>
            </label>
            <label className={cn("flex items-start gap-2.5", allowAutomation && allowSend ? "cursor-pointer" : "opacity-60")}>
              <input type="radio" name="scope" checked={level === "automate"} disabled={!allowAutomation || !allowSend} onChange={() => setLevel("automate")} className="mt-1 accent-[var(--c-accent)]" />
              <span>
                <span className="font-medium">Tudo isso e propor envios automáticos (o administrador aprova)</span>
                <span className="block text-xs text-muted">
                  {allowAutomation && allowSend ? "A IA decide sozinha quando avisar um cliente (DAS, cobrança, prazos), mas só inicia assuntos, nunca responde, e cada mensagem espera a aprovação do administrador." : "Um administrador precisa liberar os envios automáticos (e os pedidos de envio) primeiro."}
                </span>
              </span>
            </label>
            {level === "automate" && (
              <label className="ml-6 flex flex-col gap-1.5 text-sm">
                <span className="font-medium">Qual IA vai usar este token?</span>
                <select value={provider} onChange={(e) => setProvider(e.target.value)} className="h-10 rounded-lg border border-line bg-elevated px-3 text-sm outline-none focus:border-accent">
                  <option value="claude">Claude</option>
                  <option value="chatgpt">ChatGPT</option>
                  <option value="grok">Grok</option>
                  <option value="gemini">Gemini</option>
                  <option value="other">Outra</option>
                </select>
                <span className="text-xs text-muted">Aparece como logo nas mensagens enviadas, no lugar do nome de um atendente.</span>
              </label>
            )}
          </fieldset>
          <p className="text-xs text-muted">Sem a opção de envios automáticos, a IA nunca envia mensagens sozinha: rascunhos e pedidos só saem com a sua aprovação.</p>
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
    onError: (e) => { if (!fieldErrors(e).oauthRedirectHosts) setError(errorMessage(e)); },
  });
  const grants = useQuery({
    queryKey: ["ai", "grants", showAll],
    enabled: enabled && !!settings.data?.allowOauth,
    queryFn: async () => (await get<{ grants: Grant[] }>(`/ai/grants${showAll ? "?all=1" : ""}`)).grants,
  });
  const disconnect = useMutation({
    mutationFn: (id: string) => del<void>(`/ai/grants/${id}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["ai", "grants"] }),
    onError: (e) => setError(errorMessage(e)),
  });
  const revoke = useMutation({
    mutationFn: (id: string) => del<void>(`/ai/tokens/${id}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["ai", "tokens"] }),
    onError: (e) => setError(errorMessage(e)),
  });

  if (!settings.data) return (
    <section id="integracoes-ia" className="scroll-mt-6 border-t border-line py-6">
      <h2 className="font-semibold">Integrações de IA (MCP)</h2>
      {settings.isPending ? (
        <div role="status" aria-label="Carregando integrações MCP" aria-busy="true" className="mt-4 space-y-3">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-9 w-full" />
        </div>
      ) : (
        <div className="mt-3 space-y-3">
          <Alert>Não foi possível carregar as integrações. {errorMessage(settings.error)}</Alert>
          <Button variant="secondary" onClick={() => void settings.refetch()}>Tentar novamente</Button>
        </div>
      )}
    </section>
  );
  const s = settings.data;
  const change = (patch: Partial<AiSettings>) => (setError(""), save.mutate({ ...s, ...patch }));

  return (
    <section id="integracoes-ia" className="scroll-mt-6 divide-y divide-line/60">
      <div id="mcp-access" tabIndex={-1} className="scroll-mt-6 pb-5 outline-none">
        <h2 className="text-base font-semibold">Integrações de IA (MCP)</h2>
        {admin ? (
          <Toggle checked={s.enabled} disabled={save.isPending} onChange={(v) => change({ enabled: v })} label="Permitir acesso por IA" hint={s.enabled ? "Ativo. Cada assistente usa apenas os acessos de quem o conectou." : "Ative para conectar seus assistentes. Desligado, todos os acessos ficam suspensos."} />
        ) : (
          <p className="py-3 text-sm text-muted">{s.enabled ? "O acesso por IA está ativo neste escritório." : "Peça a um administrador para ativar o acesso por IA."}</p>
        )}
        <p className="flex items-start gap-2 pt-1 text-xs leading-relaxed text-muted"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-brand-ink" />Você revisa e aprova cada mensagem antes do envio ao cliente.</p>
      </div>

      {error && <div className="mt-3"><Alert>{error}</Alert></div>}

      {admin && enabled && <div className="divide-y divide-line/60">
        <details id="mcp-permissions" className="group/options scroll-mt-6">
          <summary className="flex cursor-pointer list-none items-center gap-3 py-5 [&::-webkit-details-marker]:hidden">
            <Settings2 className="size-4 shrink-0 text-muted" />
            <span className="flex-1"><span className="block text-sm font-medium">Permissões</span><span className="mt-0.5 block text-xs text-muted">{s.allowWrite ? "Alterações permitidas" : "Somente leitura"}{s.allowSend ? " · Pedidos de envio liberados" : ""}{s.allowAutomation ? " · Envios automáticos liberados" : ""}</span></span>
            <ChevronDown className="size-4 text-muted transition-transform group-open/options:rotate-180 motion-reduce:transition-none" />
          </summary>
          <div className="divide-y divide-line/60 border-t border-line/60 pb-4">
            <Toggle checked={s.allowWrite} disabled={save.isPending} onChange={v => change({ allowWrite: v })} label="Permitir alterações" hint="Etiquetas, observações, lembretes, agenda e rascunhos, conforme os acessos da pessoa." />
            <Toggle checked={s.allowSend} disabled={save.isPending || !s.allowWrite} onChange={v => change({ allowSend: v })} label="Permitir pedidos de envio" hint={s.allowWrite ? "O assistente sugere a mensagem e você decide se envia." : "Ative as alterações para liberar pedidos de envio."} />
            <Toggle checked={!!s.allowAutomation} disabled={save.isPending || !s.allowSend} onChange={v => change({ allowAutomation: v })} label="Permitir envios automáticos" hint={s.allowSend ? "A IA propõe avisos aos clientes (DAS, cobrança, prazos) e você aprova em Envios automáticos. Escolha os assuntos lá." : "Ative os pedidos de envio para liberar os envios automáticos."} />
          </div>
        </details>
        <details id="mcp-privacy" className="group/options scroll-mt-6">
          <summary className="flex cursor-pointer list-none items-center gap-3 py-5 [&::-webkit-details-marker]:hidden">
            <ShieldCheck className="size-4 shrink-0 text-muted" />
            <span className="flex-1"><span className="block text-sm font-medium">Privacidade</span><span className="mt-0.5 block text-xs text-muted">{s.strictPrivacy ? "Proteção adicional de nomes e observações" : "Escolha quais dados ocultar dos assistentes"}</span></span>
            <ChevronDown className="size-4 text-muted transition-transform group-open/options:rotate-180 motion-reduce:transition-none" />
          </summary>
          <div className="divide-y divide-line/60 border-t border-line/60">
            <Toggle checked={s.maskDocuments} disabled={save.isPending} onChange={v => change({ maskDocuments: v })} label="Ocultar CPF e CNPJ" hint="Documentos são substituídos por uma indicação de dado oculto." />
            <Toggle checked={s.maskPhones} disabled={save.isPending} onChange={v => change({ maskPhones: v })} label="Ocultar telefones" hint="Exibe apenas os quatro últimos dígitos." />
            <Toggle checked={s.maskEmails} disabled={save.isPending} onChange={v => change({ maskEmails: v })} label="Ocultar e-mails" />
            <Toggle checked={s.strictPrivacy} disabled={save.isPending} onChange={v => change({ strictPrivacy: v })} label="Proteção adicional" hint="Abrevia sobrenomes e oculta as observações internas dos contatos." />
          </div>
          <p className="border-t border-line/60 pb-5 pt-3 text-xs leading-relaxed text-muted">Senhas, chaves de acesso e dados financeiros ficam sempre ocultos. Aplicativos conectados por login também recebem telefones e observações protegidos.</p>
        </details>
        <details id="mcp-login" className="group/options scroll-mt-6">
          <summary className="flex cursor-pointer list-none items-center gap-3 py-5 [&::-webkit-details-marker]:hidden">
            <LogIn className="size-4 shrink-0 text-muted" />
            <span className="flex-1"><span className="block text-sm font-medium">Conexão por login</span><span className="mt-0.5 block text-xs text-muted">{s.allowOauth ? "Disponível para aplicativos autorizados" : "Conecte ChatGPT e outros assistentes por autorização"}</span></span>
            <ChevronDown className="size-4 text-muted transition-transform group-open/options:rotate-180 motion-reduce:transition-none" />
          </summary>
          <div className="border-t border-line/60 pb-5">
            <Toggle checked={s.allowOauth} disabled={save.isPending} onChange={v => change({ allowOauth: v })} label="Permitir conexão por login" hint="Você autoriza o aplicativo pelo ChatHM e pode desconectá-lo quando quiser." />
            {s.allowOauth && <details className="mt-1 text-xs">
              <summary className="cursor-pointer py-2 text-muted hover:text-fg">Sites autorizados</summary>
              <DomainInput domains={s.oauthRedirectHosts} disabled={save.isPending} onSave={hosts => {
                setError("");
                return save.mutateAsync({ ...s, oauthRedirectHosts: hosts });
              }} />
            </details>}
          </div>
        </details>
      </div>}

      {enabled && (
        <div id="mcp-tokens" tabIndex={-1} className="scroll-mt-6 py-6 outline-none">
          <div className="flex items-center justify-between gap-3">
            <h2 className="flex items-center gap-2 text-sm font-semibold"><KeyRound className="size-4 text-muted" /> Tokens de acesso</h2>
            <Button type="button" onClick={() => setCreating(true)}><Plus className="size-4" /> Novo token</Button>
          </div>
          <p className="mt-2 text-xs leading-relaxed text-muted">Crie um acesso para cada assistente. Você pode revogá-lo a qualquer momento.</p>
          {admin && <Toggle checked={showAll} onChange={setShowAll} label="Ver acessos de toda a equipe" />}
          <details id="mcp-connect-guide" className="mt-2 scroll-mt-6 border-t border-line pt-3">
            <summary className="cursor-pointer text-sm text-brand-ink">Como conectar o assistente</summary>
            <div className="mt-3"><ConnectGuide /></div>
          </details>
          <ul className="mt-2 divide-y divide-line">
            {tokens.isPending && <li className="py-4"><Skeleton className="h-10 w-full" /></li>}
            {tokens.isError && <li className="py-4"><Alert>{errorMessage(tokens.error)}</Alert></li>}
            {tokens.data?.length === 0 && <li className="py-5 text-center text-sm text-muted">Nenhum token criado.</li>}
            {tokens.data?.map((t) => {
              const inactive = t.revoked || t.expired;
              return (
                <li key={t.id} className="flex items-center gap-3 py-2.5">
                  <div className={cn("min-w-0 flex-1", inactive && "opacity-60")}>
                    <p className="truncate text-sm font-medium">{t.name}{t.owner && <span className="font-normal text-muted"> · {t.owner}</span>}</p>
                    <p className="text-xs text-muted">
                      <code>{t.prefix}…</code> · {t.scopes.includes("automate") ? "leitura, escrita, pedidos de envio e envios automáticos" : t.scopes.includes("send") ? "leitura, escrita e pedidos de envio" : t.scopes.includes("write") ? "leitura e escrita" : "só leitura"} · {t.revoked ? "revogado" : t.expired ? "expirado" : `vale até ${date(t.expiresAt)}`} · {t.lastUsedAt ? `usado em ${date(t.lastUsedAt)}` : "nunca usado"}
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
      {enabled && s.allowOauth && (
        <div className="py-6">
          <h2 className="text-sm font-semibold">Aplicativos conectados</h2>
          <p className="mt-2 text-xs leading-relaxed text-muted">Conexões autorizadas pelo login do ChatHM.</p>
          <ul className="mt-2 divide-y divide-line">
            {grants.isPending && <li className="py-4"><Skeleton className="h-10 w-full" /></li>}
            {grants.isError && <li className="py-4"><Alert>{errorMessage(grants.error)}</Alert></li>}
            {grants.data?.length === 0 && <li className="py-5 text-center text-sm text-muted">Nenhum aplicativo conectado.</li>}
            {grants.data?.map((g) => (
              <li key={g.id} className="flex items-center gap-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{g.client}{g.owner && <span className="font-normal text-muted"> · {g.owner}</span>}</p>
                  <p className="text-xs text-muted">
                    {g.level === "send" ? "leitura, escrita e pedidos de envio" : g.level === "write" ? "leitura e escrita" : "só leitura"} · conectado em {date(g.createdAt)} · {g.lastUsedAt ? `usado em ${date(g.lastUsedAt)}` : "nunca usado"}
                  </p>
                </div>
                <Button type="button" variant="ghost" disabled={disconnect.isPending} aria-label={`Desconectar ${g.client}`} onClick={() => disconnect.mutate(g.id)}>
                  <Trash2 className="size-4" /> Desconectar
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
      {creating && <NewToken allowWrite={s.allowWrite} allowSend={s.allowSend} allowAutomation={!!s.allowAutomation} onClose={() => setCreating(false)} />}
    </section>
  );
}
