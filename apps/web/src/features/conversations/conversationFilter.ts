import type { Conversation } from "./types";

export type ConversationFilter = "mine" | "general" | "all" | "snoozed";

export function filterConversations(conversations: Conversation[], filter: ConversationFilter, myId: string, segment: string | null) {
  return conversations.filter((c) => {
    if (c.status === "closed") return false;
    // Only the server clears the deadline, so a pending return never appears in both tabs.
    if (filter === "snoozed" ? !c.snoozedUntil : !!c.snoozedUntil) return false;
    if (filter === "mine" && c.assignee?.id !== myId) return false;
    if (filter === "general" && c.segment !== "geral") return false;
    return !segment || c.segment === segment;
  }).sort((a, b) => filter === "snoozed" ? Date.parse(a.snoozedUntil!) - Date.parse(b.snoozedUntil!) : 0);
}
