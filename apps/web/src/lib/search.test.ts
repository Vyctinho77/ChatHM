import { describe, expect, it } from "vitest";
import { countHits, fold, highlightParts, matches } from "./search";

describe("busca", () => {
  it("ignora acentos e maiúsculas", () => {
    expect(fold("Ação de FÉRIAS")).toBe("acao de ferias");
    expect(matches("Folha de Pagamento — Outubro", "pagamênto")).toBe(true);
    expect(matches("João", "joao")).toBe(true);
    expect(matches("Maria", "joao")).toBe(false);
  });
  it("casa pedaços do telefone formatado", () => {
    expect(matches("+55 92 99811-2040", "99811")).toBe(true);
    expect(matches("+55 92 99811-2040", "5592998")).toBe(true);
    expect(matches("+55 92 99811-2040", "12")).toBe(false); // menos de 3 dígitos não vale
  });
  it("destaca preservando o texto original", () => {
    const parts = highlightParts("Olá, preciso da FOLHA e da folha", "folha");
    expect(parts.map((p) => p.text).join("")).toBe("Olá, preciso da FOLHA e da folha");
    expect(parts.filter((p) => p.hit).map((p) => p.text)).toEqual(["FOLHA", "folha"]);
    expect(countHits("ação AÇÃO", "acao")).toBe(2);
    expect(highlightParts("abc", "")).toEqual([{ text: "abc", hit: false }]);
  });
});
