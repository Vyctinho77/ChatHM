import { useMemo } from "react";
import { useNavigate } from "react-router";
import { Layers, SlidersHorizontal } from "lucide-react";
import { CountBadge, ListRow, Page, PageBody, PageHeader, Section } from "@/components/list";
import { Alert, IconButton, Spinner } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { useConversations } from "@/features/conversations/store";
import type { Conversation } from "@/features/conversations/types";
import { useTeams } from "@/features/team/api";
import { useSegments } from "./api";

type Queue = { key: string; name: string; teamName?: string; isActive: boolean; general?: boolean };

function counts(list: Conversation[], key: string) {
  let waiting = 0;
  let active = 0;
  for (const c of list) {
    if (c.segment !== key) continue;
    if (c.status === "new") waiting++;
    else if (c.status === "in_progress") active++;
  }
  return { waiting, active };
}

/** Uma linha de apoio curta: equipe e andamento. */
function subtitle(q: Queue, n: { waiting: number; active: number }) {
  const team = q.general ? "Todas as equipes" : q.teamName ? `Equipe ${q.teamName}` : "Sem equipe";
  const progress =
    n.active > 0 ? `${n.active} em atendimento` : n.waiting > 0 ? "ninguém atendendo ainda" : "nenhuma conversa";
  return `${q.isActive ? "" : "Desativada · "}${team} · ${progress}`;
}

export function QueuesPage() {
  const navigate = useNavigate();
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const { data: conversations = [] } = useConversations();
  const segments = useSegments();
  const teams = useTeams();

  // Quem não é admin só vê as filas da própria equipe (e as sem equipe), além da geral.
  const queues = useMemo<Queue[]>(() => {
    const myTeams = new Set((teams.data ?? []).map((t) => t.id));
    const visible = (segments.data ?? []).filter((s) => isAdmin || !s.teamId || myTeams.has(s.teamId));
    return [
      ...visible.map((s) => ({ key: s.key, name: s.name, teamName: s.teamName, isActive: s.isActive })),
      { key: "geral", name: "Geral", isActive: true, general: true },
    ];
  }, [segments.data, teams.data, isAdmin]);

  return (
    <Page>
      <PageHeader
        title="Filas"
        actions={
          isAdmin && (
            <IconButton label="Configurar filas" onClick={() => navigate("/filas/configurar")}>
              <SlidersHorizontal className="size-5" />
            </IconButton>
          )
        }
      />
      <PageBody>
        {segments.isPending && <Spinner className="mx-auto mt-10" />}
        {segments.isError && (
          <div className="p-4">
            <Alert>Não foi possível carregar as filas.</Alert>
          </div>
        )}
        {!segments.isPending && !segments.isError && segments.data?.length === 0 && (
          <p className="px-4 pt-6 text-[13px] text-muted">
            Ainda não há segmentos: todas as conversas ficam na fila Geral.
            {isAdmin && " Crie os segmentos do escritório em Configurar filas (ícone de ajustes)."}
          </p>
        )}
        {!segments.isPending && !segments.isError && (
          <Section
            note="Cada mensagem nova vai para a fila do segmento que combina com ela. Sem correspondência clara, fica na fila Geral."
          >
            {queues.map((q) => {
              const n = counts(conversations, q.key);
              return (
                <ListRow
                  key={q.key}
                  icon={<Layers />}
                  title={q.name}
                  subtitle={subtitle(q, n)}
                  trailing={<CountBadge n={n.waiting} />}
                  chevron
                  to={`/conversas?fila=${q.key}`}
                />
              );
            })}
          </Section>
        )}
      </PageBody>
    </Page>
  );
}
