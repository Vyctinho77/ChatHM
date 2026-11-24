import { describe, expect, it } from "vitest";
import { ownsGesture } from "./menuGesture";

// Um alvo falso: closest devolve o controle interativo mais próximo (ou null).
const target = (control: { surface: boolean } | null) => ({
  closest: () => (control ? { hasAttribute: (n: string) => n === "data-menu-surface" && control.surface } : null),
});

describe("menu da mensagem em cima de controles", () => {
  it("links, botões e campos comuns mantêm o comportamento do navegador", () => {
    expect(ownsGesture(target({ surface: false }), "a, button")).toBe(true);
  });
  it("a imagem (botão que abre o visualizador) conta como parte da mensagem", () => {
    expect(ownsGesture(target({ surface: true }), "a, button")).toBe(false);
  });
  it("fora de qualquer controle o menu da mensagem vale", () => {
    expect(ownsGesture(target(null), "a, button")).toBe(false);
  });
});
