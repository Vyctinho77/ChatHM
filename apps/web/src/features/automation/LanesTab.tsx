import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, FileText, MessageSquareWarning } from "lucide-react";
import { Alert, Button, Modal } from "@/components/ui";
import { InfoTip } from "@/components/InfoTip";
import { FormattedText } from "@/components/FormattedText";
import { Skeleton } from "@/components/Skeleton";
import { api, errorMessage, fieldErrors, get } from "@/lib/api";
import { cn } from "@/lib/cn";
import { DEMO } from "@/lib/env";
import { useSegments } from "@/features/queues/api";
import { STATE_HINT, STATE_LABEL, laneState, laneSummary, needsLiveConfirm, parseDomains, reasonLabel, validateLane, type Handoff, type Lane, type LaneDraft } from "./lanes";

const inputClass = "h-10 min-w-0 rounded-xl border border-line/60 bg-app/50 px-3 text-[13px] text-fg outline-none transition-colors placeholder:text-muted/60 hover:border-line focus:border-brand focus:ring-2 focus:ring-brand/15";
const numberClass = "[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none";
const stateTone = { off: "bg-elevated text-muted", shadow: "bg-amber-500/10 text-amber-700 dark:text-amber-200", live: "bg-brand/10 text-brand-ink" } as const;

function When({ value }: { value: string }) {
  if (Number.isNaN(Date.parse(value))) return null;
  return <time dateTime={value}>{new Date(value).toLocaleString("pt-BR", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" })}</time>;
}

function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <label className="relative -mr-1 inline-flex h-9 w-11 shrink-0 cursor-pointer items-center justify-center">
      <input type="checkbox" role="switch" aria-label={label} checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="peer absolute inset-0 z-10 size-full cursor-pointer opacity-0 disabled:cursor-not-allowed" />
      <span aria-hidden className={cn("flex h-5 w-9 items-center rounded-full p-0.5 transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-brand peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-app peer-disabled:opacity-50", checked ? "bg-brand" : "bg-hover")}><span className={cn("size-4 rounded-full bg-white shadow-sm transition-transform motion-reduce:transition-none", checked && "translate-x-4")} /></span>
    </label>
  );
}

function Row({ title, hint, children }: { title: string; hint?: ReactNode; children: ReactNode }) {
  return <div className="flex items-start justify-between gap-4 py-3"><div className="min-w-0"><p className="text-[13px]">{title}</p>{hint && <p className="mt-1 text-[11px] leading-relaxed text-muted">{hint}</p>}</div>{children}</div>;
}

