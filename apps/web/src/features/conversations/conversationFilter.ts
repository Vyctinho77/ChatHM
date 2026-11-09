import type { Conversation } from "./types";

export type ConversationFilter = "mine" | "general" | "groups" | "all" | "snoozed";

export function filterConversations(conversations: Conversation[], filter: ConversationFilter, myId: string, segment: string | null) {
  return conversations.filter((c) => {
    if (c.status === "closed") return false;
    // Only the server clears the deadline, so a pending return never appears in both tabs.
    if (filter === "snoozed" ? !c.snoozedUntil : !!c.snoozedUntil) return false;
    if (filter === "mine" && c.assignee?.id !== myId) return false;
    // "Não atribuídos" = fila geral e sem responsável: atendimento iniciado (com responsável) sai desta aba.
    if (filter === "general" && (c.segment !== "geral" || c.assignee)) return false;
    if (filter === "groups" && c.contact.kind !== "group") return false;
    if (segment === "geral" && c.assignee) return false; // o chip "Não atribuídos" da fila geral segue a mesma regra
    return !segment || c.segment === segment;
  }).sort((a, b) => filter === "snoozed" ? Date.parse(a.snoozedUntil!) - Date.parse(b.snoozedUntil!) : 0);
}

export type TabUnread = Record<Exclude<ConversationFilter, "snoozed">, number>;

/** Conversas com mensagens novas em cada aba (o número ao lado de "Minhas", "Não atribuídos"...). Adiadas e encerradas não contam. */
export function unreadByTab(conversations: Conversation[], myId: string): TabUnread {
  const count = (filter: ConversationFilter) => filterConversations(conversations, filter, myId, null).filter((c) => c.unreadCount > 0).length;
  return { mine: count("mine"), general: count("general"), groups: count("groups"), all: count("all") };
}
