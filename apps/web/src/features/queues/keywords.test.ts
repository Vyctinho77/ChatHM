import { describe, expect, it } from "vitest";
import { parsePhrases, validWildcards } from "./keywords";

describe("parsePhrases", () => {
  it("separa por linha, vírgula e ponto e vírgula", () => {
    expect(parsePhrases("nota fiscal, nfe;danfe\nemit* nota")).toEqual(["nota fiscal", "nfe", "danfe", "emit* nota"]);
  });
  it("tira espaços, linhas vazias e repetidas (sem diferenciar maiúsculas)", () => {
    expect(parsePhrases("  Nota   Fiscal \n\n,, nota fiscal ,NFE,nfe")).toEqual(["Nota Fiscal", "NFE"]);
  });
  it("texto vazio não gera palavras", () => {
    expect(parsePhrases(" \n , ; ")).toEqual([]);
  });
});

describe("validWildcards", () => {
  it("aceita o curinga só no fim de uma palavra com 3+ letras", () => {
    for (const ok of ["emit*", "nota fiscal", "emit* nota", "cancel* nota"]) expect(validWildcards(ok), ok).toBe(true);
    for (const bad of ["a*", "ab*", "*nota", "no*ta", "emit*r"]) expect(validWildcards(bad), bad).toBe(false);
  });
});
