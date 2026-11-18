import type { Conversation } from "./types";

export type ConversationFilter = "mine" | "general" | "groups" | "all" | "snoozed";

/** A tela principal abre em "Não atribuídos": quem ainda não tem atendente é o que a equipe precisa ver primeiro; "Todas" é uma escolha. */
export const DEFAULT_FILTER: ConversationFilter = "general";
const FILTERS: readonly ConversationFilter[] = ["mine", "general", "groups", "all", "snoozed"];

/** O filtro pedido na URL (?vista=); sem pedido ou com valor desconhecido, o padrão. */
export function parseFilter(v: string | null): ConversationFilter {
  return FILTERS.find((f) => f === v) ?? DEFAULT_FILTER;
}

/** O que vai na URL para o filtro: nada para o padrão. */
export const filterParam = (f: ConversationFilter): string | null => (f === DEFAULT_FILTER ? null : f);

export function filterConversations(conversations: Conversation[], filter: ConversationFilter, myId: string, segment: string | null) {
  return conversations.filter((c) => {
    if (c.status === "closed") return false;
    // Only the server clears the deadline, so a pending return never appears in both tabs.
    if (filter === "snoozed" ? !c.snoozedUntil : !!c.snoozedUntil) return false;
    // "Minhas": as que eu atendo e as em que fui incluída como participante.
    if (filter === "mine" && c.assignee?.id !== myId && !c.participants?.some((p) => p.id === myId)) return false;
    // "Não atribuídos" = fila geral e sem responsável: atendimento iniciado (com responsável) sai desta aba.
    if (filter === "general" && (c.segment !== "geral" || c.assignee)) return false;
    if (filter === "groups" && c.contact.kind !== "group") return false;
    if (segment === "geral" && c.assignee) return false; // o chip "Não atribuídos" da fila geral segue a mesma regra
    return !segment || c.segment === segment;
  }).sort((a, b) => filter === "snoozed" ? Date.parse(a.snoozedUntil!) - Date.parse(b.snoozedUntil!) : 0);
}

export type TabCounts = Record<ConversationFilter, { contacts: number; unread: number }>;

/** O total acompanha os contatos da fila; ler mensagens muda somente a notificação. */
export function countsByTab(conversations: Conversation[], myId: string, segment: string | null = null): TabCounts {
  const count = (filter: ConversationFilter) => {
    const visible = filterConversations(conversations, filter, myId, segment);
    return {
      contacts: new Set(visible.map((c) => c.contact.id)).size,
      unread: new Set(visible.filter((c) => c.unreadCount > 0).map((c) => c.contact.id)).size,
    };
  };
  return { mine: count("mine"), general: count("general"), groups: count("groups"), all: count("all"), snoozed: count("snoozed") };
}
