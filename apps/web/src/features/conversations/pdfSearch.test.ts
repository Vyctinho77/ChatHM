import { describe, expect, it } from "vitest";
import { findMatches, fold } from "./pdfSearch";

describe("fold", () => {
  it("tira acento e caixa sem mudar o tamanho do texto", () => {
    expect(fold("Inscrição ESTADUAL")).toBe("inscricao estadual");
    expect(fold("Inscrição ESTADUAL")).toHaveLength("Inscrição ESTADUAL".length);
  });
  it("trata espaço não separável como espaço", () => {
    expect(fold("a b")).toBe("a b");
  });
});

describe("findMatches", () => {
  it("acha ignorando acento e maiúscula, com a posição dentro do item", () => {
    const m = findMatches(["NATUREZA DE OPERAÇÃO", "VENDA"], "operacao");
    expect(m).toEqual([{ pieces: [{ item: 0, start: 12, end: 20 }] }]);
  });

  it("acha várias ocorrências, também no mesmo item", () => {
    const m = findMatches(["aba aba", "xaba"], "aba");
    expect(m.map((x) => x.pieces)).toEqual([
      [{ item: 0, start: 0, end: 3 }],
      [{ item: 0, start: 4, end: 7 }],
      [{ item: 1, start: 1, end: 4 }],
    ]);
  });

  it("uma ocorrência partida entre itens vem em vários trechos", () => {
    const m = findMatches(["INSCRI", "ÇÃO ESTADUAL"], "inscrição estadual");
    expect(m).toHaveLength(1);
    expect(m[0]!.pieces).toEqual([
      { item: 0, start: 0, end: 6 },
      { item: 1, start: 0, end: 12 },
    ]);
  });

  it("consulta vazia ou só com espaços não acha nada", () => {
    expect(findMatches(["abc"], "")).toEqual([]);
    expect(findMatches(["abc"], "  ")).toEqual([]);
  });

  it("não acha o que não existe", () => {
    expect(findMatches(["abc"], "xyz")).toEqual([]);
  });
});
