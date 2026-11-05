import type { Message } from "./types";

/** O que a implementação atual permite fazer pela interface. */
export type Capabilities = {
  send: boolean;
  tags: boolean;
  close: boolean;
  /** Envio de áudio, imagens e documentos. */
  media: boolean;
  /** Explicação exibida quando o envio está indisponível. */
  sendUnavailableReason?: string;
  mediaUnavailableReason?: string;
};

export type OutgoingDraft =
  | { kind: "text"; text: string }
  | { kind: "audio"; durationSec: number; waveform: number[]; url: string }
  | { kind: "document"; fileName: string; mimeType: string; sizeBytes: number; url: string }
  | { kind: "image"; url: string; width: number; height: number; caption?: string; hd?: boolean };

export function buildOutgoing(conversationId: string, authorName: string, d: OutgoingDraft): Message {
  return {
    ...d,
    id: crypto.randomUUID(),
    conversationId,
    direction: "out",
    sentAt: new Date().toISOString(),
    status: "pending",
    authorName,
  } as Message;
}
