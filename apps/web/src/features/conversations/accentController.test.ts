import { describe, expect, it } from "vitest";
import { AccentController, type InputInfo } from "./accentController";

const typed = (data: string, caret: number, extra: Partial<InputInfo> = {}): InputInfo => ({ inputType: "insertText", data, isComposing: false, selectionEnd: caret, ...extra });
const make = (o: { enabled?: boolean; abbreviations?: boolean; ignored?: string[] } = {}, undone: string[] = []) =>
  new AccentController(() => ({ enabled: o.enabled ?? true, abbreviations: o.abbreviations, ignored: new Set(o.ignored ?? []) }), (w) => undone.push(w));

describe("motor do corretor", () => {
  it("fecha a palavra com espaço e corrige, devolvendo o cursor no lugar certo", () => {
    const c = make();
    expect(c.input("Oi, voce ", 9, typed(" ", 9))).toEqual({ value: "Oi, você ", caret: 9, fixed: true });
    expect(c.input("informacao.", 11, typed(".", 11))).toEqual({ value: "informação.", caret: 11, fixed: true });
  });
  it("corrige no meio do texto sem levar o cursor para o fim", () => {
    const c = make();
    const r = c.input("ola tambem  tudo bem", 11, typed(" ", 11)); // tambem + espaço digitados no meio
    expect(r).toEqual({ value: "ola também  tudo bem", caret: 11, fixed: true });
  });
  it("não corrige enquanto a palavra está aberta, durante composição, com seleção ou desligado", () => {
    expect(make().input("voce", 4, typed("e", 4))).toMatchObject({ fixed: false, value: "voce" });
    expect(make().input("voce ", 5, typed(" ", 5, { isComposing: true })).fixed).toBe(false);
    expect(make().input("voce ", 5, typed(" ", 5, { selectionEnd: 7 })).fixed).toBe(false);
    expect(make().input("voce ", 5, { inputType: "insertFromPaste", data: null, isComposing: false, selectionEnd: 5 }).fixed).toBe(false);
    expect(make({ enabled: false }).input("voce ", 5, typed(" ", 5)).fixed).toBe(false);
  });
  it("Enter com Shift (quebra de linha) também fecha a palavra", () => {
    const c = make();
    expect(c.input("nao\n", 4, { inputType: "insertLineBreak", data: null, isComposing: false, selectionEnd: 4 })).toEqual({ value: "não\n", caret: 4, fixed: true });
  });
  it("Backspace logo depois desfaz, avisa e não corrige essa palavra de novo ali", () => {
    const undone: string[] = [];
    const c = make({}, undone);
    c.input("nao ", 4, typed(" ", 4));
    const back = c.backspace("não ", 4, 4)!;
    expect(back).toEqual({ value: "nao ", caret: 4 });
    expect(undone).toEqual(["nao"]);
    expect(c.input("nao e ", 6, typed(" ", 6)).fixed).toBe(false); // "e" não é palavra do dicionário
    expect(c.input("nao ", 4, typed(" ", 4)).fixed).toBe(false); // mesma palavra, mesmo lugar: respeita o desfazer
    expect(c.input("nao nao ", 8, typed(" ", 8)).fixed).toBe(true); // outra ocorrência: corrige
  });
  it("Backspace só vale logo depois da correção, no mesmo lugar", () => {
    const c = make();
    c.input("nao ", 4, typed(" ", 4));
    expect(c.backspace("não ", 2, 2)).toBeNull(); // cursor em outro lugar
    expect(c.backspace("não ", 0, 4)).toBeNull(); // seleção
    c.input("não x", 5, typed("x", 5)); // digitou outra coisa: a última correção fica para trás
    expect(c.backspace("não x", 5, 5)).toBeNull();
  });
  it("no envio corrige a última palavra; desligado, deixa como está", () => {
    expect(make().finish("Qual o seu numero")).toBe("Qual o seu número");
    expect(make({ enabled: false }).finish("Qual o seu numero")).toBe("Qual o seu numero");
  });
  it("abreviações e palavras ignoradas valem no motor", () => {
    expect(make({ abbreviations: true }).input("vc ", 3, typed(" ", 3)).value).toBe("você ");
    expect(make().input("vc ", 3, typed(" ", 3)).fixed).toBe(false);
    expect(make({ ignored: ["voce"] }).input("voce ", 5, typed(" ", 5)).fixed).toBe(false);
  });
});
