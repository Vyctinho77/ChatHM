import { describe, expect, it } from "vitest";
import type { Message } from "@/features/conversations/types";
import { internalCapabilities } from "./caps";

const NOW = Date.parse("2026-11-12T12:00:00Z");
const msg = (over: Partial<Message> & Record<string, unknown> = {}) =>
  ({ id: "m", conversationId: "room", direction: "in", kind: "text", text: "Olá", sentAt: "2026-11-12T11:00:00Z", authorId: "bia", ...over }) as Message;

describe("internalCapabilities", () => {
  it("lets the author edit and delete their own text, and nothing from WhatsApp", () => {
    const caps = internalCapabilities(msg({ authorId: "me", direction: "out" }), { meId: "me", canModerate: false, now: NOW });
    expect(caps).toMatchObject({ reply: true, copy: true, edit: true, deleteEveryone: true, forward: false, info: false });
  });

  it("does not let an agent edit or delete someone else's message, but a manager or admin deletes (never edits)", () => {
    expect(internalCapabilities(msg(), { meId: "me", canModerate: false, now: NOW })).toMatchObject({ edit: false, deleteEveryone: false, reply: true });
    expect(internalCapabilities(msg(), { meId: "me", canModerate: true, now: NOW })).toMatchObject({ edit: false, deleteEveryone: true });
  });

  it("closes editing after 24 hours but keeps deleting", () => {
    const old = msg({ authorId: "me", direction: "out", sentAt: "2026-11-11T11:00:00Z" });
    expect(internalCapabilities(old, { meId: "me", canModerate: false, now: NOW })).toMatchObject({ edit: false, deleteEveryone: true });
  });

  it("only edits text, and offers nothing on deleted or unconfirmed messages", () => {
    const loc = msg({ authorId: "me", kind: "location", lat: 1, lng: 1 } as never);
    expect(internalCapabilities(loc, { meId: "me", canModerate: false, now: NOW }).edit).toBe(false);
    const deleted = msg({ authorId: "me", deleted: true });
    expect(internalCapabilities(deleted, { meId: "me", canModerate: true, now: NOW })).toMatchObject({ reply: false, copy: false, edit: false, deleteEveryone: false });
    const pending = msg({ authorId: "me", direction: "out", status: "pending" });
    expect(internalCapabilities(pending, { meId: "me", canModerate: false, now: NOW })).toMatchObject({ reply: false, edit: false, deleteEveryone: false });
  });

  it("offers a download only for files", () => {
    expect(internalCapabilities(msg({ kind: "image", url: "/api/v1/internal-chat/attachments/a" } as never), { meId: "me", canModerate: false, now: NOW }).download).toBe(true);
    expect(internalCapabilities(msg(), { meId: "me", canModerate: false, now: NOW }).download).toBe(false);
  });
});
