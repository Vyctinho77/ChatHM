import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { buildCategories, searchEmojis } from "./emojiData";

// Os dados reais em português (emojibase), lidos como arquivo.
const dir = new URL("../../../node_modules/emojibase-data/pt/", import.meta.url);
const raw = JSON.parse(readFileSync(new URL("compact.json", dir), "utf8"));
const messages = JSON.parse(readFileSync(new URL("messages.json", dir), "utf8"));
const cats = buildCategories(raw, messages);
const all = cats.flatMap((c) => c.items);

describe("cartela de emojis", () => {
  it("tem as oito abas, na ordem do WhatsApp, com bastante emoji", () => {
    expect(cats.map((c) => c.key)).toEqual(["smileys", "animals", "food", "activities", "travel", "objects", "symbols", "flags"]);
    expect(all.length).toBeGreaterThan(1500);
    expect(cats.find((c) => c.key === "flags")!.items.length).toBeGreaterThan(200);
  });

  it("não repete emoji nem traz tons de pele ou peças soltas", () => {
    const chars = all.map((e) => e.char);
    expect(new Set(chars).size).toBe(chars.length);
    expect(chars.some((c) => /[\u{1F3FB}-\u{1F3FF}]/u.test(c) && c.length <= 2)).toBe(false); // tom de pele sozinho
  });

  it("os emojis das reações rápidas existem na cartela", () => {
    for (const e of ["👍", "❤️", "😂", "😮", "😢", "🙏"]) expect(all.some((x) => x.char === e), e).toBe(true);
  });

  it("busca em português, sem acento e sem diferenciar maiúsculas", () => {
    const found = (q: string) => searchEmojis(cats, q).map((e) => e.char);
    expect(found("coracao")).toContain("❤️");
    expect(found("CORAÇÃO")).toContain("❤️");
    expect(found("polegar")).toContain("👍");
    expect(found("kkk")).toContain("😂");
    expect(found("gargalhada")).toContain("😂");
    expect(found("dedao")).toContain("👍"); // "dedão"
    expect(found("bandeira brasil")).toContain("🇧🇷");
    expect(found("   ")).toEqual([]);
    expect(found("zzzzqqqq")).toEqual([]);
  });

  it("todas as palavras da busca precisam aparecer", () => {
    expect(searchEmojis(cats, "rosto sorrindo").length).toBeGreaterThan(0);
    expect(searchEmojis(cats, "rosto bandeira").length).toBe(0);
  });
});
