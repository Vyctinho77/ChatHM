// Fonte de dados das conversas: API real no app; dados fictícios só no build de
// demonstração. A condição é resolvida no build, e o bundler descarta o lado não usado.
import * as api from "./store.api";
import * as mock from "./store.mock";

const impl = import.meta.env.VITE_DEMO === "1" ? mock : api;

export const capabilities = impl.capabilities;
export const useConversations = impl.useConversations;
export const useMessages = impl.useMessages;
export const useConversationActions = impl.useConversationActions;
export const useSendMessage = impl.useSendMessage;

export { buildOutgoing } from "./store.types";
export type { Capabilities, OutgoingDraft } from "./store.types";
