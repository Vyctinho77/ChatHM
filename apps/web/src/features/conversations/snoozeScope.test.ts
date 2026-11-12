import { describe, expect, it } from "vitest";
import { snoozedForAll } from "./snoozeScope";
import type { Conversation } from "./types";

const base = { id: "1" } as Conversation;

describe("snoozedForAll", () => {
  it("só bloqueia a resposta quando a conversa está adiada para todos", () => {
    expect(snoozedForAll({ ...base })).toBe(false);
    expect(snoozedForAll({ ...base, snoozedUntil: "2030-01-01T00:00:00Z", snoozedFor: "all" })).toBe(true);
    expect(snoozedForAll({ ...base, snoozedUntil: "2030-01-01T00:00:00Z" })).toBe(true); // sem o campo: comportamento de antes
    expect(snoozedForAll({ ...base, snoozedUntil: "2030-01-01T00:00:00Z", snoozedFor: "me" })).toBe(false);
  });
});
