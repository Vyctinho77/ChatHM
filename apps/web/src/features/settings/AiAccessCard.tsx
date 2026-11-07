import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, KeyRound, Plus, Trash2 } from "lucide-react";
import { Alert, Button, Field, Modal } from "@/components/ui";
import { api, del, errorMessage, fieldErrors, get, post } from "@/lib/api";
import { cn } from "@/lib/cn";
import { useMe } from "@/features/auth/api";

type AiSettings = { enabled: boolean; maskDocuments: boolean; maskPhones: boolean };
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

/** Instruções de conexão por cliente de IA, com o token recém-criado. */
function ConnectGuide({ token }: { token: string }) {
  const url = `${window.location.origin}/mcp`;
  const claude = `claude mcp add --transport http chathm ${url} --header "Authorization: Bearer ${token}"`;
  const grok = JSON.stringify({ type: "mcp", server_label: "chathm", server_url: url, authorization: token }, null, 2);
  return (
    <div className="flex flex-col gap-4 text-sm">
      <div>
        <p className="font-medium">Claude Code</p>
        <pre className="mt-1 overflow-x-auto rounded-lg bg-elevated p-3 text-xs">{claude}</pre>
        <div className="mt-1.5"><CopyButton text={claude} label="Copiar comando" /></div>
      </div>
      <div>
        <p className="font-medium">Grok (API da xAI)</p>
        <p className="text-xs text-muted">Entre nas ferramentas da requisição (Responses API). Use <code>allowed_tools</code> para limitar o que o modelo enxerga.</p>
        <pre className="mt-1 overflow-x-auto rounded-lg bg-elevated p-3 text-xs">{grok}</pre>
        <div className="mt-1.5"><CopyButton text={grok} label="Copiar configuração" /></div>
      </div>
      <div>
        <p className="font-medium">Outros clientes MCP</p>
        <p className="text-xs text-muted">Endereço: <code>{url}</code> · cabeçalho <code>Authorization: Bearer …</code> (Streamable HTTP).</p>
      </div>
      <p className="rounded-lg bg-elevated p-3 text-xs text-muted">
        ChatGPT e claude.ai pedem login por OAuth ao cadastrar o servidor; essa parte vem na próxima etapa. Por enquanto use Claude Code, Grok ou outro cliente que aceite token.
      </p>
    </div>
  );
}

function NewToken({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [days, setDays] = useState(90);
  const [error, setError] = useState("");
  const [nameError, setNameError] = useState("");
  const [created, setCreated] = useState<Created | null>(null);
  const create = useMutation({
    mutationFn: () => post<Created>("/ai/tokens", { name, expiresInDays: days }),
    onSuccess: (c) => {
      setCreated(c);
      void qc.invalidateQueries({ queryKey: ["ai", "tokens"] });
    },
    onError: (e) => {
      setNameError(fieldErrors(e).name ?? fieldErrors(e).expiresInDays ?? "");
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
          <p className="text-xs text-muted">O token só lê dados: conversas, mensagens, contatos, filas, etiquetas e agenda que <strong>você</strong> já enxerga no ChatHM. Não envia mensagens.</p>
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
        Deixe um assistente de IA (Claude, Grok e outros que usam MCP) consultar o ChatHM em seu nome, só para leitura. Ele enxerga apenas o que você já vê.
      </p>

      <div className="mt-3">
        {admin ? (
          <>
            <Toggle checked={s.enabled} disabled={save.isPending} onChange={(v) => change({ enabled: v })} label="Permitir acesso por IA" hint="Desligado, nenhum token funciona. Os dados vão para o provedor de IA que a pessoa escolher: avise a equipe." />
            {s.enabled && (
              <div className="ml-7 border-l border-line pl-3">
                <Toggle checked={s.maskDocuments} disabled={save.isPending} onChange={(v) => change({ maskDocuments: v })} label="Ocultar CPF e CNPJ" hint="Recomendado. A IA vê “[CPF oculto]” no lugar." />
                <Toggle checked={s.maskPhones} disabled={save.isPending} onChange={(v) => change({ maskPhones: v })} label="Ocultar telefones" hint="Mostra só os 4 últimos dígitos." />
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
          <ul className="mt-2 divide-y divide-line">
            {tokens.data?.length === 0 && <li className="py-4 text-sm text-muted">Nenhum token ainda.</li>}
            {tokens.data?.map((t) => {
              const inactive = t.revoked || t.expired;
              return (
                <li key={t.id} className="flex items-center gap-3 py-2.5">
                  <div className={cn("min-w-0 flex-1", inactive && "opacity-60")}>
                    <p className="truncate text-sm font-medium">{t.name}{t.owner && <span className="font-normal text-muted"> · {t.owner}</span>}</p>
                    <p className="text-xs text-muted">
                      <code>{t.prefix}…</code> · {t.revoked ? "revogado" : t.expired ? "expirado" : `vale até ${date(t.expiresAt)}`} · {t.lastUsedAt ? `usado em ${date(t.lastUsedAt)}` : "nunca usado"}
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
      {creating && <NewToken onClose={() => setCreating(false)} />}
    </section>
  );
}
