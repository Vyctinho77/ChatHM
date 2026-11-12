import { describe, expect, it } from "vitest";
import { classify, normalize, normalizeKeyword, tokens, validWildcards, type DemoKeyword } from "./classify";

const kw = (segmentKey: string, phrase: string, priority = 0): DemoKeyword => ({
  segmentId: "id-" + segmentKey,
  segmentKey,
  phrase,
  normalized: normalize(phrase),
  priority,
});

// Subconjunto das palavras padrão do backend (backend/internal/segments/defaults.go).
const defaults: DemoKeyword[] = [
  ...["orçamento", "contratar", "novo cliente", "abrir empresa", "quanto custa"].map((p) => kw("clientes_novos", p)),
  ...["folha", "folha de pagamento", "férias", "rescisão", "13º salário"].map((p) => kw("folha", p)),
  ...["emitir nota", "emissão de nota", "nota fiscal", "nfe"].map((p) => kw("notas", p)),
];

describe("tokens (mesmas regras do backend)", () => {
  it("normaliza caixa, acentos, ligação e plural", () => {
    expect(tokens("Quero EMITIR uma Nota!")).toEqual(["quero", "emitir", "nota"]);
    expect(tokens("Preciso de FÉRIAS de funcionário")).toEqual(["preciso", "feria", "funcionario"]);
    expect(tokens("Emissões de notas fiscais")).toEqual(["emissao", "nota", "fiscal"]);
    expect(tokens("13º salário, por favor")).toEqual(["13o", "salario", "favor"]);
    expect(tokens("valores mensais, vezes, imagens")).toEqual(["valor", "mensal", "vez", "imagem"]);
    expect(tokens("classe")).toEqual(["classe"]);
    expect(tokens("de a o")).toEqual([]);
  });
});

describe("classify (critérios de aceite do blueprint)", () => {
  const cases: [string, string | undefined, string][] = [
    ["quero emitir uma nota", "notas", "keyword"],
    ["preciso calcular a folha", "folha", "keyword"],
    ["quero contratar", "clientes_novos", "keyword"],
    ["preciso de férias de funcionário", "folha", "keyword"],
    ["quero orçamento", "clientes_novos", "keyword"],
    ["bom dia", undefined, "none"],
    ["quero orçamento da folha de pagamento", undefined, "conflict"],
    ["o folheto ficou pronto", undefined, "none"],
    ["preciso das notas fiscais de outubro", "notas", "keyword"],
  ];
  it.each(cases)("%s", (text, key, reason) => {
    const r = classify(text, defaults);
    expect(r.segmentKey).toBe(key);
    expect(r.reason).toBe(reason);
  });

  it("prioridade maior resolve o conflito", () => {
    const kws = [kw("folha", "folha", 10), kw("clientes_novos", "orçamento")];
    const r = classify("orçamento da folha", kws);
    expect(r.reason).toBe("keyword");
    expect(r.segmentKey).toBe("folha");
    expect(r.matches[0]?.priority).toBe(10);
  });

  it("palavra-chave vazia nunca combina", () => {
    expect(classify("qualquer coisa", [{ ...kw("x", "a"), normalized: "" }]).reason).toBe("none");
  });
});

describe("curinga no fim da palavra (mesmas regras do backend)", () => {
  const kws = [
    { segmentId: "n", segmentKey: "notas", phrase: "emit* nota", normalized: normalizeKeyword("emit* nota"), priority: 50 },
    { segmentId: "f", segmentKey: "folha", phrase: "folha", normalized: normalizeKeyword("folha"), priority: 50 },
  ];
  it("normalizeKeyword mantém o prefixo como foi escrito", () => {
    expect(normalizeKeyword("emit* nota")).toBe("emit* nota");
    expect(normalizeKeyword("EMISS* de Notas")).toBe("emiss* nota");
    expect(normalizeKeyword("ab*")).toBe("ab"); // curto demais: sem curinga
    expect(normalizeKeyword("Férias")).toBe("feria");
  });
  it("valida onde o * pode ficar", () => {
    for (const ok of ["emit*", "nota fiscal", "emit* nota"]) expect(validWildcards(ok), ok).toBe(true);
    for (const bad of ["a*", "ab*", "*nota", "no*ta", "emit*r"]) expect(validWildcards(bad), bad).toBe(false);
  });
  it("pega as formas do verbo, mas só no começo da palavra e com as palavras juntas e em ordem", () => {
    for (const t of ["Preciso emitir uma nota", "emita a nota por favor", "já emitiu nota?"]) {
      expect(classify(t, kws).segmentKey, t).toBe("notas");
    }
    for (const t of ["remitir nota", "emitir folha", "nota emitir", "emi nota"]) {
      expect(classify(t, kws).segmentKey, t).not.toBe("notas");
    }
  });
});
