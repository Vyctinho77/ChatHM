// Encerrar o atendimento: para todos, ou só a própria parte. Que ações a pessoa tem depende de como ela está no atendimento.
import type { Conversation } from "./types";

export type EndKind = "leave" | "all";

export type EndOptions = {
  /** Ações disponíveis, na ordem em que aparecem. */
  actions: EndKind[];
  /** Uma única ação que encerra tudo e não pede confirmação (a pessoa é a única no atendimento). */
  direct: boolean;
  /** Pessoas no atendimento além de quem olha: responsável (se for outra pessoa) e participantes. */
  others: { id: string; name: string }[];
};

/** Quem mais está no atendimento além de `myId`. */
export function otherPeople(c: Pick<Conversation, "assignee" | "participants">, myId: string) {
  return [...(c.assignee && c.assignee.id !== myId ? [c.assignee] : []), ...(c.participants ?? []).filter((p) => p.id !== myId)];
}

/**
 * - quem está no atendimento (responsável ou participante) pode encerrar só a própria parte ("leave");
 * - responsável, gestor e administrador podem encerrar para todos ("all");
 * - sozinha no atendimento, a pessoa só tem "Encerrar atendimento" (as duas coisas são iguais), sem confirmação.
 */
export function endOptions(c: Pick<Conversation, "assignee" | "participants" | "status">, o: { myId: string; supervisor: boolean }): EndOptions {
  const others = otherPeople(c, o.myId);
  const mine = c.assignee?.id === o.myId;
  const involved = mine || !!c.participants?.some((p) => p.id === o.myId);
  if (c.status === "closed") return { actions: [], direct: false, others };
  if (mine && others.length === 0) return { actions: ["all"], direct: true, others };
  const actions: EndKind[] = [];
  if (involved) actions.push("leave");
  if (mine || o.supervisor) actions.push("all");
  return { actions, direct: false, others };
}

export const endLabel: Record<EndKind, string> = { leave: "Encerrar minha parte", all: "Encerrar para todos" };

/** Frase curta sobre o que cada ação faz, para o menu. */
export function endHint(kind: EndKind, others: { name: string }[]): string {
  const names = joinNames(others.map((p) => p.name));
  if (kind === "leave") return others.length ? `Você sai; o atendimento continua com ${names}` : "Você sai; o atendimento volta à fila";
  return others.length ? `Também para ${names}; o cliente volta à fila` : "O cliente que escrever depois volta à fila";
}

/** "Ana", "Ana e Bia", "Ana, Bia e Caio". */
export function joinNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} e ${names[names.length - 1]}`;
}
