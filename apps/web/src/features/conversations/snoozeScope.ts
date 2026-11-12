import type { Conversation } from "./types";

/** Adiado para toda a equipe (bloqueia a resposta de todos), em vez de só para quem está vendo. */
export const snoozedForAll = (c: Conversation) => !!c.snoozedUntil && c.snoozedFor !== "me";
