import { beforeEach, describe, expect, it } from "vitest";
import { getWritingPrefs, IGNORE_DAYS, ignoredWords, parseWritingPrefs, recordUndo, resetWritingPrefsCache, UNDO_LIMIT } from "./writingPrefs";

// Sem navegador (ambiente node): um localStorage de mentira.
const store = new Map<string, string>();
beforeEach(() => {
  store.clear();
  (globalThis as unknown as { localStorage: Storage }).localStorage = {
    getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(), key: () => null, length: 0,
  } as Storage;
  resetWritingPrefsCache();
});

const DAY = 24 * 3600 * 1000;
const NOW = Date.parse("2026-11-18T12:00:00Z");

describe("o que o corretor aprende", () => {
  it("começa sem nada aprendido", () => {
    expect(getWritingPrefs()).toEqual({ ignored: {}, undone: {} });
    expect(ignoredWords().size).toBe(0);
  });
  it("lê só o que é válido (lixo no armazenamento é descartado)", () => {
    expect(parseWritingPrefs(null, NOW)).toEqual({ ignored: {}, undone: {} });
    expect(parseWritingPrefs("x", NOW)).toEqual({ ignored: {}, undone: {} });
    expect(parseWritingPrefs({ ignored: ["marcio"], undone: [1] }, NOW)).toEqual({ ignored: {}, undone: {} }); // formato antigo/errado
    const p = parseWritingPrefs({ ignored: { Marcio: NOW - DAY, "a b": NOW, x1: NOW, ruim: "ontem", futuro: NOW + 30 * DAY }, undone: { Ana: 1, mil: 9, neg: -1, marcio: 1 } }, NOW);
    expect(p).toEqual({ ignored: { marcio: NOW - DAY }, undone: { ana: 1 } });
  });
  it("o que foi aprendido expira depois de IGNORE_DAYS dias", () => {
    const raw = { ignored: { velha: NOW - (IGNORE_DAYS + 1) * DAY, recente: NOW - (IGNORE_DAYS - 1) * DAY }, undone: {} };
    expect(Object.keys(parseWritingPrefs(raw, NOW).ignored)).toEqual(["recente"]);
  });
  it("desfazer a mesma palavra duas vezes passa a ignorá-la; desfazer outra palavra não", () => {
    expect(UNDO_LIMIT).toBe(2);
    recordUndo("Marcio", NOW);
    expect(getWritingPrefs().ignored).toEqual({});
    expect(getWritingPrefs().undone).toEqual({ marcio: 1 });
    recordUndo("nao", NOW);
    expect(getWritingPrefs().ignored).toEqual({});
    recordUndo("marcio", NOW);
    expect(getWritingPrefs().ignored).toEqual({ marcio: NOW });
    expect(getWritingPrefs().undone).toEqual({ nao: 1 });
    recordUndo("marcio", NOW + 1000); // já ignorada: nada muda
    expect(getWritingPrefs().ignored).toEqual({ marcio: NOW });
    expect(ignoredWords(NOW).has("marcio")).toBe(true);
    expect(ignoredWords(NOW + (IGNORE_DAYS + 1) * DAY).has("marcio")).toBe(false); // expirou
  });
  it("o aprendizado fica salvo para a próxima vez", () => {
    recordUndo("tecnoprint", NOW);
    recordUndo("tecnoprint", NOW);
    resetWritingPrefsCache();
    expect(Object.keys(getWritingPrefs().ignored)).toEqual(["tecnoprint"]);
  });
  it("ignora o que não é palavra", () => {
    recordUndo("não é palavra!", NOW);
    recordUndo("", NOW);
    expect(getWritingPrefs()).toEqual({ ignored: {}, undone: {} });
  });
});
