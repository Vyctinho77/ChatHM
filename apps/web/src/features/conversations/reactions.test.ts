import { describe, expect, it } from "vitest";
import { QUICK_REACTIONS, myReaction, nextReaction, withMyReaction, type Reaction } from "./reactions";
import type { Message } from "./types";

const msg = (reactions?: Reaction[]) => ({ id: "m1", reactions }) as unknown as Message;

describe("reações da tela (antes de o servidor confirmar)", () => {
  it("a primeira reação entra como minha", () => {
    expect(withMyReaction(undefined, "👍")).toEqual([{ emoji: "👍", count: 1, mine: true }]);
  });

  it("trocar de emoji substitui a minha (uma por conta) e não mexe nas dos outros", () => {
    const before: Reaction[] = [{ emoji: "👍", count: 1, mine: true }, { emoji: "🙏", count: 1, mine: false }];
    expect(withMyReaction(before, "❤️")).toEqual([{ emoji: "🙏", count: 1, mine: false }, { emoji: "❤️", count: 1, mine: true }]);
  });

  it("reagir com o mesmo emoji de outra pessoa soma na contagem", () => {
    expect(withMyReaction([{ emoji: "🙏", count: 1, mine: false }], "🙏")).toEqual([{ emoji: "🙏", count: 2, mine: true }]);
  });

  it("tirar a minha reduz a contagem; se era só minha, o emoji some", () => {
    expect(withMyReaction([{ emoji: "🙏", count: 2, mine: true }], "")).toEqual([{ emoji: "🙏", count: 1, mine: false }]);
    expect(withMyReaction([{ emoji: "👍", count: 1, mine: true }], "")).toEqual([]);
  });

  it("não altera a lista original", () => {
    const before: Reaction[] = [{ emoji: "👍", count: 1, mine: true }];
    withMyReaction(before, "❤️");
    expect(before).toEqual([{ emoji: "👍", count: 1, mine: true }]);
  });
});

describe("escolher a reação", () => {
  it("o mesmo emoji de novo remove; outro troca", () => {
    const m = msg([{ emoji: "👍", count: 1, mine: true }]);
    expect(myReaction(m)).toBe("👍");
    expect(nextReaction(m, "👍")).toBe("");
    expect(nextReaction(m, "❤️")).toBe("❤️");
    expect(nextReaction(msg(), "👍")).toBe("👍");
  });

  it("a barra rápida tem as seis reações do WhatsApp", () => {
    expect([...QUICK_REACTIONS]).toEqual(["👍", "❤️", "😂", "😮", "😢", "🙏"]);
  });
});
