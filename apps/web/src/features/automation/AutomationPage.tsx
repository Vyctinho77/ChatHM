import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Pause, Pencil, Play, RotateCcw, Trash2, TriangleAlert, X } from "lucide-react";
import { Alert, Button, Field } from "@/components/ui";
import { ProviderLogo } from "@/components/ProviderLogo";
import { providerLabel } from "@/lib/providers";
import { Skeleton } from "@/components/Skeleton";
import { api, del, errorMessage, get, post } from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatBytes } from "@/lib/format";
import { useContacts } from "@/features/contacts/api";
import { DEMO } from "@/lib/env";

type Send = {
  id: string; createdAt: string; category: string; categoryLabel: string; reason: string; text: string; preview: string;
  contactId: string; contactName: string; documentName?: string; status: string; statusDetail?: string; flags: string[];
  provider: string; app: string; edited: boolean; scheduledFor?: string; sentAt?: string;
};
type Overview = { pending: number; queued: number; sentToday: number; sentLastHour: number; failed: number; hourlyCap: number; dailyCap: number; paused: boolean; pausedReason: string; whatsappReady: boolean; masterOn: boolean };
type Category = { key: string; label: string; enabled: boolean; level: number; windowStart: string; windowEnd: string; weekdays: number[]; minIntervalDays: number; guidelines: string };
type Settings = { paused: boolean; pausedReason: string; signature: string; hourlyCap: number; dailyCap: number; gapMinSeconds: number; gapMaxSeconds: number; dialogHours: number; clientDaily: number; timezone: string; allowedDomains: string[]; officeFields: Record<string, string> };
type Doc = { id: string; name: string; sizeBytes: number; ownerContactId?: string; ownerName?: string; shareable: boolean };

const STATUS: Record<string, string> = {
  pending_approval: "Aguardando aprovação", approved: "Na fila", sending: "Enviando", sent: "Enviado", failed: "Falhou",
  cancelled: "Cancelado", rejected: "Rejeitado", skipped: "Pulado",
};
const FLAG: Record<string, string> = { pressao: "Termo de pressão: revise com cuidado" };
const DAYS = ["", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb", "Dom"];
const TABS = [["aprovar", "Para aprovar"], ["fila", "Fila e histórico"], ["assuntos", "Assuntos"], ["controles", "Controles"], ["docs", "Documentos"]] as const;
type Tab = (typeof TABS)[number][0];

const useInvalidate = () => { const qc = useQueryClient(); return () => void qc.invalidateQueries({ queryKey: ["automation"] }); };

export function AutomationPage() {
  const [tab, setTab] = useState<Tab>("aprovar");
  const inv = useInvalidate();
  const ov = useQuery({ queryKey: ["automation", "overview"], enabled: !DEMO, queryFn: () => get<Overview>("/automation/overview"), refetchInterval: 5000 });
  const pause = useMutation({ mutationFn: (paused: boolean) => post<void>("/automation/pause", { paused, reason: paused ? "Pausado pelo administrador" : "" }), onSuccess: inv });
  const o = ov.data;
  return (
    <div className="h-full overflow-y-auto bg-app">
      <header className="flex flex-wrap items-start justify-between gap-4 border-b border-line px-4 py-4 sm:px-6">
        <div>
          <h1 className="text-xl font-semibold">Envios automáticos</h1>
          <p className="mt-1 text-sm text-muted">Avisos que a IA propõe aos clientes. Nada sai sem a sua aprovação e a IA nunca responde a ninguém.</p>
        </div>
        {o && (
          <Button variant={o.paused ? "primary" : "secondary"} loading={pause.isPending} onClick={() => pause.mutate(!o.paused)}>
            {o.paused ? <><Play className="size-4" />Retomar envios</> : <><Pause className="size-4" />Pausar tudo</>}
          </Button>
        )}
      </header>
      <div className="mx-auto max-w-4xl px-4 py-5 sm:px-8">
        {DEMO && <p className="text-sm text-muted">Os envios automáticos não estão disponíveis nesta demonstração.</p>}
        {o && !o.masterOn && <Alert tone="info">Desligado: ative “Permitir envios automáticos” em MCP → Permissões para a IA poder propor envios.</Alert>}
        {o?.paused && <div className="mt-3"><Alert>Envios pausados. {o.pausedReason}</Alert></div>}
        {o && !o.whatsappReady && <div className="mt-3"><Alert tone="info">O WhatsApp está desconectado: os envios aprovados esperam a conexão.</Alert></div>}
        {o && (
          <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            {([["Para aprovar", o.pending], ["Na fila", o.queued], [`Hoje (máx. ${o.dailyCap})`, o.sentToday], ["Falhas", o.failed]] as const).map(([k, v]) => (
              <div key={k} className="rounded-lg border border-line px-3 py-2.5"><dt className="text-xs text-muted">{k}</dt><dd className="mt-0.5 text-lg font-semibold">{v}</dd></div>
            ))}
          </dl>
        )}
        <div role="tablist" className="mt-5 flex gap-1 overflow-x-auto border-b border-line">
          {TABS.map(([k, label]) => (
            <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)} className={cn("whitespace-nowrap border-b-2 px-3 py-2 text-sm", tab === k ? "border-accent font-medium" : "border-transparent text-muted hover:text-fg")}>
              {label}{k === "aprovar" && o?.pending ? ` (${o.pending})` : ""}
            </button>
          ))}
        </div>
        <div className="py-4">
          {tab === "aprovar" && <SendList status="pending_approval" approval />}
          {tab === "fila" && <SendList status="" />}
          {tab === "assuntos" && <Categories />}
          {tab === "controles" && <Controls />}
          {tab === "docs" && <Documents />}
        </div>
      </div>
    </div>
  );
}

