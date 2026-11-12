import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, CheckCheck, ChevronDown, Clock3, FileText, Pause, Pencil, Play, Plus, RefreshCw, RotateCcw, Trash2, TriangleAlert, Upload, X } from "lucide-react";
import { Alert, Button, Field, IconButton } from "@/components/ui";
import { ProviderLogo } from "@/components/ProviderLogo";
import { FormattedText } from "@/components/FormattedText";
import { AiIcon } from "@/components/AiIcon";
import { InfoTip } from "@/components/InfoTip";
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
  provider: string; app: string; edited: boolean; scheduledFor?: string; sentAt?: string; chargeLabel?: string;
};
type Overview = { pending: number; queued: number; sentToday: number; sentLastHour: number; failed: number; hourlyCap: number; dailyCap: number; paused: boolean; pausedReason: string; whatsappReady: boolean; masterOn: boolean };
type Category = { key: string; label: string; enabled: boolean; level: number; windowStart: string; windowEnd: string; weekdays: number[]; minIntervalDays: number; guidelines: string };
type Settings = { paused: boolean; pausedReason: string; signature: string; hourlyCap: number; dailyCap: number; gapMinSeconds: number; gapMaxSeconds: number; dialogHours: number; clientDaily: number; timezone: string; allowedDomains: string[]; officeFields: Record<string, string> };
type Gateway = { provider: string; label: string; configured: boolean; enabled: boolean; sandbox: boolean; lastSyncAt?: string; lastError?: string };
type Charge = { id: string; provider: string; contactId?: string; contactName?: string; customerName: string; description: string; amount: string; dueDate: string; daysOverdue: number; status: string; hasLink: boolean };
type SyncResult = { seen: number; created: number; updated: number; closed: number; linked: number; unlinked: number };
type ImportResult = { created: number; updated: number; linked: number; unlinked: number; errors: string[] };
type Doc = { id: string; name: string; sizeBytes: number; ownerContactId?: string; ownerName?: string; shareable: boolean };

