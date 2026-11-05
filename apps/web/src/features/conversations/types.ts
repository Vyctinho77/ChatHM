// Modelos do domínio de atendimento. Espelham o que a API da Fase 2/3 vai expor.

export type SegmentKey = "clientes_novos" | "folha" | "notas" | "geral";

export const segments: Record<SegmentKey, { label: string; className: string }> = {
  clientes_novos: { label: "Clientes novos", className: "bg-violet-500/15 text-violet-600 dark:text-violet-300" },
  folha: { label: "Folha de pagamento", className: "bg-amber-500/15 text-amber-700 dark:text-amber-300" },
  notas: { label: "Emissão de notas", className: "bg-sky-500/15 text-sky-700 dark:text-sky-300" },
  geral: { label: "Geral", className: "bg-zinc-500/15 text-muted" },
};

export type ConversationStatus = "new" | "in_progress" | "closed";

export type DeliveryStatus = "pending" | "sent" | "delivered" | "read" | "failed";

type Base = {
  id: string;
  conversationId: string;
  direction: "in" | "out";
  sentAt: string;
  status?: DeliveryStatus; // apenas mensagens enviadas
  authorName?: string; // atendente que enviou
};

export type Message = Base &
  (
    | { kind: "text"; text: string }
    | { kind: "audio"; durationSec: number; waveform: number[]; url?: string }
    | { kind: "document"; fileName: string; mimeType: string; sizeBytes: number; url?: string }
    | { kind: "image"; url: string; caption?: string; width: number; height: number; hd?: boolean }
    | { kind: "location"; lat: number; lng: number; name?: string; address?: string }
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
  status: ConversationStatus;
  assignee?: { id: string; name: string };
  unreadCount: number;
  tags: string[];
  lastMessage: Message;
};

export const tagCatalog = ["PENDENTE", "URGENTE", "PGDAS", "CERTIFICADO", "ACOMPANHAMENTO"] as const;
