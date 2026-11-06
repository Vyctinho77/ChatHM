// Modelos do domínio de atendimento (espelham a API).

/** Chave do segmento ("geral" = fila geral). Os segmentos são configuráveis pelo admin. */
export type SegmentKey = string;

export type SegmentOption = { key: SegmentKey; name: string };
export type TagInfo = { id: string; name: string; inUse?: number };

// Cores dos segmentos iniciais; segmentos criados depois usam a cor padrão.
const segmentStyles: Record<string, { label: string; className: string }> = {
  clientes_novos: { label: "Clientes novos", className: "bg-violet-500/15 text-violet-600 dark:text-violet-300" },
  folha: { label: "Folha de pagamento", className: "bg-amber-500/15 text-amber-700 dark:text-amber-300" },
  notas: { label: "Emissão de notas", className: "bg-sky-500/15 text-sky-700 dark:text-sky-300" },
  geral: { label: "Não atribuídos", className: "bg-zinc-500/15 text-muted" },
};
const defaultSegmentClass = "bg-teal-500/15 text-teal-700 dark:text-teal-300";

export const segmentClass = (key: SegmentKey) => segmentStyles[key]?.className ?? defaultSegmentClass;
export const segmentLabel = (c: { segment: SegmentKey; segmentName?: string }) =>
  c.segment === "geral" ? "Não atribuídos" : c.segmentName ?? segmentStyles[c.segment]?.label ?? c.segment;

export type ConversationStatus = "new" | "in_progress" | "closed";

export type DeliveryStatus = "pending" | "sent" | "delivered" | "read" | "failed";

type Base = {
  id: string;
  conversationId: string;
  direction: "in" | "out";
  sentAt: string;
  status?: DeliveryStatus; // apenas mensagens enviadas
  deliveredAt?: string; // recibo de entrega (mensagens enviadas)
  readAt?: string; // recibo de leitura (mensagens enviadas)
  authorName?: string; // atendente que enviou
  historical?: boolean;
  /** Mídia recebida sem arquivo: o download falhou ou foi bloqueado pela verificação de segurança. */
  mediaState?: "failed" | "blocked";
};

// `url` ausente = mídia recebida cujo arquivo ainda não é baixado (download chega na Fase 4).
export type Message = Base &
  (
    | { kind: "text"; text: string }
    | { kind: "audio"; durationSec: number; waveform?: number[]; url?: string }
    | { kind: "document"; fileName: string; mimeType: string; sizeBytes: number; url?: string; caption?: string }
    | { kind: "image"; url?: string; caption?: string; width?: number; height?: number; hd?: boolean }
    | { kind: "location"; lat: number; lng: number; name?: string; address?: string }
    | { kind: "unsupported"; label: string; text?: string }
  );

export type Contact = {
  id: string;
  name: string;
  phone: string;
  company?: string;
  avatarUrl?: string;
  note: string;
};

export type Conversation = {
  id: string;
  contact: Contact;
  segment: SegmentKey;
  segmentName?: string;
  /** Como a fila foi definida: palavra-chave, conflito entre segmentos ou sem correspondência. */
  classification?: "none" | "keyword" | "conflict";
  status: ConversationStatus;
  snoozedUntil?: string;
  assignee?: { id: string; name: string };
  unreadCount: number;
  tags: string[];
  /** Ausente em atendimento recém-iniciado por nós (ainda sem mensagens). */
  lastMessage?: Message;
};

/** Resultado de mensagem na busca. */
export type MessageHit = {
  id: string;
  conversationId: string;
  contactId: string;
  openConversationId?: string;
  contactName: string;
  direction: "in" | "out";
  kind: string;
  snippet: string;
  sentAt: string;
  closed: boolean;
};

export type ContactRecord = Contact & { conversationId?: string };
export type SearchResult = { contacts: ContactRecord[]; messages: MessageHit[] };

