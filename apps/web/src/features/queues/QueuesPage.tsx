import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ArrowRight } from "lucide-react";
import { Alert, Spinner } from "@/components/ui";
import { cn } from "@/lib/cn";
import { useMe } from "@/features/auth/api";
import { useConversations } from "@/features/conversations/store";
import { segmentClass, type Conversation } from "@/features/conversations/types";
import { useTeams } from "@/features/team/api";
import { SegmentConfig, NewSegmentButton, ClassifyTester } from "./SegmentConfig";
import { useSegments, type Segment } from "./api";

type Queue = { key: string; name: string; teamName?: string; isActive: boolean; general?: boolean };

function counts(list: Conversation[], key: string) {
  let waiting = 0;
  let active = 0;
  let unread = 0;
  for (const c of list) {
    if (c.segment !== key) continue;
    if (c.status === "new") waiting++;
    if (c.status === "in_progress") active++;
    unread += c.unreadCount > 0 ? 1 : 0;
  }
  return { waiting, active, unread };
}

function QueueCard({ q, list }: { q: Queue; list: Conversation[] }) {
  const n = counts(list, q.key);
  const teamLine = q.general
    ? "Visível a todas as equipes"
    : q.teamName
      ? `Equipe ${q.teamName}`
      : "Sem equipe vinculada · visível a todos";
  return (
    <article className={cn("flex flex-col gap-3 rounded-xl border border-line bg-panel p-4", !q.isActive && "opacity-60")}>
      <header className="flex flex-wrap items-center gap-2">
        <span className={cn("rounded-full px-2.5 py-0.5 text-[12px] font-medium", segmentClass(q.key))}>{q.name}</span>
        {!q.isActive && <span className="text-[11px] text-muted">inativo</span>}
      </header>
      <p className="-mt-1 text-[12px] text-muted">{teamLine}</p>
      <dl className="grid grid-cols-2 gap-3">
        <div className="rounded-lg bg-hover px-3 py-2">
          <dd className="text-2xl font-semibold tabular-nums">{n.waiting}</dd>
          <dt className="text-[11.5px] text-muted">Aguardando</dt>
        </div>
        <div className="rounded-lg bg-hover px-3 py-2">
          <dd className="text-2xl font-semibold tabular-nums">{n.active}</dd>
          <dt className="text-[11.5px] text-muted">Em atendimento</dt>
        </div>
      </dl>
      <Link
        to={`/conversas?fila=${q.key}`}
        className="mt-auto inline-flex items-center gap-1 text-[13px] font-medium text-accent hover:underline"
      >
        Abrir fila <ArrowRight className="size-3.5" />
        {n.unread > 0 && <span className="ml-auto text-[11.5px] font-normal text-muted">{n.unread} com mensagens novas</span>}
      </Link>
    </article>
  );
}

export function QueuesPage() {
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const [tab, setTab] = useState<"queues" | "config">("queues");
  const { data: conversations = [], isPending: loadingConvs } = useConversations();
  const segments = useSegments();
  const teams = useTeams();

  // Quem não é admin só vê as filas da própria equipe (e as sem equipe), além da geral.
  const queues = useMemo<Queue[]>(() => {
    const myTeams = new Set((teams.data ?? []).map((t) => t.id));
    const visible = (segments.data ?? []).filter((s: Segment) => isAdmin || !s.teamId || myTeams.has(s.teamId));
    return [
      ...visible.map((s) => ({ key: s.key, name: s.name, teamName: s.teamName, isActive: s.isActive })),
      { key: "geral", name: "Geral", isActive: true, general: true },
    ];
  }, [segments.data, teams.data, isAdmin]);

  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto max-w-5xl px-4 py-6 sm:px-8">
        <h1 className="text-xl font-semibold">Filas / Segmentos</h1>
        <p className="mt-1 text-[13px] text-muted">
          Cada mensagem nova é classificada por palavras-chave e vai direto para a fila da equipe. Quem responder
          primeiro assume o atendimento.
        </p>

        {isAdmin && (
          <div role="tablist" className="mt-5 flex gap-1 border-b border-line">
            {(
              [
                ["queues", "Filas"],
                ["config", "Configuração"],
              ] as const
            ).map(([k, label]) => (
              <button
                key={k}
                role="tab"
                aria-selected={tab === k}
                onClick={() => setTab(k)}
                className={cn(
                  "-mb-px border-b-2 px-4 py-2 text-[13px] font-medium",
                  tab === k ? "border-accent text-fg" : "border-transparent text-muted hover:text-fg",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        )}

        <div className="mt-5">
          {(segments.isPending || loadingConvs) && <Spinner className="mx-auto" />}
          {segments.isError && <Alert>Não foi possível carregar as filas.</Alert>}

          {tab === "queues" && !segments.isPending && (
            <>
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {queues.map((q) => (
                  <QueueCard key={q.key} q={q} list={conversations} />
                ))}
              </div>
              <p className="mt-4 rounded-lg border border-dashed border-line px-4 py-3 text-[12.5px] text-muted">
                A fila <strong className="font-medium text-fg">Geral</strong> recebe mensagens sem correspondência clara
                (como &ldquo;bom dia&rdquo;) ou que combinam com mais de um segmento. Ela aparece para todas as equipes,
                sem cópias da conversa.
              </p>
            </>
          )}

          {tab === "config" && isAdmin && segments.data && (
            <div className="flex flex-col gap-4">
              <ClassifyTester segments={segments.data} />
              <div className="flex items-center justify-between">
                <h2 className="text-[15px] font-semibold">Segmentos e palavras-chave</h2>
                <NewSegmentButton teams={teams.data ?? []} />
              </div>
              {segments.data.map((s) => (
                <SegmentConfig key={s.id} segment={s} teams={teams.data ?? []} />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
