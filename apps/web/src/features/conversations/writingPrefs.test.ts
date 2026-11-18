import { beforeEach, describe, expect, it } from "vitest";
import { DEFAULT_WRITING_PREFS, getWritingPrefs, ignoreWord, parseWritingPrefs, recordUndo, resetWritingPrefsCache, setWritingPrefs, UNDO_LIMIT, unignoreWord } from "./writingPrefs";

// Sem navegador (ambiente node): um localStorage de mentira.
const store = new Map<string, string>();
beforeEach(() => {
  store.clear();
  (globalThis as unknown as { localStorage: Storage }).localStorage = {
    getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(), key: () => null, length: 0,
  } as Storage;
  (globalThis as unknown as { window: unknown }).window = { addEventListener: () => undefined, removeEventListener: () => undefined };
  resetWritingPrefsCache();
});

describe("preferências de escrita", () => {
  it("padrão: acentos ligados, abreviações desligadas, nada ignorado", () => {
    expect(getWritingPrefs()).toEqual(DEFAULT_WRITING_PREFS);
  });
  it("lê só o que é válido (lixo no armazenamento vira o padrão)", () => {
    expect(parseWritingPrefs(null)).toEqual(DEFAULT_WRITING_PREFS);
    expect(parseWritingPrefs("x")).toEqual(DEFAULT_WRITING_PREFS);
    const p = parseWritingPrefs({ autoAccent: "sim", abbreviations: true, ignored: ["Marcio", "marcio", "", "a b", 7, "x".repeat(60)], undone: { Ana: 1, ruim: -1, mil: 99, "a1": 1 } });
    expect(p).toEqual({ autoAccent: true, abbreviations: true, ignored: ["marcio"], undone: { ana: 1 } });
  });
  it("salva, e o que foi salvo volta", () => {
    setWritingPrefs({ autoAccent: false, abbreviations: true });
    resetWritingPrefsCache();
    expect(getWritingPrefs()).toMatchObject({ autoAccent: false, abbreviations: true });
  });
  it("desfazer a mesma palavra duas vezes passa a ignorá-la; desfazer outra palavra não", () => {
    expect(UNDO_LIMIT).toBe(2);
    recordUndo("Marcio");
    expect(getWritingPrefs().ignored).toEqual([]);
    expect(getWritingPrefs().undone).toEqual({ marcio: 1 });
    recordUndo("nao");
    expect(getWritingPrefs().ignored).toEqual([]);
    recordUndo("marcio");
    expect(getWritingPrefs().ignored).toEqual(["marcio"]);
    expect(getWritingPrefs().undone).toEqual({ nao: 1 });
    recordUndo("marcio"); // já ignorada: nada muda
    expect(getWritingPrefs().ignored).toEqual(["marcio"]);
  });
  it("adiciona e remove palavras ignoradas à mão", () => {
    ignoreWord("  Tecnoprint ");
    ignoreWord("tecnoprint");
    ignoreWord("não é palavra!");
    expect(getWritingPrefs().ignored).toEqual(["tecnoprint"]);
    unignoreWord("TECNOPRINT");
    expect(getWritingPrefs().ignored).toEqual([]);
  });
});
