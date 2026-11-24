import { describe, expect, it } from "vitest";
import { nextUnheardTrack } from "./audioQueue";
import type { Track } from "./audioEngine";
import type { Message } from "./types";

const finished: Track = { id: "a1", conversationId: "c1", url: "/api/v1/attachments/a1", label: "Cliente", avatarName: "Cliente", avatarSrc: "/foto.jpg", durationSec: 10, outgoing: false };
const audio = (id: string, o: Record<string, unknown> = {}) => ({ id, conversationId: "c1", direction: "in", kind: "audio", durationSec: 7, url: `/api/v1/attachments/${id}`, sentAt: "2026-11-24T10:00:00Z", ...o }) as unknown as Message;
const other = (id: string, kind: string, o: Record<string, unknown> = {}) => ({ id, conversationId: "c1", direction: "in", kind, text: "oi", sentAt: "2026-11-24T10:00:00Z", ...o }) as unknown as Message;
const none = () => false;

describe("próximo áudio da sequência", () => {
  it("o primeiro áudio recebido ainda não ouvido logo depois", () => {
    const next = nextUnheardTrack([audio("a1"), audio("a2"), audio("a3")], finished, none);
    expect(next?.id).toBe("a2");
    expect(next).toMatchObject({ conversationId: "c1", label: "Cliente", avatarName: "Cliente", avatarSrc: "/foto.jpg", outgoing: false, durationSec: 7, url: "/api/v1/attachments/a2" });
    expect(next?.wave?.length).toBeGreaterThan(0);
  });

  it("pula o que já foi ouvido", () => {
    expect(nextUnheardTrack([audio("a1"), audio("a2"), audio("a3")], finished, (id) => id === "a2")?.id).toBe("a3");
  });

  it("pula o que a equipe enviou, o apagado e o que ainda não tem arquivo", () => {
    const msgs = [audio("a1"), audio("o1", { direction: "out" }), audio("d1", { deleted: true }), audio("s1", { url: undefined }), audio("a2")];
    expect(nextUnheardTrack(msgs, finished, none)?.id).toBe("a2");
  });

  it("uma mensagem do cliente que não é áudio encerra a sequência", () => {
    expect(nextUnheardTrack([audio("a1"), other("t1", "text"), audio("a2")], finished, none)).toBeNull();
    expect(nextUnheardTrack([audio("a1"), other("i1", "image"), audio("a2")], finished, none)).toBeNull();
  });

  it("mensagem nossa no meio não encerra", () => {
    expect(nextUnheardTrack([audio("a1"), other("t1", "text", { direction: "out" }), audio("a2")], finished, none)?.id).toBe("a2");
  });

  it("sem próximo, sem lista ou com o áudio fora da lista: null", () => {
    expect(nextUnheardTrack([audio("a1")], finished, none)).toBeNull();
    expect(nextUnheardTrack([audio("a1"), audio("a2")], finished, () => true)).toBeNull();
    expect(nextUnheardTrack(undefined, finished, none)).toBeNull();
    expect(nextUnheardTrack([audio("x")], finished, none)).toBeNull();
  });

  it("em grupo, cada áudio mostra quem o enviou (sem a foto do contato)", () => {
    const next = nextUnheardTrack([audio("a1", { group: true }), audio("g2", { group: true, authorName: "Marta" })], finished, none);
    expect(next).toMatchObject({ id: "g2", avatarName: "Marta", avatarSrc: undefined, group: false });
  });
});
