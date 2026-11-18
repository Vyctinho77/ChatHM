import { describe, expect, it } from "vitest";
import { applyFix, correctWord, DICTIONARY, fixTail, fixWordEndingAt, isTrigger, setExtendedDictionary, undoFix } from "./autocorrect";

describe("corretor de acentos: palavras", () => {
  it("põe o acento nas palavras comuns", () => {
    for (const [from, to] of [["nao", "não"], ["voce", "você"], ["tambem", "também"], ["orcamento", "orçamento"], ["numero", "número"], ["escritorio", "escritório"], ["sabado", "sábado"], ["cartao", "cartão"], ["ola", "olá"], ["ja", "já"]]) {
      expect(correctWord(from)).toBe(to);
    }
  });
  it("terminações seguras: -ção, -ções, -são, -ível, -ável", () => {
    for (const [from, to] of [["informacao", "informação"], ["declaracoes", "declarações"], ["atencao", "atenção"], ["decisao", "decisão"], ["decisoes", "decisões"], ["questao", "questão"], ["sugestoes", "sugestões"], ["possivel", "possível"], ["disponiveis", "disponíveis"], ["responsavel", "responsável"], ["sao", "são"]]) {
      expect(correctWord(from)).toBe(to);
    }
  });
  it("mantém as maiúsculas", () => {
    expect(correctWord("Voce")).toBe("Você");
    expect(correctWord("NAO")).toBe("NÃO");
    expect(correctWord("Informacao")).toBe("Informação");
    expect(correctWord("nAo")).toBeNull(); // misturado: de propósito
  });
  it("não mexe no que é ambíguo, já acentuado, curto demais ou não é palavra", () => {
    for (const w of ["esta", "e", "pais", "negocio", "para", "por", "marco", "analise", "casa", "mesa", "pessoa", "não", "você", "Cao", "cao", "o", "a", "x1", "nao2", "iPhone", "n"]) {
      expect(correctWord(w)).toBeNull();
    }
  });
  it("o dicionário não corrige palavras que existem sem acento", () => {
    // Palavras comuns da língua que NÃO podem estar na lista curada (a forma sem acento é outra palavra). ("sera", "ta", "ne": gírias de chat, entram de propósito.)
    for (const w of ["esta", "e", "pais", "para", "por", "negocio", "comercio", "inicio", "marco", "publico", "pratica", "analise", "vira", "ha", "ai", "duvida", "debito", "credito", "titulo", "secretaria"]) {
      expect(DICTIONARY.has(w)).toBe(false);
    }
    // E toda entrada tem acento de verdade.
    for (const [from, to] of DICTIONARY) expect(to).not.toBe(from);
  });
});