export function LanesTab() {
  const lanes = useQuery({ queryKey: ["automation", "lanes"], enabled: !DEMO, queryFn: async () => (await get<{ lanes: Lane[] }>("/automation/lanes")).lanes });
  const handoffs = useQuery({ queryKey: ["automation", "handoffs"], enabled: !DEMO, queryFn: async () => (await get<{ handoffs: Handoff[] }>("/automation/handoffs?open=1&limit=50")).handoffs, refetchInterval: 15000 });
  const drafts = useQuery({ queryKey: ["automation", "lane-drafts"], enabled: !DEMO, queryFn: async () => (await get<{ drafts: LaneDraft[] }>("/automation/lane-drafts?limit=50")).drafts, refetchInterval: 15000 });
  const segments = useSegments();
  if (DEMO) return <p className="text-sm text-muted">As raias da IA não estão disponíveis nesta demonstração.</p>;
  if (lanes.isPending) return <Skeleton className="h-24 w-full" />;
  if (lanes.isError) return <Alert>Não foi possível carregar as raias. <button className="underline" onClick={() => void lanes.refetch()}>Tentar novamente</button></Alert>;
  const segName = (id: string) => segments.data?.find((s) => s.id === id)?.name;
  const laneLabel = (key: string) => lanes.data?.find((l) => l.key === key)?.label ?? key;
  const open = handoffs.data ?? [];
  const list = drafts.data ?? [];
  return (
    <div className="flex flex-col gap-8">
      <section>
        <div className="mb-1 flex items-center gap-1.5 text-xs text-muted">Raias da IA<InfoTip title="Raias da IA">Uma raia é o espaço em que a IA atende com liberdade, cercada por poucas regras que o servidor aplica a cada envio. Ela só vale para um token do atendente autônomo ligado a ela (MCP → Tokens de acesso). Toda raia nasce desligada e em sombra: a IA escreve, mas nada vai ao cliente.</InfoTip></div>
        {lanes.data.map((l) => <LaneCard key={l.key + l.updatedAt} lane={l} segmentName={segName(l.segmentId)} segments={(segments.data ?? []).filter((s) => s.isActive).map((s) => ({ id: s.id, name: s.name }))} />)}
      </section>

      <section aria-label="Pediram ajuda de uma pessoa">
        <h3 className="mb-1 flex items-center gap-1.5 text-[13px] font-medium"><MessageSquareWarning className="size-4 text-muted" />Pediram ajuda de uma pessoa{open.length > 0 && <span className="rounded-full bg-amber-500/10 px-1.5 text-[10px] text-amber-700 dark:text-amber-200">{open.length}</span>}</h3>
        <p className="mb-3 text-xs text-muted">Conversas que a IA passou para a equipe. Ela fica em silêncio nelas até alguém responder.</p>
        {handoffs.isPending && <Skeleton className="h-12 w-full" />}
        {handoffs.isError && <Alert>Não foi possível carregar os pedidos de ajuda.</Alert>}
        {handoffs.isSuccess && open.length === 0 && <p className="border-t border-line/60 py-4 text-xs text-muted">Nenhum pedido de ajuda em aberto.</p>}
        {open.map((h) => (
          <article key={h.id} className="border-t border-line/60 py-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-[13px] font-medium">{reasonLabel(h.reason)}</p>
              <p className="text-[11px] text-muted">{laneLabel(h.lane)} · {h.source === "server" ? "regra do servidor" : "pedido da IA"} · <When value={h.createdAt} /></p>
            </div>
            {h.summary && <p className="mt-2 whitespace-pre-wrap break-words text-[13px] leading-relaxed text-muted">{h.summary}</p>}
            <Link to={`/conversas/${h.conversationId}`} className="mt-2 inline-block text-xs font-medium text-brand-ink hover:underline">Abrir a conversa</Link>
          </article>
        ))}
      </section>

      <section aria-label="Rascunhos em sombra">
        <h3 className="mb-1 flex items-center gap-1.5 text-[13px] font-medium"><FileText className="size-4 text-muted" />Rascunhos em sombra</h3>
        <p className="mb-3 text-xs text-muted">O que a IA enviaria se a raia estivesse valendo. Nada disso foi para o cliente.</p>
        {drafts.isPending && <Skeleton className="h-12 w-full" />}
        {drafts.isError && <Alert>Não foi possível carregar os rascunhos.</Alert>}
        {drafts.isSuccess && list.length === 0 && <p className="border-t border-line/60 py-4 text-xs text-muted">Ainda não há rascunhos.</p>}
        {list.map((d) => (
          <article key={d.id} className="border-t border-line/60 py-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-[13px] font-medium">{d.contactName}</p>
              <p className="text-[11px] text-muted">{laneLabel(d.lane)} · <When value={d.createdAt} /></p>
            </div>
            <p className="mt-3 whitespace-pre-wrap break-words border-l-2 border-brand/30 pl-3 text-[13px] leading-relaxed"><FormattedText text={d.body} /></p>
            {d.conversationId && <Link to={`/conversas/${d.conversationId}`} className="mt-2 inline-block text-xs font-medium text-brand-ink hover:underline">Abrir a conversa</Link>}
          </article>
        ))}
      </section>
    </div>
  );
}

type SegOpt = { id: string; name: string };

