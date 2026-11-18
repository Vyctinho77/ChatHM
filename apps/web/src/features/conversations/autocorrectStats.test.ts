import { beforeEach, describe, expect, it } from "vitest";
import { flushStats, pendingStats, recordApplied, recordUndone, resetStats, type StatsEntry } from "./autocorrectStats";
import { AccentController } from "./accentController";
import type { Fix } from "./autocorrect";

const fix = (from: string, to: string, layer: Fix["layer"] = "curated"): Fix => ({ start: 0, end: from.length, from, to, layer });
beforeEach(resetStats);

describe("medição do corretor", () => {
  it("conta aplicadas e desfeitas por camada e por par de palavras, em minúsculas", () => {
    recordApplied(fix("Voce", "Você"));
    recordApplied(fix("voce", "você"));
    recordUndone(fix("voce", "você"));
    recordApplied(fix("tmepo", "tempo", "typo_tr"));
    expect(pendingStats().sort((a, b) => a.from.localeCompare(b.from))).toEqual([
      { layer: "typo_tr", from: "tmepo", to: "tempo", applied: 1, undone: 0 },
      { layer: "curated", from: "voce", to: "você", applied: 2, undone: 1 },
    ]);
  });
  it("só guarda palavras soltas: letras, até 40", () => {
    recordApplied(fix("voce2", "você"));
    recordApplied(fix("oi tudo", "oi"));
    recordApplied(fix("a".repeat(41), "b"));
    recordApplied(fix("", "x"));
    expect(pendingStats()).toEqual([]);
  });
  it("o que é guardado tem só 5 campos: nada de texto, frase, conversa nem quem digitou", () => {
    recordApplied(fix("voce", "você"));
    expect(Object.keys(pendingStats()[0]!).sort()).toEqual(["applied", "from", "layer", "to", "undone"]);
  });
  it("não cresce sem limite quando está sem rede", () => {
    for (let i = 0; i < 700; i++) recordApplied(fix("a" + "b".repeat(i % 30) + "c".repeat(Math.floor(i / 30) % 9), "x" + i.toString(36).replace(/\d/g, "z")));
    expect(pendingStats().length).toBeLessThanOrEqual(500);
  });
});

describe("envio da medição", () => {
  it("envia em lotes de até 200 e esvazia o que foi enviado", async () => {
    for (let i = 0; i < 250; i++) recordApplied(fix("w" + String.fromCharCode(97 + (i % 26)) + String.fromCharCode(97 + (Math.floor(i / 26) % 26)), "t" + String.fromCharCode(97 + (i % 26))));
    const total = pendingStats().length;
    const sent: StatsEntry[][] = [];
    await flushStats(async (b) => void sent.push(b));
    expect(sent).toHaveLength(1);
    expect(sent[0]!.length).toBeLessThanOrEqual(200);
    expect(pendingStats().length).toBe(total - sent[0]!.length);
    await flushStats(async (b) => void sent.push(b));
    await flushStats(async (b) => void sent.push(b));
    expect(pendingStats()).toEqual([]);
    expect(sent.flat().length).toBe(total);
  });
  it("se o envio falhar, nada se perde: volta para a fila, somando com o que chegou depois", async () => {
    recordApplied(fix("voce", "você"));
    await flushStats(async () => { throw new Error("sem rede"); });
    expect(pendingStats()).toEqual([{ layer: "curated", from: "voce", to: "você", applied: 1, undone: 0 }]);
    // enquanto falhava, a pessoa continuou digitando
    const sent: StatsEntry[][] = [];
    const slow = flushStats(async (b) => { recordApplied(fix("voce", "você")); sent.push(b); });
    await slow;
    expect(sent[0]).toEqual([{ layer: "curated", from: "voce", to: "você", applied: 1, undone: 0 }]);
    expect(pendingStats()).toEqual([{ layer: "curated", from: "voce", to: "você", applied: 1, undone: 0 }]);
  });
  it("não envia nada quando não há nada", async () => {
    let calls = 0;
    await flushStats(async () => void calls++);
    expect(calls).toBe(0);
  });
  it("dois envios ao mesmo tempo não duplicam", async () => {
    recordApplied(fix("voce", "você"));
    let release: () => void = () => undefined;
    const gate = new Promise<void>((r) => (release = r));
    const sent: StatsEntry[][] = [];
    const first = flushStats(async (b) => { await gate; sent.push(b); });
    await flushStats(async (b) => void sent.push(b)); // enquanto o primeiro está em andamento: ignora
    release();
    await first;
    expect(sent).toHaveLength(1);
  });
});

describe("o motor conta o que faz", () => {
  const typed = (data: string, caret: number) => ({ inputType: "insertText", data, isComposing: false, selectionEnd: caret });
  it("conta a correção ao fechar a palavra e a desfeita no Backspace", () => {
    const c = new AccentController(() => ({}));
    c.input("nao ", 4, typed(" ", 4));
    c.backspace("não ", 4, 4);
    expect(pendingStats()).toEqual([{ layer: "curated", from: "nao", to: "não", applied: 1, undone: 1 }]);
  });
  it("conta a correção feita no envio (última palavra)", () => {
    const c = new AccentController(() => ({}));
    expect(c.finish("seu numero")).toBe("seu número");
    expect(pendingStats()).toEqual([{ layer: "curated", from: "numero", to: "número", applied: 1, undone: 0 }]);
  });
  it("não conta quando não corrigiu", () => {
    const c = new AccentController(() => ({}));
    c.input("casa ", 5, typed(" ", 5));
    c.finish("casa");
    expect(pendingStats()).toEqual([]);
  });
});
