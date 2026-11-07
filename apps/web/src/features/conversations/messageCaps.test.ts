import { describe, expect, it } from "vitest";
import { copyText, messageCapabilities } from "./messageCaps";
import type { Message } from "./types";

const base = { id: "m1", conversationId: "c1", sentAt: "2026-11-07T12:00:00Z" };
const at = (min: number) => Date.parse(base.sentAt) + min * 60_000;
const out = (extra: Partial<Message> = {}) => ({ ...base, direction: "out", kind: "text", text: "olá", status: "read", ...extra }) as Message;
const incoming = (extra: Partial<Message> = {}) => ({ ...base, direction: "in", kind: "text", text: "oi", ...extra }) as Message;

describe("messageCapabilities", () => {
  it("edita só texto enviado pelo escritório, até 20 minutos", () => {
    expect(messageCapabilities(out(), at(5)).edit).toBe(true);
    expect(messageCapabilities(out(), at(21)).edit).toBe(false);
    expect(messageCapabilities(incoming(), at(1)).edit).toBe(false);
    expect(messageCapabilities(out({ kind: "location", lat: 1, lng: 2 } as Partial<Message>), at(1)).edit).toBe(false);
    expect(messageCapabilities(out({ status: "failed" }), at(1)).edit).toBe(false);
  });

  it("apaga para todos só o que o escritório enviou, até 2 dias", () => {
    expect(messageCapabilities(out(), at(60 * 24)).deleteEveryone).toBe(true);
    expect(messageCapabilities(out(), at(60 * 49)).deleteEveryone).toBe(false);
    expect(messageCapabilities(incoming(), at(1)).deleteEveryone).toBe(false);
    expect(messageCapabilities(out({ status: "pending" }), at(1)).deleteEveryone).toBe(false);
  });

  it("mensagem apagada não oferece responder, copiar nem encaminhar", () => {
    const c = messageCapabilities(out({ deleted: true } as Partial<Message>), at(1));
    expect([c.reply, c.copy, c.forward, c.edit, c.deleteEveryone]).toEqual([false, false, false, false, false]);
  });

  it("baixar só aparece com arquivo", () => {
    expect(messageCapabilities(incoming({ kind: "image", url: "/api/v1/attachments/1" } as Partial<Message>), at(1)).download).toBe(true);
    expect(messageCapabilities(incoming({ kind: "image" } as Partial<Message>), at(1)).download).toBe(false);
  });
});

describe("copyText", () => {
  it("copia texto, legenda ou o link da localização", () => {
    expect(copyText(incoming())).toBe("oi");
    expect(copyText(incoming({ kind: "image", caption: "foto" } as Partial<Message>))).toBe("foto");
    expect(copyText(incoming({ kind: "location", lat: -3.1, lng: -60 } as Partial<Message>))).toBe("https://www.google.com/maps?q=-3.1,-60");
    expect(copyText(incoming({ kind: "audio" } as Partial<Message>))).toBe("");
  });
});
