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

/** `file` é o conteúdo a enviar; `url` é só a prévia local (blob:) da bolha. */
export type OutgoingDraft =
  | { kind: "text"; text: string }
  | { kind: "audio"; durationSec: number; waveform: number[]; url: string; file?: Blob }
  | { kind: "document"; fileName: string; mimeType: string; sizeBytes: number; url: string; file?: Blob }
  | { kind: "image"; url: string; width: number; height: number; caption?: string; hd?: boolean; file?: Blob }
  | { kind: "location"; lat: number; lng: number; name?: string; address?: string };

/** Arquivos de mensagens ainda não confirmadas (para "tentar de novo" sem pedir o arquivo outra vez). */
export const pendingFiles = new Map<string, Blob>();

export function buildOutgoing(conversationId: string, authorName: string, d: OutgoingDraft): Message {
  const id = crypto.randomUUID();
  const rest = { ...d } as OutgoingDraft & { file?: Blob };
  if (rest.kind !== "text" && rest.kind !== "location" && rest.file) pendingFiles.set(id, rest.file);
  delete (rest as { file?: Blob }).file;
  return {
    ...rest,
    id,
    conversationId,
    direction: "out",
    sentAt: new Date().toISOString(),
    status: "pending",
    authorName,
  } as Message;
}
