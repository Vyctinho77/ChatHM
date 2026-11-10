// Fonte de dados das conversas: API real no app; dados fictícios só no build de
// demonstração. A condição é resolvida no build, e o bundler descarta o lado não usado.
import * as api from "./store.api";
import * as mock from "./store.mock";

const impl = import.meta.env.VITE_DEMO === "1" ? mock : api;

export const capabilities = impl.capabilities;
export const useConversations = impl.useConversations;
export const useMessages = impl.useMessages;
export const usePrefetchMessages = impl.usePrefetchMessages;
export const useConversationActions = impl.useConversationActions;
export const useSendMessage = impl.useSendMessage;
export const useMessageSearch = impl.useMessageSearch;
export const useContactOpener = impl.useContactOpener;

// Segmentos e etiquetas vêm da API tanto no app quanto na demonstração (que simula a API).
export { useSegmentOptions } from "@/features/queues/api";
export { useTagCatalog } from "@/features/tags/api";

export { buildOutgoing } from "./store.types";
export type { Capabilities, OutgoingDraft } from "./store.types";
