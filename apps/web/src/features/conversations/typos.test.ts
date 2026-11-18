import { beforeAll, afterAll, describe, expect, it } from "vitest";
import { correctTypo, fixFolded, fold, isTypoDictionaryLoaded, setTypoDictionary } from "./typos";
import { fixTail, fixWordEndingAt, setExtendedDictionary } from "./autocorrect";
import fixture from "./typos.fixture.json";

beforeAll(async () => {
  const [{ PT_WORDS }, { PT_TYPO_EXCEPTIONS }] = await Promise.all([import("./ptWords.data"), import("./ptTypoExceptions.data")]);
  setTypoDictionary(PT_WORDS.split(" "), PT_TYPO_EXCEPTIONS.split(" "));
});
afterAll(() => setTypoDictionary(null));

describe("erros de digitação: o mesmo algoritmo do gerador (Python)", () => {
  it("o dicionário carrega", () => expect(isTypoDictionaryLoaded()).toBe(true));
  it("concorda com o gerador nos casos de teste (typos.fixture.json)", () => {
    expect(fixture.length).toBeGreaterThan(1000);
    const wrong = (fixture as [string, string | null][]).filter(([typed, expected]) => fixFolded(typed) !== expected);
    expect(wrong.slice(0, 10)).toEqual([]);
    const fixed = (fixture as [string, string | null][]).filter(([, e]) => e).length;
    expect(fixed).toBeGreaterThan(300); // o fixture exercita as correções de verdade
  });
});

describe("erros de digitação: exemplos", () => {
  it("letras trocadas, letra faltando, letra repetida e tecla vizinha", () => {
    expect(fixFolded("tmepo")).toBe("tempo");
    expect(fixFolded("obrigdo")).toBe("obrigado");
    expect(fixFolded("obrigaddo")).toBe("obrigado");
    expect(fixFolded("obrigafo")).toBe("obrigado"); // f fica ao lado do d
    expect(fixFolded("obrigapo")).toBeNull(); // p não fica ao lado do d: troca qualquer não é corrigida
    expect(fixFolded("pssoa")).toBe("pessoa");
  });
  it("traz o acento da palavra certa", () => {
    expect(fixFolded("vocce")).toBe("você");
    expect(fixFolded("quadno")).toBe("quando");
  });
  it("não mexe em palavras válidas, siglas, risadas, nomes de uso do escritório nem palavras curtas", () => {
    for (const w of ["casa", "voce", "tempo", "kkkkk", "rsrsrs", "hahaha", "cnpj", "darf", "esocial", "presumido", "whatsapp", "zap", "xyz", "abc"]) expect(fixFolded(w)).toBeNull();
  });
  it("não mexe quando dois candidatos empatam em frequência (ambíguo)", () => {
    // "casp": casa, caso, capa, cada... nenhum domina
    expect(fixFolded("casp")).toBeNull();
  });
});

describe("erros de digitação: capitalização e contexto", () => {
  it("mantém a capitalização e só corrige inicial maiúscula no começo da frase", () => {
    expect(correctTypo("tmepo", { sentenceStart: false })).toBe("tempo");
    expect(correctTypo("Tmepo", { sentenceStart: true })).toBe("Tempo");
    expect(correctTypo("Tmepo", { sentenceStart: false })).toBeNull(); // no meio: costuma ser nome
    expect(correctTypo("TMEPO", { sentenceStart: true })).toBeNull(); // maiúsculas: sigla
    expect(correctTypo("tmEpo", { sentenceStart: true })).toBeNull();
    expect(correctTypo("tempó", { sentenceStart: true })).toBeNull(); // já tem acento
  });
  it("sem o dicionário carregado, não faz nada", () => {
    setTypoDictionary(null);
    expect(fixFolded("tmepo")).toBeNull();
    return import("./ptWords.data").then(async ({ PT_WORDS }) => {
      const { PT_TYPO_EXCEPTIONS } = await import("./ptTypoExceptions.data");
      setTypoDictionary(PT_WORDS.split(" "), PT_TYPO_EXCEPTIONS.split(" "));
    });
  });
  it("fold tira acentos e cedilha", () => expect(fold("ação")).toBe("acao"));
});

describe("erros de digitação: no texto (junto com os acentos)", () => {
  it("corrige ao fechar a palavra, com o mesmo Fix do Backspace", () => {
    setExtendedDictionary(new Map());
    const text = "Qual o seu tmepo ";
    const fix = fixWordEndingAt(text, text.length - 1)!;
    expect(fix).toEqual({ start: 11, end: 16, from: "tmepo", to: "tempo" });
  });
  it("no envio corrige a última palavra", () => {
    expect(fixTail("Quanto tmepo")).toBe("Quanto tempo");
    expect(fixTail("Quanto tempo")).toBe("Quanto tempo");
  });
  it("no começo da frase aceita inicial maiúscula; no meio, não", () => {
    expect(fixTail("Tmepo")).toBe("Tempo");
    expect(fixTail("Vamos. Tmepo")).toBe("Vamos. Tempo");
    expect(fixTail("Falou com Tmepo")).toBe("Falou com Tmepo");
  });
  it("palavras que a pessoa mandou não corrigir (desfez duas vezes) ficam como estão", () => {
    expect(fixTail("tmepo", undefined, { ignored: new Set(["tmepo"]) })).toBe("tmepo");
  });
  it("e-mail, link, menção e número não são mexidos", () => {
    for (const t of ["fulano@tmepo", "https://x.com/tmepo", "@tmepo", "#tmepo", "tmepo2"]) expect(fixTail(t)).toBe(t);
  });
});