describe("corretor de acentos: no texto", () => {
  it("corrige a palavra que acabou de ser fechada", () => {
    const text = "Oi, voce ";
    const fix = fixWordEndingAt(text, text.length - 1)!;
    expect(fix).toEqual({ start: 4, end: 8, from: "voce", to: "você" });
    expect(applyFix(text, fix, text.length)).toEqual({ text: "Oi, você ", caret: text.length });
  });
  it("só a palavra inteira: não corrige pedaços", () => {
    expect(fixWordEndingAt("nao", 3)?.to).toBe("não");
    expect(fixWordEndingAt("vienao", 6)).toBeNull();
    expect(fixWordEndingAt("", 0)).toBeNull();
  });
  it("respeita e-mail, link, caminho, menção, tag e variável", () => {
    for (const t of ["fulano@nao", "https://x.com/nao", "www.nao", "c:\\nao", "@voce", "#nao", "{numero", "pre-nao", "2nao"]) {
      expect(fixWordEndingAt(t, t.length)).toBeNull();
    }
  });
  it("a palavra desfeita com Backspace não volta a ser corrigida naquele lugar; as outras, sim", () => {
    expect(fixWordEndingAt("nao", 3, { start: 0, word: "nao" })).toBeNull();
    expect(fixWordEndingAt("voce", 4, { start: 0, word: "nao" })?.to).toBe("você");
    expect(fixWordEndingAt("nao nao", 7, { start: 0, word: "nao" })?.to).toBe("não"); // outra ocorrência
    expect(fixTail("nao", { start: 0, word: "nao" })).toBe("nao");
  });
  it("na hora de enviar corrige a última palavra", () => {
    expect(fixTail("Qual o seu numero")).toBe("Qual o seu número");
    expect(fixTail("tudo bem?")).toBe("tudo bem?");
    expect(fixTail("")).toBe("");
    expect(fixTail("Obrigado, tambem")).toBe("Obrigado, também");
  });
  it("desfazer devolve a palavra original e o cursor", () => {
    const text = "Oi, você ";
    const fix = { start: 4, end: 8, from: "voce", to: "você" };
    // o texto já corrigido tem o mesmo tamanho aqui (4 letras); com palavra de tamanho diferente o cursor acompanha
    expect(undoFix(text, fix, text.length)).toEqual({ text: "Oi, voce ", caret: text.length });
    const longer = { start: 0, end: 10, from: "informacao", to: "informação" };
    expect(applyFix("informacao ", longer, 11)).toEqual({ text: "informação ", caret: 11 });
    const shorter = { start: 0, end: 3, from: "nao", to: "não" };
    expect(undoFix("não ", shorter, 4)).toEqual({ text: "nao ", caret: 4 });
  });
  it("só estes caracteres fecham a palavra", () => {
    for (const c of [" ", ".", ",", "!", "?", ";", ":", ")", "\n"]) expect(isTrigger(c)).toBe(true);
    for (const c of ["a", "-", "@", "/", "1", "ab", ""]) expect(isTrigger(c)).toBe(false);
  });
});

describe("corretor de acentos: dicionário estendido (gerado)", () => {
  it("carrega o arquivo gerado e traz palavras que a lista curada não tem", async () => {
    const { PT_ACCENTS } = await import("./ptAccents.data");
    const pairs = PT_ACCENTS.split(" ").map((p) => p.split(":") as [string, string]);
    expect(pairs.length).toBeGreaterThan(4000);
    setExtendedDictionary(new Map(pairs));
    try {
      for (const [from, to] of [["abencoado", "abençoado"], ["ausencia", "ausência"], ["audiencia", "audiência"], ["incrivel", "incrível"], ["tinhamos", "tínhamos"], ["dolares", "dólares"], ["atras", "atrás"], ["agua", "água"]]) {
        expect(correctWord(from)).toBe(to);
      }
      // nunca as ambíguas nem as que a forma sem acento é de uso comum
      for (const w of ["esta", "pais", "para", "ha", "ai", "sos", "pra", "aqui", "ali", "pelo", "pelos", "tem", "da", "de", "nos", "as"]) expect(correctWord(w)).toBeNull();
    } finally {
      setExtendedDictionary(null);
    }
  });
  it("o arquivo gerado não tem lixo: toda entrada é um par sem acento → com acento", async () => {
    const { PT_ACCENTS } = await import("./ptAccents.data");
    for (const pair of PT_ACCENTS.split(" ")) {
      const [from, to] = pair.split(":");
      expect(from).toMatch(/^[a-z]{2,}$/);
      expect(to).toBeTruthy();
      expect(to!.normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/ç/g, "c")).toBe(from);
      expect(to).not.toBe(from);
    }
  });
  it("siglas de duas letras em maiúsculas não são mexidas", () => {
    expect(correctWord("PE")).toBeNull();
    expect(correctWord("AI")).toBeNull();
    expect(correctWord("NAO")).toBe("NÃO");
  });
});

describe("corretor de acentos: palavras aprendidas", () => {
  it("palavras ignoradas nunca são corrigidas", () => {
    expect(correctWord("voce", { ignored: new Set(["voce"]) })).toBeNull();
    expect(correctWord("Voce", { ignored: new Set(["voce"]) })).toBeNull();
    expect(fixTail("a gente fala voce", undefined, { ignored: new Set(["voce"]) })).toBe("a gente fala voce");
  });
  it("não existe expansão de abreviações: o corretor só acentua", () => {
    for (const w of ["vc", "tb", "tbm", "obg", "blz", "hj", "pfv", "n", "q"]) expect(correctWord(w)).toBeNull();
  });
});