function SendList({ status, approval }: { status: string; approval?: boolean }) {
  const inv = useInvalidate();
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<{ id: string; text: string } | null>(null);
  const q = useQuery({ queryKey: ["automation", "sends", status], enabled: !DEMO, queryFn: async () => (await get<{ sends: Send[] }>(`/automation/sends?limit=100${status ? `&status=${status}` : ""}`)).sends, refetchInterval: 5000 });
  const act = useMutation({
    mutationFn: async (v: { kind: "approve" | "reject" | "cancel" | "retry"; id: string; text?: string }) => {
      if (v.kind === "retry") return post<void>(`/automation/sends/${v.id}/retry`);
      const body = v.kind === "approve" ? { ids: [v.id], edits: v.text ? { [v.id]: v.text } : {} } : { ids: [v.id] };
      const r = await post<{ refused?: Record<string, string> }>(`/automation/sends/${v.kind}`, body);
      const why = r?.refused?.[v.id];
      if (why) throw new Error(why);
    },
    onSuccess: () => { setError(""); setEditing(null); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  if (q.isPending) return <Skeleton className="h-24 w-full" />;
  const list = q.data ?? [];
  if (list.length === 0) return <p className="py-8 text-center text-sm text-muted">{approval ? "Nada para aprovar agora." : "Nenhum envio ainda."}</p>;
  return (
    <div className="flex flex-col gap-3">
      {error && <Alert>{error}</Alert>}
      {approval && list.length > 1 && (
        <div><Button variant="secondary" onClick={() => void Promise.all(list.filter((s) => s.flags.length === 0).map((s) => post("/automation/sends/approve", { ids: [s.id] }))).then(inv)}>Aprovar todos sem aviso</Button></div>
      )}
      {list.map((s) => (
        <article key={s.id} className="rounded-xl border border-line p-4">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
            <ProviderLogo provider={s.provider} /><span>{s.app || providerLabel(s.provider)}</span><span>·</span>
            <span className="font-medium text-fg">{s.contactName}</span><span>·</span><span>{s.categoryLabel}</span>
            <span className="ml-auto rounded-full bg-elevated px-2 py-0.5">{STATUS[s.status] ?? s.status}</span>
          </div>
          <p className="mt-2 text-xs text-muted">Motivo: {s.reason}</p>
          {editing?.id === s.id ? (
            <textarea value={editing.text} onChange={(e) => setEditing({ id: s.id, text: e.target.value })} rows={6} className="mt-2 w-full rounded-lg border border-line bg-elevated p-2.5 text-sm outline-none focus:border-accent" />
          ) : (
            <p className="mt-2 whitespace-pre-wrap rounded-lg bg-elevated p-3 text-sm">{s.preview || s.text}</p>
          )}
          {s.documentName && <p className="mt-2 text-xs text-muted">Anexo: {s.documentName}</p>}
          {s.flags.map((f) => <p key={f} className="mt-2 flex items-center gap-1.5 text-xs text-danger"><TriangleAlert className="size-3.5" />{FLAG[f] ?? f}</p>)}
          {s.statusDetail && <p className="mt-2 text-xs text-muted">{s.statusDetail}</p>}
          <div className="mt-3 flex flex-wrap gap-2">
            {s.status === "pending_approval" && (<>
              <Button loading={act.isPending} onClick={() => act.mutate({ kind: "approve", id: s.id, text: editing?.id === s.id ? editing.text : undefined })}><Check className="size-4" />{editing?.id === s.id ? "Editar e aprovar" : "Aprovar"}</Button>
              {editing?.id === s.id ? <Button variant="ghost" onClick={() => setEditing(null)}>Cancelar edição</Button> : <Button variant="secondary" onClick={() => setEditing({ id: s.id, text: s.text })}><Pencil className="size-4" />Editar</Button>}
              <Button variant="ghost" onClick={() => act.mutate({ kind: "reject", id: s.id })}><X className="size-4" />Rejeitar</Button>
            </>)}
            {s.status === "approved" && <Button variant="secondary" onClick={() => act.mutate({ kind: "cancel", id: s.id })}>Cancelar envio</Button>}
            {(s.status === "failed" || s.status === "skipped") && <Button variant="secondary" onClick={() => act.mutate({ kind: "retry", id: s.id })}><RotateCcw className="size-4" />Tentar de novo</Button>}
          </div>
        </article>
      ))}
    </div>
  );
}

function Categories() {
  const inv = useInvalidate();
  const q = useQuery({ queryKey: ["automation", "categories"], enabled: !DEMO, queryFn: async () => (await get<{ categories: Category[] }>("/automation/categories")).categories });
  const [error, setError] = useState("");
  const save = useMutation({ mutationFn: (c: Category) => api<Category>("PUT", `/automation/categories/${c.key}`, c), onSuccess: () => { setError(""); inv(); }, onError: (e) => setError(errorMessage(e)) });
  if (q.isPending) return <Skeleton className="h-24 w-full" />;
  return (
    <div className="flex flex-col gap-3">
      {error && <Alert>{error}</Alert>}
      <p className="text-xs text-muted">Todos os assuntos começam com aprovação sua. Desligado = a IA não consegue enfileirar.</p>
      {(q.data ?? []).map((c) => <CategoryRow key={c.key + JSON.stringify(c)} c={c} onSave={(n) => save.mutate(n)} busy={save.isPending} />)}
    </div>
  );
}

function CategoryRow({ c, onSave, busy }: { c: Category; onSave: (c: Category) => void; busy: boolean }) {
  const [d, setD] = useState(c);
  const dirty = JSON.stringify(d) !== JSON.stringify(c);
  return (
    <section className="rounded-xl border border-line p-4">
      <label className="flex cursor-pointer items-center justify-between gap-4">
        <span className="font-medium">{c.label}</span>
        <input type="checkbox" role="switch" aria-label={`Liberar ${c.label}`} checked={d.enabled} onChange={(e) => setD({ ...d, enabled: e.target.checked })} className="size-4 accent-[var(--c-accent)]" />
      </label>
      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Field label="Das" type="time" value={d.windowStart} onChange={(e) => setD({ ...d, windowStart: e.target.value })} />
        <Field label="Até" type="time" value={d.windowEnd} onChange={(e) => setD({ ...d, windowEnd: e.target.value })} />
        <Field label="Intervalo mínimo (dias)" type="number" min={0} value={d.minIntervalDays} onChange={(e) => setD({ ...d, minIntervalDays: Number(e.target.value) })} />
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {[1, 2, 3, 4, 5, 6, 7].map((n) => (
          <button key={n} type="button" aria-pressed={d.weekdays.includes(n)} onClick={() => setD({ ...d, weekdays: d.weekdays.includes(n) ? d.weekdays.filter((x) => x !== n) : [...d.weekdays, n].sort() })} className={cn("rounded-full border px-2.5 py-1 text-xs", d.weekdays.includes(n) ? "border-accent bg-accent text-accent-fg" : "border-line text-muted")}>{DAYS[n]}</button>
        ))}
      </div>
      <label className="mt-3 flex flex-col gap-1.5 text-sm">
        <span className="font-medium">Diretrizes (a IA lê antes de escrever)</span>
        <textarea value={d.guidelines} maxLength={2000} rows={3} onChange={(e) => setD({ ...d, guidelines: e.target.value })} className="rounded-lg border border-line bg-elevated p-2.5 text-sm outline-none focus:border-accent" />
      </label>
      {dirty && <div className="mt-3 flex justify-end gap-2"><Button variant="ghost" onClick={() => setD(c)}>Descartar</Button><Button loading={busy} onClick={() => onSave(d)}>Salvar</Button></div>}
    </section>
  );
}

function Controls() {
  const inv = useInvalidate();
  const q = useQuery({ queryKey: ["automation", "settings"], enabled: !DEMO, queryFn: () => get<Settings>("/automation/settings") });
  if (q.isPending || !q.data) return <Skeleton className="h-24 w-full" />;
  return <ControlsForm s={q.data} onSaved={inv} />;
}

function ControlsForm({ s, onSaved }: { s: Settings; onSaved: () => void }) {
  const [d, setD] = useState(s);
  const [domains, setDomains] = useState(s.allowedDomains.join(", "));
  const [fields, setFields] = useState(Object.entries(s.officeFields).map(([k, v]) => `${k}=${v}`).join("\n"));
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);
  const save = useMutation({
    mutationFn: () => api<Settings>("PUT", "/automation/settings", {
      ...d,
      allowedDomains: domains.split(/[,\s]+/).filter(Boolean),
      officeFields: Object.fromEntries(fields.split("\n").map((l) => l.trim()).filter((l) => l.includes("=")).map((l) => { const i = l.indexOf("="); return [l.slice(0, i).trim(), l.slice(i + 1).trim()]; })),
    }),
    onSuccess: () => { setError(""); setOk(true); onSaved(); },
    onError: (e) => { setOk(false); setError(errorMessage(e)); },
  });
  const num = (k: keyof Settings, label: string, min: number, max: number) => (
    <Field label={label} type="number" min={min} max={max} value={d[k] as number} onChange={(e) => setD({ ...d, [k]: Number(e.target.value) })} />
  );
  return (
    <form onSubmit={(e) => { e.preventDefault(); setOk(false); save.mutate(); }} className="flex flex-col gap-4">
      {error && <Alert>{error}</Alert>}
      <Field label="Assinatura para o cliente (nunca o nome de uma pessoa)" value={d.signature} maxLength={60} placeholder="Assistente do escritório" onChange={(e) => setD({ ...d, signature: e.target.value })} />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {num("hourlyCap", "Máx. por hora", 1, 500)}
        {num("dailyCap", "Máx. por dia", 1, 3000)}
        {num("clientDaily", "Máx. por cliente/dia", 1, 10)}
        {num("gapMinSeconds", "Intervalo mín. (s)", 1, 300)}
        {num("gapMaxSeconds", "Intervalo máx. (s)", 1, 600)}
        {num("dialogHours", "Sem conversa há (h)", 0, 168)}
      </div>
      <p className="text-xs text-muted">Se o cliente falou com a equipe nas últimas horas indicadas, a IA não envia nada: ela só inicia assuntos, nunca entra em conversa.</p>
      <Field label="Links permitidos (domínios, separados por vírgula)" value={domains} onChange={(e) => setDomains(e.target.value)} placeholder="meuescritorio.com.br" />
      <label className="flex flex-col gap-1.5 text-sm">
        <span className="font-medium">Campos fixos do escritório (um por linha, ex.: pix_escritorio=CNPJ 00.000.000/0001-00)</span>
        <textarea value={fields} onChange={(e) => setFields(e.target.value)} rows={3} className="rounded-lg border border-line bg-elevated p-2.5 font-mono text-xs outline-none focus:border-accent" />
        <span className="text-xs text-muted">A IA usa como {"{pix_escritorio}"}; ela nunca digita dados de pagamento.</span>
      </label>
      <div className="flex items-center justify-end gap-3">{ok && <span className="text-xs text-muted">Salvo.</span>}<Button type="submit" loading={save.isPending}>Salvar controles</Button></div>
    </form>
  );
}

function Documents() {
  const inv = useInvalidate();
  const q = useQuery({ queryKey: ["automation", "documents"], enabled: !DEMO, queryFn: async () => (await get<{ documents: Doc[] }>("/automation/documents")).documents });
  const { contacts } = useContacts();
  const [file, setFile] = useState<File | null>(null);
  const [name, setName] = useState("");
  const [owner, setOwner] = useState("");
  const [error, setError] = useState("");
  const up = useMutation({
    mutationFn: () => {
      const f = new FormData();
      f.set("name", name.trim() || file!.name);
      if (owner) f.set("ownerContactId", owner); else f.set("shareable", "true");
      f.set("file", file!);
      return post<Doc>("/automation/documents", f);
    },
    onSuccess: () => { setFile(null); setName(""); setOwner(""); setError(""); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  const rm = useMutation({ mutationFn: (id: string) => del<void>(`/automation/documents/${id}`), onSuccess: () => { setError(""); inv(); }, onError: (e) => setError(errorMessage(e)) });
  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted">PDFs que a IA pode anexar. Um PDF de cliente só vai para o próprio cliente; “para todos” pode ir a qualquer um.</p>
      {error && <Alert>{error}</Alert>}
      <form onSubmit={(e) => { e.preventDefault(); if (file) up.mutate(); }} className="grid gap-3 rounded-xl border border-line p-4 sm:grid-cols-2">
        <Field label="Nome (opcional)" value={name} maxLength={150} onChange={(e) => setName(e.target.value)} />
        <label className="flex flex-col gap-1.5 text-sm"><span className="font-medium">Cliente</span>
          <select value={owner} onChange={(e) => setOwner(e.target.value)} className="h-10 rounded-lg border border-line bg-elevated px-3 text-sm outline-none focus:border-accent">
            <option value="">Para todos (compartilhável)</option>
            {contacts.filter((c) => c.kind !== "group").map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </label>
        <input type="file" accept="application/pdf" aria-label="Arquivo PDF" onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="text-sm sm:col-span-2" />
        <div className="sm:col-span-2"><Button type="submit" loading={up.isPending} disabled={!file}>Enviar PDF</Button></div>
      </form>
      {(q.data ?? []).map((d) => (
        <div key={d.id} className="flex items-center justify-between gap-3 rounded-lg border border-line px-3 py-2.5 text-sm">
          <span className="min-w-0"><span className="block truncate font-medium">{d.name}</span><span className="text-xs text-muted">{d.shareable ? "Para todos" : d.ownerName} · {formatBytes(d.sizeBytes)}</span></span>
          <Button variant="ghost" aria-label={`Excluir ${d.name}`} onClick={() => rm.mutate(d.id)}><Trash2 className="size-4" /></Button>
        </div>
      ))}
    </div>
  );
}
