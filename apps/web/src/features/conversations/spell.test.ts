import { readFileSync } from "node:fs";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { bloomHas, editDistance, findUnknownWords, hashes, isKnownWord, isSpellDictionaryLoaded, setBloom, suggest } from "./spell";
import { setTypoDictionary } from "./typos";
import { setExtendedDictionary, setSuffixRules } from "./autocorrect";
import fixture from "./spell.fixture.json";

beforeAll(async () => {
  setBloom(new Uint8Array(readFileSync(new URL("./ptValid.bin", import.meta.url))));
  const [{ PT_WORDS }, { PT_TYPO_EXCEPTIONS }, { PT_SUFFIX_RULES, PT_SUFFIX_EXCEPTIONS }] = await Promise.all([import("./ptWords.data"), import("./ptTypoExceptions.data"), import("./ptSuffixes.data")]);
  setTypoDictionary(PT_WORDS.split(" "), PT_TYPO_EXCEPTIONS.split(" "));
  setSuffixRules(PT_SUFFIX_RULES, PT_SUFFIX_EXCEPTIONS);
});
afterAll(() => {
  setBloom(null);
  setTypoDictionary(null);
  setSuffixRules(null);
  setExtendedDictionary(null);
});

describe("dicionário de palavras (filtro de Bloom)", () => {
  it("carrega", () => expect(isSpellDictionaryLoaded()).toBe(true));
  it("as funções de hash são as do gerador (valores fixos)", () => {
    expect(hashes("a")).toEqual([3826002220, 177605]); // FNV-1a("a") e djb2-xor("a") | 1
    expect(hashes("você")).toEqual(hashes("você"));
  });
  it("concorda com o gerador (Python) em todos os casos, para palavras existentes e inexistentes", () => {
    expect(fixture.length).toBe(1000);
    const wrong = (fixture as [string, boolean][]).filter(([w, expected]) => bloomHas(w) !== expected);
    expect(wrong.slice(0, 10)).toEqual([]);
  });
  it("palavras comuns existem; palavras inventadas e erros de digitação não", () => {
    for (const w of ["casa", "você", "pendência", "obrigado", "tempo", "atenção", "pessoa", "certificado"]) expect(isKnownWord(w)).toBe(true);
    for (const w of ["asdfgh", "tmepo", "obrigdo", "pessao", "xyzabc"]) expect(isKnownWord(w)).toBe(false);
  });
  it("sem o dicionário carregado, nada é sublinhado", () => {
    setBloom(null);
    try {
      expect(findUnknownWords("asdfgh qwerty", 0)).toEqual([]);
      expect(isKnownWord("asdfgh")).toBe(true);
    } finally {
      setBloom(new Uint8Array(readFileSync(new URL("./ptValid.bin", import.meta.url))));
    }
  });
});

describe("palavras sublinhadas", () => {
  const words = (text: string, caret = 0, ignored: string[] = []) => findUnknownWords(text, caret, new Set(ignored)).map((u) => u.word);
  it("sublinha o que não existe e dá a posição certa", () => {
    const text = "oi asdfgh tudo bem";
    expect(findUnknownWords(text, 0)).toEqual([{ start: 3, end: 9, word: "asdfgh" }]);
  });
  it("erros de digitação e palavras sem acento que não existem sem ele", () => {
    expect(words("o tmepo e a voce")).toEqual(["tmepo", "voce"]);
  });
  it("não sublinha nomes no meio da frase, siglas, risadas, termos do escritório, números, e-mail e link", () => {
    expect(words("falei com Marcio ontem. CNPJ e DARF kkkk rsrs cnpj darf esocial 123abc fulano@asdfgh.com https://asdfgh.com/qwert gov.asdf")).toEqual([]);
  });
  it("no começo da frase a inicial maiúscula é verificada", () => {
    const text = "Tmepo passa. Asdfgh passa";
    expect(words(text, text.length)).toEqual(["Tmepo", "Asdfgh"]);
  });
  it("a palavra em que o cursor está ainda está sendo digitada: não é sublinhada", () => {
    const text = "oi tmepo";
    expect(words(text, text.length)).toEqual([]);
    expect(words(text, 0)).toEqual(["tmepo"]);
    expect(words("oi tmepo e", 6)).toEqual([]); // cursor no meio da palavra
  });
  it("palavras ignoradas e de 1 ou 2 letras não são sublinhadas", () => {
    expect(words("oi asdfgh ab", 0, ["asdfgh"])).toEqual([]);
  });
});

describe("sugestões", () => {
  it("a primeira é a correção que o corretor faria (acento ou erro de digitação)", () => {
    expect(suggest("tmepo")[0]).toBe("tempo");
    expect(suggest("pendencia")[0]).toBe("pendência");
    expect(suggest("obrigdo")[0]).toBe("obrigado");
    expect(suggest("Tmepo")[0]).toBe("Tempo");
  });
  it("traz outras palavras próximas, sem repetir nem devolver a própria palavra", () => {
    const s = suggest("pessao");
    expect(s).toContain("pessoa");
    expect(new Set(s).size).toBe(s.length);
    expect(s).not.toContain("pessao");
    expect(s.length).toBeLessThanOrEqual(5);
  });
  it("dois enganos também têm sugestão", () => {
    expect(suggest("tmpeo").length).toBeGreaterThan(0);
  });
  it("sem nada parecido, sem sugestão", () => {
    expect(suggest("qzxwvk")).toEqual([]);
  });
  it("mantém a capitalização", () => {
    for (const w of suggest("Pessao")) expect(w[0]).toBe(w[0]!.toUpperCase());
  });
});

describe("distância de edição", () => {
  it("conta inserir, apagar, trocar uma letra e trocar duas vizinhas de lugar como 1", () => {
    expect(editDistance("casa", "casa", 2)).toBe(0);
    expect(editDistance("tmepo", "tempo", 2)).toBe(1); // vizinhas trocadas
    expect(editDistance("obrigdo", "obrigado", 2)).toBe(1); // faltou
    expect(editDistance("obrigaddo", "obrigado", 2)).toBe(1); // sobrou
    expect(editDistance("obrigafo", "obrigado", 2)).toBe(1); // trocada
    expect(editDistance("pessao", "pessoa", 2)).toBe(1);
    expect(editDistance("tmpeo", "tempo", 2)).toBe(2);
  });
  it("devolve max+1 quando passa do limite, sem calcular tudo", () => {
    expect(editDistance("casa", "tempo", 1)).toBe(2);
    expect(editDistance("abc", "abcdefgh", 2)).toBe(3); // tamanhos muito diferentes
    expect(editDistance("qzxwvk", "tempo", 2)).toBe(3);
  });
});

describe("desempenho (a tela não pode travar)", () => {
  it("sugestões saem em milissegundos mesmo para palavras longas", () => {
    for (const w of ["asdfgh", "qzxwvkpd", "internacionalizacao", "anticonstitucionalissimamente", "xyzzyqrstuvwxyz"]) {
      const t0 = performance.now();
      suggest(w);
      expect(performance.now() - t0, w).toBeLessThan(120);
    }
  });
  it("achar as palavras inexistentes de uma mensagem grande é rápido", () => {
    const text = Array.from({ length: 80 }, (_, i) => `Mensagem de teste ${i} com qzxwvk e asdfgh no meio do texto digitado aqui. `).join("");
    const t0 = performance.now();
    for (let i = 0; i < 10; i++) findUnknownWords(text, 0);
    expect((performance.now() - t0) / 10).toBeLessThan(30);
  });
});
