// Agenda persistida na API. A demonstração mantém seus exemplos em memória.
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTeams } from "@/features/team/api";
import { addDays, toISO } from "./dates";
import { DEMO } from "@/lib/env";
import { api, get, del, patch } from "@/lib/api";

export type AgendaKind = "appointment" | "deadline";

export type AgendaItem = {
  id: string;
  kind: AgendaKind;
  title: string;
  date: string; // YYYY-MM-DD
  time?: string; // HH:MM (só compromissos)
  team: string; // nome da equipe
  teamId?: string;
  contact?: { name: string; conversationId: string };
  note: string;
  done: boolean;
};

export const kindLabel: Record<AgendaKind, string> = { appointment: "Compromisso", deadline: "Prazo" };

/** Cor do marcador: azul para compromissos, âmbar para prazos (únicas cores da agenda). */
export const kindDot = (kind: AgendaKind) => (kind === "deadline" ? "bg-warning" : "bg-accent");

export function seedItems(today: Date): AgendaItem[] {
  const d = (n: number) => toISO(addDays(today, n));
  const monthDay = (day: number) => toISO(new Date(today.getFullYear(), today.getMonth(), day));
  const nextMonth = (day: number) => toISO(new Date(today.getFullYear(), today.getMonth() + 1, day));
  let i = 0;
  const item = (x: Omit<AgendaItem, "id" | "done" | "note"> & Partial<Pick<AgendaItem, "note" | "done">>): AgendaItem => ({
    id: `seed-${++i}`, note: "", done: false, ...x,
  });
  return [
    item({ kind: "appointment", title: "Retorno: orçamento da confeitaria", date: d(0), time: "16:00", team: "Clientes novos", contact: { name: "Mel Confeitaria", conversationId: "c5" }, note: "Cliente pediu proposta com valores para contabilidade completa." }),
    item({ kind: "appointment", title: "Reunião: abertura de MEI", date: d(1), time: "10:00", team: "Clientes novos", contact: { name: "Otávio Nunes", conversationId: "c9" }, note: "Levar a lista de documentos (RG, CPF, comprovante de endereço)." }),
    item({ kind: "appointment", title: "Ligar sobre as férias", date: d(2), time: "09:00", team: "Folha de pagamento", contact: { name: "Silvia Andrade Clínica", conversationId: "c8" }, note: "Confirmar período e se há abono." }),
    item({ kind: "appointment", title: "Revisar folha da clínica", date: d(2), time: "11:30", team: "Folha de pagamento" }),
    item({ kind: "appointment", title: "Alinhar notas pendentes", date: d(2), time: "15:00", team: "Emissão de notas", contact: { name: "Rafael Moreira", conversationId: "c1" } }),
    item({ kind: "appointment", title: "Treinar a equipe nova", date: d(5), time: "14:00", team: "Folha de pagamento" }),
    item({ kind: "deadline", title: "FGTS: guia", date: monthDay(7), team: "Folha de pagamento", done: true }),
    item({ kind: "deadline", title: "eSocial: folha", date: monthDay(15), team: "Folha de pagamento", note: "Fechamento do evento mensal." }),
    item({ kind: "deadline", title: "DAS (Simples Nacional)", date: monthDay(20), team: "Emissão de notas", note: "Vencimento do PGDAS-D do mês." }),
    item({ kind: "deadline", title: "Certificado digital vence", date: d(10), team: "Emissão de notas", contact: { name: "Luciana Prado Arquitetura", conversationId: "c4" }, note: "Avisar a cliente e agendar a renovação." }),
    item({ kind: "deadline", title: "DAS (Simples Nacional)", date: nextMonth(20), team: "Emissão de notas" }),
  ];
}

// --- Armazenamento em memória -------------------------------------------------

let db: AgendaItem[] | null = null;
const store = () => (db ??= seedItems(new Date()));
const key = ["agenda"] as const;

/** Itens que o usuário pode ver: só os das equipes dele (admin vê todas), como nas filas. */
export function useAgendaItems() {
  const teams = useTeams();
  const visible = useMemo(() => new Set((teams.data ?? []).map((t) => t.name)), [teams.data]);
  const q = useQuery({ queryKey: key, queryFn: async () => DEMO ? [...store()] : (await get<{items: AgendaItem[]}>("/agenda")).items, refetchInterval: DEMO ? false : 15_000 });
  const items = useMemo(() => (q.data ?? []).filter((i) => visible.has(i.team)), [q.data, visible]);
  return { items, teamNames: [...visible].sort(), isPending: q.isPending || teams.isPending, error: q.error ?? teams.error };
}

function useAgendaMutation<V>(fn: (v: V) => void | Promise<unknown>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: V) => fn(v),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });
}

export const useSaveItem = () => {
  const teams = useTeams();
  return useAgendaMutation(async (item: AgendaItem) => {
    if (!DEMO) {
      const teamId = teams.data?.find((t) => t.name === item.team)?.id;
      if (!teamId) throw new Error("Escolha uma equipe válida.");
      return api<AgendaItem>("PUT", `/agenda/${item.id}`, {kind:item.kind,title:item.title,date:item.date,time:item.time ?? "",teamId,conversationId:item.contact?.conversationId ?? "",note:item.note,done:item.done});
    }
    const list = store();
    const i = list.findIndex((x) => x.id === item.id);
    if (i >= 0) list[i] = item;
    else list.push(item);
  });
};

export const useDeleteItem = () =>
  useAgendaMutation(async (id: string) => {
    if (!DEMO) return del(`/agenda/${id}`);
    db = store().filter((x) => x.id !== id);
  });

export const useToggleDone = () => {
  const qc = useQueryClient();
  return useAgendaMutation(async (id: string) => {
    if (!DEMO) {
      const item = qc.getQueryData<AgendaItem[]>(key)?.find((i) => i.id === id);
      if (!item) throw new Error("Item não encontrado.");
      return patch(`/agenda/${id}/done`, {done:!item.done});
    }
    const it = store().find((x) => x.id === id);
    if (it) it.done = !it.done;
  });
};

/** Compromissos antes dos prazos? Não: prazos (dia inteiro) primeiro, depois por horário. */
export function sortItems(items: AgendaItem[]) {
  return [...items].sort((a, b) => {
    if (a.kind !== b.kind) return a.kind === "deadline" ? -1 : 1;
    return (a.time ?? "").localeCompare(b.time ?? "") || a.title.localeCompare(b.title);
  });
}