const STATUS: Record<string, string> = {
  pending_approval: "Aguardando aprovação", approved: "Na fila", sending: "Enviando", sent: "Enviado", failed: "Falhou",
  cancelled: "Cancelado", rejected: "Rejeitado", skipped: "Pulado",
};
const FLAG: Record<string, string> = { pressao: "Termo de pressão: revise com cuidado" };
const DAYS = ["", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb", "Dom"];
const TABS = [["aprovar", "Para aprovar"], ["fila", "Fila e histórico"], ["assuntos", "Assuntos"], ["cobrancas", "Cobranças"], ["controles", "Controles"], ["docs", "Documentos"]] as const;
type Tab = (typeof TABS)[number][0];

const useInvalidate = () => { const qc = useQueryClient(); return () => void qc.invalidateQueries({ queryKey: ["automation"] }); };
const inputClass = "h-10 min-w-0 rounded-xl border border-line/60 bg-app/50 px-3 text-[13px] text-fg outline-none transition-colors placeholder:text-muted/60 hover:border-line focus:border-brand focus:ring-2 focus:ring-brand/15";
const numberClass = "[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none";
const labelClass = "flex min-w-0 flex-col gap-2 text-xs font-medium text-muted";
const statusTone: Record<string, string> = {
  pending_approval: "bg-amber-500/10 text-amber-700 dark:text-amber-200",
  approved: "bg-sky-500/10 text-sky-700 dark:text-sky-200", sending: "bg-sky-500/10 text-sky-700 dark:text-sky-200",
  sent: "bg-brand/10 text-brand-ink", failed: "bg-danger/10 text-danger",
};

function EmptyState({ icon, title, description }: { icon: ReactNode; title: string; description: string }) {
  return <div className="flex flex-col items-center px-4 py-14 text-center">
    <span className="mb-4 flex size-11 items-center justify-center rounded-2xl bg-elevated text-muted [&>svg]:size-5">{icon}</span>
    <p className="text-sm font-medium">{title}</p><p className="mt-1.5 max-w-xs text-xs leading-relaxed text-muted">{description}</p>
  </div>;
}

function ShortDate({ value }: { value?: string }) {
  if (!value || Number.isNaN(Date.parse(value))) return null;
  return <time dateTime={value}>{new Date(value).toLocaleString("pt-BR", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" })}</time>;
}

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
          <div className="flex items-center gap-1.5">
            <h1 className="text-xl font-semibold">Envios automáticos</h1>
            <InfoTip title="Envios automáticos">Acompanhe os avisos da IA e defina como cada assunto pode enviar. Horários e regras são sempre aplicados pelo servidor.</InfoTip>
          </div>
        </div>
        {o && (
          <Button variant={o.paused ? "primary" : "ghost"} loading={pause.isPending} onClick={() => pause.mutate(!o.paused)}>
            {o.paused ? <><Play className="size-4" />Retomar envios</> : <><Pause className="size-4" />Pausar tudo</>}
          </Button>
        )}
      </header>
      <div className="mx-auto max-w-4xl px-4 py-5 sm:px-8">
        {pause.error && <div className="mb-3"><Alert>{errorMessage(pause.error)}</Alert></div>}
        {ov.isError && <div className="mb-3"><Alert>Não foi possível carregar o resumo. <button className="underline" onClick={() => void ov.refetch()}>Tentar novamente</button></Alert></div>}
        {DEMO && <p className="text-sm text-muted">Os envios automáticos não estão disponíveis nesta demonstração.</p>}
        {o && !o.masterOn && <Alert tone="info">Desligado: ative “Permitir envios automáticos” em MCP → Permissões para a IA poder enfileirar envios.</Alert>}
        {o?.paused && <div className="mt-3"><Alert>Envios pausados. {o.pausedReason}</Alert></div>}
        {o && !o.whatsappReady && <div className="mt-3"><Alert tone="info">O WhatsApp está desconectado: a fila espera a conexão.</Alert></div>}
        {ov.isPending && !DEMO && <div aria-label="Carregando resumo" className="grid grid-cols-2 gap-5 pb-5 sm:grid-cols-4">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-12 w-full" />)}</div>}
        {o && (
          <dl className="grid grid-cols-2 gap-x-8 gap-y-5 border-b border-line/60 pb-5 pt-2 text-sm sm:grid-cols-4">
            {([["Para aprovar", o.pending], ["Na fila", o.queued], ["Enviados hoje", o.sentToday], ["Falhas", o.failed]] as const).map(([k, v]) => (
              <div key={k}><dt className="text-xs text-muted">{k}</dt><dd className={cn("mt-1 text-2xl font-semibold tabular-nums", k === "Falhas" && v > 0 && "text-danger")}>{v}{k === "Enviados hoje" && <span className="ml-1.5 text-xs font-normal text-muted">/ {o.dailyCap}</span>}</dd></div>
            ))}
          </dl>
        )}
        <div role="tablist" aria-label="Envios automáticos" className="sticky top-0 z-10 mt-2 flex gap-2 overflow-x-auto border-b border-line bg-app [scrollbar-width:none] sm:gap-4">
          {TABS.map(([k, label]) => (
            <button key={k} id={`automation-tab-${k}`} role="tab" tabIndex={tab === k ? 0 : -1} aria-controls="automation-panel" aria-selected={tab === k} onClick={() => setTab(k)} onKeyDown={(e) => {
              const i = TABS.findIndex(([key]) => key === k);
              const next = e.key === "ArrowRight" ? (i + 1) % TABS.length : e.key === "ArrowLeft" ? (i + TABS.length - 1) % TABS.length : e.key === "Home" ? 0 : e.key === "End" ? TABS.length - 1 : -1;
              if (next < 0) return;
              e.preventDefault(); const key = TABS[next]![0]; setTab(key); document.getElementById(`automation-tab-${key}`)?.focus();
            }} className={cn("flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-1 py-3 text-[13px] transition-colors", tab === k ? "border-brand font-medium text-fg" : "border-transparent text-muted hover:text-fg")}>
              {label}{k === "aprovar" && !!o?.pending && <span className="rounded-full bg-brand/10 px-1.5 text-[10px] text-brand-ink">{o.pending}</span>}
            </button>
          ))}
        </div>
        <div id="automation-panel" role="tabpanel" aria-labelledby={`automation-tab-${tab}`} className="py-5">
          {tab === "aprovar" && <SendList status="pending_approval" approval />}
          {tab === "fila" && <SendList status="" />}
          {tab === "assuntos" && <Categories />}
          {tab === "cobrancas" && <Charges />}
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
      const body = v.kind === "approve" ? { ids: [v.id], edits: v.text !== undefined ? { [v.id]: v.text } : {} } : { ids: [v.id] };
      const r = await post<{ refused?: Record<string, string> }>(`/automation/sends/${v.kind}`, body);
      const why = r?.refused?.[v.id];
      if (why) throw new Error(why);
    },
    onSuccess: () => { setError(""); setEditing(null); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  const batch = useMutation({
    mutationFn: async () => {
      const ids = (q.data ?? []).filter((s) => s.flags.length === 0).map((s) => s.id);
      const result = await post<{ refused?: Record<string, string> }>("/automation/sends/approve", { ids });
      const refused = Object.values(result.refused ?? {});
      if (refused.length) throw new Error(`${refused.length} pedido(s) não foram aprovados. ${refused[0]}`);
    },
    onSuccess: () => setError(""), onError: (e) => setError(errorMessage(e)), onSettled: inv,
  });
  if (q.isPending) return <Skeleton className="h-24 w-full" />;
  if (q.isError) return <Alert>Não foi possível carregar os envios. <button className="underline" onClick={() => void q.refetch()}>Tentar novamente</button></Alert>;
  const list = q.data ?? [];
  if (list.length === 0) return <EmptyState icon={approval ? <CheckCheck /> : <Clock3 />} title={approval ? "Tudo em dia" : "Nenhum envio por aqui"} description={approval ? "Os pedidos que precisarem da sua revisão aparecerão aqui." : "Acompanhe aqui os próximos envios e o histórico dos avisos da IA."} />;
  return (
    <div>
      {error && <Alert>{error}</Alert>}
      {approval && list.length > 1 && list.some((s) => s.flags.length === 0) && (
        <div className="mb-1 flex flex-wrap items-center justify-between gap-3 border-b border-line/60 pb-4"><p className="text-xs text-muted">{list.length} pedidos para revisar</p><Button variant="ghost" loading={batch.isPending} disabled={act.isPending} className="text-brand-ink" onClick={() => batch.mutate()}><CheckCheck className="size-4" />Aprovar sem avisos</Button></div>
      )}
      {list.map((s) => (
        <article key={s.id} aria-label={`Envio para ${s.contactName}`} className="border-b border-line/60 py-5 first:pt-0 last:border-b-0">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="text-sm font-semibold">{s.contactName}</h3>
              <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted"><ProviderLogo provider={s.provider} className="size-3.5" /><span>{s.app || providerLabel(s.provider)}</span><span aria-hidden>·</span><span>{s.categoryLabel}</span><span aria-hidden>·</span><ShortDate value={s.sentAt ?? s.scheduledFor ?? s.createdAt} /></div>
            </div>
            <span className={cn("shrink-0 rounded-full px-2 py-1 text-[10px] font-medium", statusTone[s.status] ?? "bg-elevated text-muted")}>{STATUS[s.status] ?? s.status}</span>
          </div>
          {editing?.id === s.id ? (
            <textarea aria-label={`Mensagem para ${s.contactName}`} value={editing.text} onChange={(e) => setEditing({ id: s.id, text: e.target.value })} rows={5} className={cn(inputClass, "mt-4 h-auto w-full resize-y py-3 leading-relaxed")} />
          ) : (
            <p className="mt-4 whitespace-pre-wrap break-words border-l-2 border-brand/30 pl-3 text-[13px] leading-relaxed"><FormattedText text={s.preview || s.text} /></p>
          )}
          <p className="mt-3 text-[11px] leading-relaxed text-muted">Motivo: {s.reason}</p>
          {s.documentName && <p className="mt-2 flex items-center gap-1.5 text-xs text-muted"><FileText className="size-3.5 shrink-0" />{s.documentName}</p>}
          {s.chargeLabel && <p className="mt-2 text-xs text-muted">Cobrança: {s.chargeLabel}</p>}
          {s.flags.map((f) => <p key={f} className="mt-2 flex items-center gap-1.5 text-xs text-danger"><TriangleAlert className="size-3.5" />{FLAG[f] ?? f}</p>)}
          {s.statusDetail && <p className="mt-2 text-xs text-muted">{s.statusDetail}</p>}
          <div className="mt-4 flex flex-wrap gap-2">
            {s.status === "pending_approval" && (<>
              <Button loading={act.isPending} disabled={batch.isPending} onClick={() => act.mutate({ kind: "approve", id: s.id, text: editing?.id === s.id ? editing.text : undefined })}><Check className="size-4" />{editing?.id === s.id ? "Editar e aprovar" : "Aprovar"}</Button>
              {editing?.id === s.id ? <Button variant="ghost" onClick={() => setEditing(null)}>Cancelar edição</Button> : <Button variant="ghost" onClick={() => setEditing({ id: s.id, text: s.text })}><Pencil className="size-3.5" />Editar</Button>}
              <Button variant="ghost" disabled={act.isPending || batch.isPending} className="text-muted hover:text-danger" onClick={() => act.mutate({ kind: "reject", id: s.id })}><X className="size-3.5" />Rejeitar</Button>
            </>)}
            {s.status === "approved" && <Button variant="ghost" disabled={act.isPending} className="text-muted" onClick={() => act.mutate({ kind: "cancel", id: s.id })}>Cancelar envio</Button>}
            {(s.status === "failed" || s.status === "skipped") && <Button variant="ghost" disabled={act.isPending} className="text-brand-ink" onClick={() => act.mutate({ kind: "retry", id: s.id })}><RotateCcw className="size-3.5" />Tentar de novo</Button>}
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
  if (q.isError) return <Alert>Não foi possível carregar os assuntos. <button className="underline" onClick={() => void q.refetch()}>Tentar novamente</button></Alert>;
  return (
    <div>
      {error && <Alert>{error}</Alert>}
      <div className="mb-1 flex items-center gap-1.5 text-xs text-muted">Assuntos e autonomia<InfoTip title="Assuntos e autonomia">Libere os assuntos e escolha a autonomia de cada um. Horários e regras continuam sendo aplicados pelo servidor.</InfoTip></div>
      {(q.data ?? []).map((c) => <CategoryRow key={c.key + JSON.stringify(c)} c={c} onSave={(n) => save.mutate(n)} busy={save.isPending} />)}
    </div>
  );
}

function CategoryRow({ c, onSave, busy }: { c: Category; onSave: (c: Category) => void; busy: boolean }) {
  const [d, setD] = useState(c);
  const dirty = JSON.stringify(d) !== JSON.stringify(c);
  return (
    <section aria-label={c.label} className="border-b border-line/60 py-5 last:border-b-0">
      <div className="flex items-start gap-4">
      <details className="group min-w-0 flex-1">
        <summary className="flex cursor-pointer list-none items-start justify-between gap-3 rounded-sm [&::-webkit-details-marker]:hidden">
          <span><span className="block text-sm font-medium">{c.label}</span><span className="mt-1.5 block text-[11px] leading-relaxed text-muted">{d.enabled ? d.level === 3 ? "Automático" : "Com aprovação" : "Desativado"} · {d.windowStart} às {d.windowEnd}{dirty && <span className="ml-2 text-brand-ink">Não salvo</span>}</span></span>
          <ChevronDown className="mt-1 size-4 shrink-0 text-muted transition-transform group-open:rotate-180" />
        </summary>
        <div className="mt-5 flex flex-col gap-4">
      <label className={cn(labelClass, "max-w-sm")}>
        <span>Autonomia</span>
        <span className="relative">
        <select aria-label={`Autonomia de ${c.label}`} value={d.level} onChange={(e) => setD({ ...d, level: Number(e.target.value) })} className={cn(inputClass, "w-full appearance-none pr-9")}>
          <option value={1}>Com aprovação</option>
          <option value={3}>Automático, sem aprovação</option>
        </select>
        <ChevronDown aria-hidden className="pointer-events-none absolute right-3 top-1/2 size-3.5 -translate-y-1/2" />
        </span>
        <span className="text-[11px] font-normal leading-relaxed">{d.level === 3 ? "Segue sem aprovação quando as regras permitirem. Conteúdo sinalizado exige revisão." : "Os pedidos aguardam revisão antes de entrar na fila."}</span>
      </label>
      <div className="grid max-w-lg grid-cols-2 gap-3 sm:grid-cols-3">
        <label className={labelClass}>Das<input type="time" value={d.windowStart} onChange={(e) => setD({ ...d, windowStart: e.target.value })} className={cn(inputClass, "w-full [color-scheme:light] dark:[color-scheme:dark]")} /></label>
        <label className={labelClass}>Até<input type="time" value={d.windowEnd} onChange={(e) => setD({ ...d, windowEnd: e.target.value })} className={cn(inputClass, "w-full [color-scheme:light] dark:[color-scheme:dark]")} /></label>
        <label className={cn(labelClass, "col-span-2 max-w-44 sm:col-span-1")}>Intervalo mínimo (dias)<input type="number" min={0} max={365} value={d.minIntervalDays} onChange={(e) => setD({ ...d, minIntervalDays: Number(e.target.value) })} className={cn(inputClass, numberClass)} /></label>
      </div>
      <fieldset><legend className="mb-2 text-xs font-medium text-muted">Dias de envio</legend><div className="flex flex-wrap gap-1.5">
        {[1, 2, 3, 4, 5, 6, 7].map((n) => (
          <button key={n} type="button" aria-pressed={d.weekdays.includes(n)} onClick={() => setD({ ...d, weekdays: d.weekdays.includes(n) ? d.weekdays.filter((x) => x !== n) : [...d.weekdays, n].sort() })} className={cn("rounded-full border px-2.5 py-1.5 text-[11px] transition-colors", d.weekdays.includes(n) ? "border-brand/30 bg-brand/10 text-brand-ink" : "border-line/60 text-muted hover:bg-hover")}>{DAYS[n]}</button>
        ))}
      </div></fieldset>
      <label className={labelClass}>
        <span>Orientações para a IA</span>
        <textarea value={d.guidelines} maxLength={2000} rows={3} onChange={(e) => setD({ ...d, guidelines: e.target.value })} className={cn(inputClass, "h-auto resize-y py-3 font-normal leading-relaxed")} />
      </label>
        </div>
      </details>
      <label className="relative -mr-1 -mt-1 inline-flex h-9 w-11 shrink-0 cursor-pointer items-center justify-center">
        <input type="checkbox" role="switch" aria-label={`Liberar ${c.label}`} checked={d.enabled} disabled={busy} onChange={(e) => setD({ ...d, enabled: e.target.checked })} className="peer absolute inset-0 z-10 size-full cursor-pointer opacity-0 disabled:cursor-not-allowed" />
        <span aria-hidden className={cn("flex h-5 w-9 items-center rounded-full p-0.5 transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-brand peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-app peer-disabled:opacity-50", d.enabled ? "bg-brand" : "bg-hover")}><span className={cn("size-4 rounded-full bg-white shadow-sm transition-transform motion-reduce:transition-none", d.enabled && "translate-x-4")} /></span>
      </label>
      </div>
      {dirty && <div className="mt-3 flex justify-end gap-2"><Button variant="ghost" onClick={() => setD(c)}>Descartar</Button><Button loading={busy} onClick={() => onSave(d)}>Salvar</Button></div>}
    </section>
  );
}

function Controls() {
  const inv = useInvalidate();
  const q = useQuery({ queryKey: ["automation", "settings"], enabled: !DEMO, queryFn: () => get<Settings>("/automation/settings") });
  if (q.isPending) return <Skeleton className="h-24 w-full" />;
  if (q.isError || !q.data) return <Alert>Não foi possível carregar os controles. <button className="underline" onClick={() => void q.refetch()}>Tentar novamente</button></Alert>;
  return <ControlsForm s={q.data} onSaved={inv} />;
}

function ControlsForm({ s, onSaved }: { s: Settings; onSaved: () => void }) {
  const [d, setD] = useState(s);
  const [domains, setDomains] = useState(s.allowedDomains.join(", "));
  const [fields, setFields] = useState(() => Object.entries(s.officeFields).map(([key, value], id) => ({ id, key, value })));
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);
  const save = useMutation({
    mutationFn: () => {
      const entries = fields.filter((f) => f.key.trim() || f.value.trim()).map((f) => [f.key.trim(), f.value.trim()]);
      if (new Set(entries.map(([key]) => key)).size !== entries.length) throw new Error("Use um nome diferente para cada campo do escritório.");
      return api<Settings>("PUT", "/automation/settings", {
      ...d,
      allowedDomains: domains.split(/[,\s]+/).filter(Boolean),
      officeFields: Object.fromEntries(entries),
    });
    },
    onSuccess: () => { setError(""); setOk(true); onSaved(); },
    onError: (e) => { setOk(false); setError(errorMessage(e)); },
  });
  const num = (k: keyof Settings, label: string, min: number, max: number, unit: string) => (
    <label className="flex min-w-0 items-center justify-between gap-3 py-3">
      <span className="text-[13px]">{label}</span>
      <span className="flex shrink-0 items-center gap-2">
        <input aria-label={label} type="number" required min={min} max={max} value={d[k] as number} onChange={(e) => { setOk(false); setD({ ...d, [k]: Number(e.target.value) }); }} className={cn(inputClass, numberClass, "w-20 text-center tabular-nums")} />
        <span className="w-6 text-xs text-muted">{unit}</span>
      </span>
    </label>
  );
  return (
    <form onSubmit={(e) => { e.preventDefault(); setOk(false); save.mutate(); }} className="divide-y divide-line/60">
      {error && <Alert>{error}</Alert>}
      <section className="pb-5">
        <div className="mb-5 flex items-center gap-1.5">
          <h3 className="flex items-center gap-2 text-[13px] font-medium"><AiIcon className="size-4 text-muted" />Identidade da IA</h3>
          <InfoTip title="Identidade da IA">Escolha o nome em negrito que o cliente verá nas mensagens enviadas pela IA. Vale para novos avisos automáticos e respostas da IA pelo MCP; mensagens já enviadas mantêm o nome original.</InfoTip>
        </div>
        <div className="grid gap-5 sm:grid-cols-2 sm:gap-10">
          <div>
            <label className="flex max-w-sm flex-col gap-2">
              <span className="text-[13px] font-medium">Nome nas mensagens</span>
              <input value={d.signature} maxLength={60} placeholder="Assistente do escritório" onChange={(e) => { setOk(false); setD({ ...d, signature: e.target.value }); }} className={inputClass} />
            </label>
            <Button type="button" variant="ghost" className="mt-2 -ml-3 text-xs" disabled={!d.signature} onClick={() => { setOk(false); setD({ ...d, signature: "" }); }}>Usar nome padrão</Button>
          </div>
          <div>
            <p className="mb-2 text-xs font-medium text-muted">Como o cliente vai ver</p>
            <div className="rounded-xl bg-elevated/50 p-4">
              <div className="ml-auto w-fit max-w-full rounded-xl rounded-tr-sm bg-out px-3 py-2 text-[13px] leading-relaxed text-out-fg">
                <strong className="block break-words" data-testid="agent-name-preview">{d.signature.trim().replace(/\s+/g, " ") || "Assistente do escritório"}:</strong>
                <span>Olá! Como posso ajudar você hoje?</span>
              </div>
            </div>
          </div>
        </div>
      </section>
      <section className="grid gap-5 py-5 sm:grid-cols-2 sm:gap-10">
        <div>
          <h3 className="mb-1 text-xs font-medium text-muted">Limites de mensagens</h3>
          <div className="divide-y divide-line/40">
            {num("hourlyCap", "Por hora", 1, 500, "msg")}
            {num("dailyCap", "Por dia", 1, 3000, "msg")}
            {num("clientDaily", "Por cliente / dia", 1, 10, "msg")}
          </div>
        </div>
        <div>
          <h3 className="mb-1 text-xs font-medium text-muted">Intervalos</h3>
          <div className="divide-y divide-line/40">
            {num("gapMinSeconds", "Entre envios · mínimo", 1, 300, "s")}
            {num("gapMaxSeconds", "Entre envios · máximo", d.gapMinSeconds, 600, "s")}
            {num("dialogHours", "Após conversa humana", 0, 168, "h")}
          </div>
          <p className="mt-1 flex items-center gap-1 text-xs text-muted">Após conversa humana<InfoTip title="Após conversa humana">Avisos esperam esse período após a última conversa com a equipe.</InfoTip></p>
        </div>
      </section>
      <section className="py-5">
        <label className="flex max-w-lg flex-col gap-2">
          <span className="flex items-center gap-1.5 text-[13px] font-medium">Links permitidos<InfoTip title="Links permitidos">Domínios autorizados, separados por vírgula. Deixe vazio para bloquear links.</InfoTip></span>
          <input value={domains} onChange={(e) => { setOk(false); setDomains(e.target.value); }} placeholder="meuescritorio.com.br, gov.br" className={inputClass} />
        </label>
      </section>
      <details className="group py-5">
        <summary className="flex cursor-pointer list-none items-center justify-between gap-3 [&::-webkit-details-marker]:hidden">
          <span className="text-[13px] font-medium">Dados fixos do escritório<span className="ml-1 text-xs font-normal text-muted">{fields.length || "opcional"}</span></span>
          <ChevronDown className="size-4 text-muted transition-transform group-open:rotate-180" />
        </summary>
        <p className="mb-4 mt-2 text-xs text-muted">Ex.: {"{pix_escritorio}"}</p>
        <div className="flex flex-col gap-3">
          {fields.map((f, i) => <div key={f.id} className="flex items-start gap-2">
            <div className="grid min-w-0 flex-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]">
              <input aria-label={`Nome do campo ${i + 1}`} placeholder="pix_escritorio" value={f.key} onChange={(e) => { setOk(false); setFields(fields.map((x) => x.id === f.id ? { ...x, key: e.target.value } : x)); }} className={cn(inputClass, "font-mono text-xs")} />
              <input aria-label={`Valor do campo ${i + 1}`} placeholder="Valor do campo" value={f.value} maxLength={200} onChange={(e) => { setOk(false); setFields(fields.map((x) => x.id === f.id ? { ...x, value: e.target.value } : x)); }} className={inputClass} />
            </div>
            <IconButton label={`Remover campo ${i + 1}`} className="mt-1" onClick={() => { setOk(false); setFields(fields.filter((x) => x.id !== f.id)); }}><Trash2 className="size-4" /></IconButton>
          </div>)}
        </div>
        <Button type="button" variant="ghost" disabled={fields.length >= 12} className="mt-3 text-brand-ink" onClick={() => { setOk(false); setFields([...fields, { id: Math.max(-1, ...fields.map((f) => f.id)) + 1, key: "", value: "" }]); }}><Plus className="size-3.5" />Adicionar campo</Button>
      </details>
      <div className="flex items-center justify-end gap-3 pt-4">{ok && <span role="status" className="text-xs text-muted">Salvo.</span>}<Button type="submit" loading={save.isPending}>Salvar alterações</Button></div>
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
  const [adding, setAdding] = useState(false);
  const up = useMutation({
    mutationFn: () => {
      const f = new FormData();
      f.set("name", name.trim() || file!.name);
      if (owner) f.set("ownerContactId", owner); else f.set("shareable", "true");
      f.set("file", file!);
      return post<Doc>("/automation/documents", f);
    },
    onSuccess: () => { setFile(null); setName(""); setOwner(""); setAdding(false); setError(""); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  const rm = useMutation({ mutationFn: (id: string) => del<void>(`/automation/documents/${id}`), onSuccess: () => { setError(""); inv(); }, onError: (e) => setError(errorMessage(e)) });
  return (
    <div>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-1.5 text-xs text-muted">Biblioteca de PDFs<InfoTip title="Biblioteca de PDFs">PDFs disponíveis para os avisos. Documentos de um cliente são enviados apenas para ele.</InfoTip></div>
        {!adding && <Button onClick={() => setAdding(true)}><Plus className="size-3.5" />Adicionar PDF</Button>}
      </div>
      {error && <Alert>{error}</Alert>}
      {adding && <form onSubmit={(e) => { e.preventDefault(); if (file) up.mutate(); }} className="mb-5 flex flex-col gap-4 border-b border-line/60 pb-5">
        <label className="flex cursor-pointer items-center gap-3 rounded-xl border border-dashed border-line bg-app/50 p-4 transition-colors hover:border-brand/50 focus-within:ring-2 focus-within:ring-brand/30">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-elevated text-brand-ink">{file ? <FileText className="size-5" /> : <Upload className="size-5" />}</span>
          <span className="min-w-0"><span className="block truncate text-[13px] font-medium">{file?.name ?? "Selecionar PDF"}</span><span className="mt-1 block text-xs text-muted">{file ? `${formatBytes(file.size)} · Clique para trocar` : "Escolha um arquivo do seu computador"}</span></span>
          <input type="file" accept="application/pdf" aria-label="Arquivo PDF" onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="sr-only" />
        </label>
        <div className="grid gap-4 sm:grid-cols-2">
        <label className={labelClass}><span>Nome <span className="font-normal opacity-70">opcional</span></span><input value={name} maxLength={150} placeholder={file?.name ?? "Nome do documento"} onChange={(e) => setName(e.target.value)} className={inputClass} /></label>
        <label className={labelClass}><span>Disponível para</span>
          <span className="relative">
          <select value={owner} onChange={(e) => setOwner(e.target.value)} className={cn(inputClass, "w-full appearance-none pr-9")}>
            <option value="">Para todos (compartilhável)</option>
            {contacts.filter((c) => c.kind !== "group").map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
          <ChevronDown aria-hidden className="pointer-events-none absolute right-3 top-1/2 size-3.5 -translate-y-1/2" />
          </span>
        </label>
        </div>
        <div className="flex justify-end gap-2"><Button type="button" variant="ghost" disabled={up.isPending} onClick={() => { setAdding(false); setFile(null); setName(""); setOwner(""); }}>Cancelar</Button><Button type="submit" loading={up.isPending} disabled={!file}><Upload className="size-3.5" />Enviar PDF</Button></div>
      </form>}
      {q.isPending && <Skeleton className="h-20 w-full" />}
      {q.isError && <Alert>Não foi possível carregar os documentos. <button className="underline" onClick={() => void q.refetch()}>Tentar novamente</button></Alert>}
      {q.isSuccess && !q.data.length && !adding && <EmptyState icon={<FileText />} title="Sua biblioteca começa aqui" description="Adicione os PDFs que a IA poderá anexar aos avisos dos clientes." />}
      {(q.data ?? []).map((d) => (
        <div key={d.id} className="flex items-center gap-3 border-b border-line/60 py-4 last:border-b-0">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-elevated text-muted"><FileText className="size-5" /></span>
          <span className="min-w-0 flex-1"><span className="block truncate text-[13px] font-medium">{d.name}</span><span className="mt-1 block text-[11px] text-muted">{d.shareable ? "Para todos" : d.ownerName} · {formatBytes(d.sizeBytes)}</span></span>
          <IconButton label={`Excluir ${d.name}`} disabled={rm.isPending} className="hover:text-danger" onClick={() => rm.mutate(d.id)}><Trash2 className="size-4" /></IconButton>
        </div>
      ))}
    </div>
  );
}

const CHARGE_STATUS: Record<string, string> = { open: "Em aberto", overdue: "Vencida", paid: "Paga", cancelled: "Cancelada" };
const fmtDate = (iso: string) => iso.split("-").reverse().join("/");

function Charges() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-1.5 text-xs text-muted">Como a cobrança funciona<InfoTip title="Como a cobrança funciona">Para cobrar, a IA aponta uma cobrança daqui e o servidor coloca o link de pagamento, o valor e o vencimento reais na mensagem: ela nunca digita nem enxerga o link. Antes de enviar, o sistema confere de novo no gateway; quem já pagou não recebe a cobrança.</InfoTip></div>
      <GatewayCards />
      <ImportCharges />
      <ChargeList />
    </div>
  );
}

function GatewayCards() {
  const q = useQuery({ queryKey: ["automation", "gateways"], enabled: !DEMO, queryFn: async () => (await get<{ gateways: Gateway[] }>("/automation/gateways")).gateways });
  if (q.isPending) return <Skeleton className="h-24 w-full" />;
  return <>{(q.data ?? []).map((g) => <GatewayCard key={g.provider + String(g.enabled) + String(g.sandbox) + String(g.configured)} g={g} />)}</>;
}

function GatewayCard({ g }: { g: Gateway }) {
  const inv = useInvalidate();
  const [key, setKey] = useState("");
  const [sandbox, setSandbox] = useState(g.sandbox);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const save = useMutation({
    mutationFn: (enabled: boolean) => api<void>("PUT", `/automation/gateways/${g.provider}`, { apiKey: key.trim(), sandbox, enabled }),
    onSuccess: () => { setKey(""); setError(""); setInfo(""); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  const sync = useMutation({
    mutationFn: () => post<SyncResult>(`/automation/gateways/${g.provider}/sync`),
    onSuccess: (r) => { setError(""); setInfo(`${r.seen} cobranças em aberto (${r.created} novas, ${r.closed} encerradas). ${r.unlinked > 0 ? `${r.unlinked} ainda sem cliente: vincule abaixo.` : "Todas vinculadas a um cliente."}`); inv(); },
    onError: (e) => { setInfo(""); setError(errorMessage(e)); inv(); },
  });
  return (
    <section className="rounded-xl border border-line p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">{g.label}</h2>
        <span className={cn("rounded-full px-2 py-0.5 text-xs", g.enabled ? "bg-elevated" : "text-muted")}>{g.enabled ? "Conectado" : g.configured ? "Desligado" : "Não conectado"}</span>
      </div>
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
      {g.lastError && !error && <div className="mt-3"><Alert>{g.lastError}</Alert></div>}
      <form onSubmit={(e) => { e.preventDefault(); save.mutate(true); }} className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label={g.configured ? "Nova chave de API (deixe vazio para manter a atual)" : "Chave de API"} type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder={g.configured ? "•••••••• (salva e protegida)" : "$aact_..."} />
        <label className="flex items-center gap-2 self-end pb-2 text-sm"><input type="checkbox" checked={sandbox} onChange={(e) => setSandbox(e.target.checked)} className="size-4 accent-[var(--c-accent)]" />Ambiente de testes (sandbox)</label>
        <div className="flex flex-wrap gap-2 sm:col-span-2">
          <Button type="submit" loading={save.isPending} disabled={!g.configured && !key.trim()}>{g.enabled ? "Salvar" : "Conectar"}</Button>
          {g.enabled && <Button type="button" variant="secondary" loading={sync.isPending} onClick={() => sync.mutate()}><RefreshCw className="size-4" />Sincronizar agora</Button>}
          {g.enabled && <Button type="button" variant="ghost" onClick={() => save.mutate(false)}>Desligar</Button>}
        </div>
      </form>
      {info && <p className="mt-3 text-xs text-muted">{info}</p>}
      <p className="mt-3 text-xs text-muted">
        Só leitura: o ChatHM consulta as cobranças em aberto a cada 15 minutos e nunca cria, altera ou cancela nada em {g.label}. A chave fica cifrada no servidor e não volta para esta tela nem para a IA.
        {g.lastSyncAt ? ` Última sincronização: ${new Date(g.lastSyncAt).toLocaleString("pt-BR")}.` : ""}
      </p>
    </section>
  );
}

function ImportCharges() {
  const inv = useInvalidate();
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState("");
  const [res, setRes] = useState<ImportResult | null>(null);
  const up = useMutation({
    mutationFn: () => { const f = new FormData(); f.set("file", file!); return post<ImportResult>("/automation/charges/import", f); },
    onSuccess: (r) => { setError(""); setRes(r); setFile(null); inv(); },
    onError: (e) => { setRes(null); setError(errorMessage(e)); },
  });
  return (
    <section className="rounded-xl border border-line p-4">
      <h2 className="font-medium">Outro gateway ou planilha</h2>
      <p className="mt-1 text-xs text-muted">
        Importe um CSV com as colunas <code>codigo; telefone; cliente; descricao; valor; vencimento; situacao; link</code> (vírgula ou ponto e vírgula). O domínio de cada link precisa estar em
        <em> Controles → Links permitidos</em>. Importar de novo o mesmo código atualiza a cobrança. Dá para automatizar o envio da mesma lista por <code>POST /automation/charges/import</code>.
      </p>
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
      <form onSubmit={(e) => { e.preventDefault(); if (file) up.mutate(); }} className="mt-3 flex flex-wrap items-center gap-3">
        <input type="file" accept=".csv,text/csv" aria-label="Planilha CSV de cobranças" onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="text-sm" />
        <Button type="submit" loading={up.isPending} disabled={!file}>Importar</Button>
      </form>
      {res && (
        <div className="mt-3 text-xs text-muted">
          <p>{res.created} novas, {res.updated} atualizadas{res.unlinked > 0 ? `, ${res.unlinked} sem cliente` : ""}.</p>
          {res.errors.length > 0 && <ul className="mt-1 list-disc pl-4 text-danger">{res.errors.map((m) => <li key={m}>{m}</li>)}</ul>}
        </div>
      )}
    </section>
  );
}

function ChargeList() {
  const inv = useInvalidate();
  const [filter, setFilter] = useState<"open" | "unlinked" | "">("open");
  const { contacts } = useContacts();
  const [error, setError] = useState("");
  const q = useQuery({
    queryKey: ["automation", "charges", filter], enabled: !DEMO,
    queryFn: async () => (await get<{ charges: Charge[] }>(`/automation/charges${filter === "open" ? "?status=open" : filter === "unlinked" ? "?unlinked=1" : ""}`)).charges,
  });
  const link = useMutation({
    mutationFn: (v: { id: string; contactId: string }) => api<void>("PUT", `/automation/charges/${v.id}/contact`, { contactId: v.contactId }),
    onSuccess: () => { setError(""); inv(); },
    onError: (e) => setError(errorMessage(e)),
  });
  const list = q.data ?? [];
  return (
    <section>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-medium">Cobranças</h2>
        <div role="group" aria-label="Filtro" className="flex gap-1">
          {([["open", "Em aberto"], ["unlinked", "Sem cliente"], ["", "Todas"]] as const).map(([k, label]) => (
            <button key={k} type="button" aria-pressed={filter === k} onClick={() => setFilter(k)} className={cn("rounded-full border px-2.5 py-1 text-xs", filter === k ? "border-accent bg-accent text-accent-fg" : "border-line text-muted")}>{label}</button>
          ))}
        </div>
      </div>
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
      {q.isPending ? <Skeleton className="mt-3 h-24 w-full" /> : list.length === 0 ? <p className="py-6 text-center text-sm text-muted">Nenhuma cobrança por aqui.</p> : (
        <div className="mt-3 flex flex-col gap-2">
          {list.map((c) => (
            <div key={c.id} className="rounded-lg border border-line px-3 py-2.5 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="min-w-0"><span className="block truncate font-medium">{c.amount} · vence {fmtDate(c.dueDate)}{c.daysOverdue > 0 ? ` (${c.daysOverdue} dia${c.daysOverdue > 1 ? "s" : ""} de atraso)` : ""}</span>
                  <span className="text-xs text-muted">{c.description || "Sem descrição"} · {c.customerName || "Cliente sem nome"} · {c.provider === "manual" ? "Importada" : c.provider}</span></span>
                <span className="rounded-full bg-elevated px-2 py-0.5 text-xs">{CHARGE_STATUS[c.status] ?? c.status}</span>
              </div>
              {!c.hasLink && (c.status === "open" || c.status === "overdue") && <p className="mt-1 text-xs text-danger">Sem link de pagamento: a IA não consegue usar esta cobrança.</p>}
              <label className="mt-2 flex items-center gap-2 text-xs text-muted">
                <span>Cliente do ChatHM</span>
                <select aria-label={`Cliente da cobrança ${c.description || c.id}`} value={c.contactId ?? ""} onChange={(e) => link.mutate({ id: c.id, contactId: e.target.value })} className="h-8 min-w-0 flex-1 rounded-lg border border-line bg-elevated px-2 text-sm text-fg outline-none focus:border-accent">
                  <option value="">Sem vínculo (a IA não vê)</option>
                  {contacts.filter((x) => x.kind !== "group").map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                </select>
              </label>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