function LaneCard({ lane, segmentName, segments }: { lane: Lane; segmentName?: string; segments: SegOpt[] }) {
  const qc = useQueryClient();
  const [d, setD] = useState<Lane>(lane);
  const [domains, setDomains] = useState(lane.allowedDomains.join(", "));
  const [error, setError] = useState("");
  const [confirm, setConfirm] = useState(false);
  const edited: Lane = { ...d, allowedDomains: parseDomains(domains) };
  const dirty = JSON.stringify(edited) !== JSON.stringify(lane);
  const errors = validateLane(edited);
  const invalid = Object.keys(errors).length > 0;
  const state = laneState(edited);
  const save = useMutation({
    mutationFn: () => api<Lane>("PUT", `/automation/lanes/${lane.key}`, {
      label: edited.label.trim(), enabled: edited.enabled, mode: edited.mode, canInitiate: edited.canInitiate, segmentId: edited.segmentId, handoffSegmentId: edited.handoffSegmentId,
      maxTurns: edited.maxTurns, maxConsecutive: edited.maxConsecutive, contactDaily: edited.contactDaily, humanQuietMinutes: edited.humanQuietMinutes, staleHours: edited.staleHours,
      requireSources: edited.requireSources, allowedDomains: edited.allowedDomains, guidelines: edited.guidelines,
    }),
    onSuccess: () => { setError(""); setConfirm(false); void qc.invalidateQueries({ queryKey: ["automation", "lanes"] }); },
    onError: (e) => { setConfirm(false); const f = fieldErrors(e); setError(Object.values(f)[0] ?? errorMessage(e)); },
  });
  const submit = () => { if (invalid) return; if (needsLiveConfirm(lane, edited)) setConfirm(true); else save.mutate(); };
  const set = (patch: Partial<Lane>) => { setError(""); setD({ ...d, ...patch }); };
  const num = (k: "maxTurns" | "maxConsecutive" | "contactDaily" | "humanQuietMinutes" | "staleHours", label: string, unit: string, hint?: string) => (
    <div className="py-3">
      <label className="flex min-w-0 items-center justify-between gap-3">
        <span className="text-[13px]">{label}</span>
        <span className="flex shrink-0 items-center gap-2">
          <input aria-label={label} type="number" value={Number.isFinite(d[k]) ? d[k] : ""} onChange={(e) => set({ [k]: e.target.value === "" ? NaN : Number(e.target.value) })} aria-invalid={!!errors[k]} className={cn(inputClass, numberClass, "w-20 text-center tabular-nums", errors[k] && "border-danger")} />
          <span className="w-8 text-xs text-muted">{unit}</span>
        </span>
      </label>
      {(errors[k] || hint) && <p className={cn("mt-1 text-[11px] leading-relaxed", errors[k] ? "text-danger" : "text-muted")}>{errors[k] ?? hint}</p>}
    </div>
  );
  const select = (label: string, value: string, empty: string, onChange: (v: string) => void) => (
    <label className="flex min-w-0 flex-col gap-2 text-xs font-medium text-muted">
      <span>{label}</span>
      <span className="relative">
        <select aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} className={cn(inputClass, "w-full appearance-none pr-9")}>
          <option value="">{empty}</option>
          {segments.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          {value && !segments.some((s) => s.id === value) && <option value={value}>Fila que não existe mais</option>}
        </select>
        <ChevronDown aria-hidden className="pointer-events-none absolute right-3 top-1/2 size-3.5 -translate-y-1/2" />
      </span>
    </label>
  );
  return (
    <section aria-label={lane.label} className="border-b border-line/60 py-5 last:border-b-0">
      <div className="flex items-start gap-4">
        <details className="group min-w-0 flex-1">
          <summary className="flex cursor-pointer list-none items-start justify-between gap-3 rounded-sm [&::-webkit-details-marker]:hidden">
            <span className="min-w-0">
              <span className="flex flex-wrap items-center gap-2"><span className="text-sm font-medium">{lane.label}</span><span className={cn("rounded-full px-2 py-0.5 text-[10px] font-medium", stateTone[state])}>{STATE_LABEL[state]}</span>{dirty && <span className="text-[11px] text-brand-ink">Não salvo</span>}</span>
              <span className="mt-1.5 block text-[11px] leading-relaxed text-muted">{laneSummary(lane, segmentName)}</span>
            </span>
            <ChevronDown className="mt-1 size-4 shrink-0 text-muted transition-transform group-open:rotate-180" />
          </summary>
          <div className="mt-4 divide-y divide-line/40">
            {error && <div className="pb-3"><Alert>{error}</Alert></div>}
            <div className="pb-4">
              <label className="flex max-w-sm flex-col gap-2 text-xs font-medium text-muted">Nome da raia
                <input value={d.label} maxLength={60} onChange={(e) => set({ label: e.target.value })} aria-invalid={!!errors.label} className={cn(inputClass, errors.label && "border-danger")} />
                {errors.label && <span className="text-[11px] font-normal text-danger">{errors.label}</span>}
              </label>
            </div>
            <fieldset className="py-4">
              <legend className="mb-2 text-xs font-medium text-muted">Modo</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {(["shadow", "live"] as const).map((m) => (
                  <button key={m} type="button" aria-pressed={d.mode === m} onClick={() => set({ mode: m })} className={cn("rounded-xl border px-3 py-2.5 text-left transition-colors", d.mode === m ? "border-brand/40 bg-brand/10" : "border-line/60 hover:bg-hover")}>
                    <span className="block text-[13px] font-medium">{m === "shadow" ? "Sombra" : "Valendo"}</span>
                    <span className="mt-1 block text-[11px] leading-relaxed text-muted">{m === "shadow" ? "A IA escreve e o ChatHM guarda o rascunho. Nada vai ao cliente." : "A IA envia de verdade aos clientes."}</span>
                  </button>
                ))}
              </div>
              <p className="mt-2 text-[11px] text-muted">{STATE_HINT[state]}</p>
            </fieldset>
            <Row title="Pode iniciar conversas" hint="Desligado: só responde a clientes que já escreveram. Ligado: também pode escrever primeiro (por exemplo, uma cobrança)."><Switch checked={d.canInitiate} onChange={(v) => set({ canInitiate: v })} label={`${lane.label}: pode iniciar conversas`} /></Row>
            <div className="grid gap-4 py-4 sm:grid-cols-2">
              {select("Fila da raia", d.segmentId, "Qualquer fila", (v) => set({ segmentId: v }))}
              {select("Quando pedir uma pessoa, mover para", d.handoffSegmentId, "Manter onde está", (v) => set({ handoffSegmentId: v }))}
            </div>
            <div className="grid gap-x-10 sm:grid-cols-2">
              <div className="divide-y divide-line/40">
                {num("maxTurns", "Rodadas por conversa", "msg", "Depois disso a conversa passa para uma pessoa.")}
                {num("maxConsecutive", "Seguidas sem resposta", "msg", "A IA espera o cliente responder.")}
                {num("contactDaily", "Por cliente / dia", "msg")}
              </div>
              <div className="divide-y divide-line/40">
                {num("humanQuietMinutes", "Silêncio após uma pessoa", "min", "Depois que alguém da equipe responde, a IA espera este tempo.")}
                {num("staleHours", "Conversas paradas após", "h", "A IA procura conversas sem nenhum atendimento há este tempo. 0 desliga.")}
              </div>
            </div>
            <Row title="Exigir fontes em regras, leis e prazos" hint="Respostas sobre regra do governo, lei, norma ou prazo só saem com as páginas consultadas, mostradas ao cliente no fim da mensagem."><Switch checked={d.requireSources} onChange={(v) => set({ requireSources: v })} label={`${lane.label}: exigir fontes`} /></Row>
            <div className="py-4">
              <label className="flex max-w-lg flex-col gap-2 text-xs font-medium text-muted">Links permitidos
                <input value={domains} onChange={(e) => { setError(""); setDomains(e.target.value); }} placeholder="Vazio = qualquer link. Ex.: asaas.com" aria-invalid={!!errors.allowedDomains} className={cn(inputClass, errors.allowedDomains && "border-danger")} />
                {errors.allowedDomains ? <span className="text-[11px] font-normal text-danger">{errors.allowedDomains}</span> : <span className="text-[11px] font-normal leading-relaxed">Só estes domínios podem aparecer no texto da IA. Deixe vazio para liberar qualquer link.</span>}
              </label>
            </div>
            <div className="pt-4">
              <label className="flex flex-col gap-2 text-xs font-medium text-muted">Orientações para a IA
                <textarea value={d.guidelines} maxLength={6000} rows={5} onChange={(e) => set({ guidelines: e.target.value })} className={cn(inputClass, "h-auto resize-y py-3 font-normal leading-relaxed")} />
                <span className="text-[11px] font-normal">A IA lê isto em “my_access”. Diga o tom, o que resolver sozinha e quando passar para uma pessoa.</span>
              </label>
            </div>
          </div>
        </details>
        <Switch checked={d.enabled} onChange={(v) => set({ enabled: v })} label={`Ligar ${lane.label}`} />
      </div>
      {dirty && <div className="mt-3 flex justify-end gap-2"><Button variant="ghost" onClick={() => { setD(lane); setDomains(lane.allowedDomains.join(", ")); setError(""); }}>Descartar</Button><Button loading={save.isPending} disabled={invalid} onClick={submit}>Salvar</Button></div>}
      <Modal open={confirm} onClose={() => { if (!save.isPending) setConfirm(false); }} title="Fazer a raia valer de verdade?" className="max-w-lg">
        <div className="space-y-4 text-[13px] leading-relaxed">
          <p>A raia <strong>{edited.label}</strong> passará a <strong>enviar mensagens reais</strong> aos clientes pelo WhatsApp do escritório, sem aprovação, sempre que a IA ligada a ela responder.</p>
          <p className="text-muted">Recomendamos deixar alguns dias em sombra e conferir os rascunhos antes. As regras do servidor continuam valendo: conversa com atendente, pedido de uma pessoa, foto ou documento do cliente, limites e a lista de links.</p>
          <div className="flex flex-wrap justify-end gap-2 pt-2"><Button variant="ghost" disabled={save.isPending} onClick={() => setConfirm(false)}>Cancelar</Button><Button loading={save.isPending} onClick={() => save.mutate()}>Fazer valer</Button></div>
        </div>
      </Modal>
    </section>
  );
}
