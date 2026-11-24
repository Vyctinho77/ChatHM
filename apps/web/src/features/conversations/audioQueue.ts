import { shapeWave, speechWave } from "./waveform";
import type { Track } from "./audioEngine";
import type { Message } from "./types";

/** Chave das mensagens de uma conversa no cache (a mesma da API real e da demonstração). */
export const messagesKey = (conversationId: string) => ["conversations", conversationId, "messages"] as const;

/**
 * O próximo áudio a tocar sozinho depois de `finished`, como no WhatsApp: os áudios recebidos, ainda não ouvidos, que vêm logo depois.
 * Pula o que a equipe enviou, o que foi apagado, o que já foi ouvido e o que ainda não tem arquivo. Uma mensagem do cliente que não é áudio
 * (texto, foto, documento...) encerra a sequência. Devolve null quando não há próximo.
 */
export function nextUnheardTrack(messages: Message[] | undefined, finished: Track, heard: (id: string) => boolean): Track | null {
  if (!messages) return null;
  const at = messages.findIndex((m) => m.id === finished.id);
  if (at < 0) return null;
  for (let i = at + 1; i < messages.length; i++) {
    const m = messages[i]!;
    if (m.deleted || m.direction === "out") continue;
    if (m.kind !== "audio") return null;
    if (!m.url || heard(m.id)) continue;
    const grouped = !!m.group;
    return {
      id: m.id,
      conversationId: finished.conversationId,
      url: m.url,
      label: finished.label,
      durationSec: m.durationSec,
      outgoing: false,
      // Em grupo cada áudio é de uma pessoa; no 1:1 a foto é a do contato.
      avatarName: grouped ? (m.authorName ?? finished.label) : (finished.avatarName ?? finished.label),
      avatarSrc: grouped ? undefined : finished.avatarSrc,
      group: grouped ? !m.authorName : finished.group,
      wave: m.waveform?.length ? shapeWave(m.waveform) : speechWave(m.id),
    };
  }
  return null;
}
