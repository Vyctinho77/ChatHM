import { beforeEach, describe, expect, it, vi } from "vitest";
import { appearanceVars, applyAppearance, contrast, presets, readableText, sanitize, setAppearance } from "./chatAppearance";

// O ambiente de testes é "node": simula só o que a biblioteca usa do navegador.
function stubBrowser() {
  const props = new Map<string, string>();
  const store = new Map<string, string>();
  vi.stubGlobal("document", {
    documentElement: {
      style: {
        setProperty: (k: string, v: string) => void props.set(k, v),
        removeProperty: (k: string) => void props.delete(k),
        getPropertyValue: (k: string) => props.get(k) ?? "",
      },
    },
  });
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  });
}

describe("aparência do chat", () => {
  beforeEach(stubBrowser);

  it("escolhe texto claro em fundo escuro e escuro em fundo claro", () => {
    expect(readableText("#14532d")).toBe("#ffffff");
    expect(readableText("#ffffff")).toBe("#000000");
    expect(readableText("#cfe8b0")).toBe("#000000");
    expect(readableText("#000000")).toBe("#ffffff");
  });

  it("todo estilo pronto e qualquer cor de fundo mantêm contraste legível (>= 4.5)", () => {
    for (const p of presets) {
      for (const bg of [p.value.out, p.value.in]) {
        if (bg) expect(contrast(bg, readableText(bg))).toBeGreaterThanOrEqual(4.5);
      }
    }
    // Varredura de todo o espaço de cores (passo 17 por canal): o melhor entre preto e branco sempre passa de 4.5.
    const hex = (n: number) => n.toString(16).padStart(2, "0");
    for (let r = 0; r < 256; r += 17) {
      for (let g = 0; g < 256; g += 17) {
        for (let b = 0; b < 256; b += 17) {
          const bg = `#${hex(r)}${hex(g)}${hex(b)}`;
          expect(contrast(bg, readableText(bg)), bg).toBeGreaterThanOrEqual(4.5);
        }
      }
    }
    for (const bg of ["#777777", "#767676", "#8b9d40", "#e67e22", "#2563eb"]) {
      expect(contrast(bg, readableText(bg)), bg).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("descarta valores inválidos em vez de aplicar CSS arbitrário", () => {
    expect(sanitize({ out: "url(javascript:x)", in: "#ZZZ000", wallpaper: 500 })).toEqual({});
    expect(sanitize({ out: "#ABCDEF", wallpaper: 40.4 })).toEqual({ out: "#abcdef", wallpaper: 40 });
    expect(sanitize(null)).toEqual({});
  });

  it("gera as variáveis só do que foi escolhido", () => {
    expect(appearanceVars({})).toEqual({});
    expect(appearanceVars({ wallpaper: 25 })).toEqual({ "--wallpaper-i": "0.25" });
    expect(appearanceVars({ in: "#ffffff" })).toEqual({ "--c-in": "#ffffff", "--c-in-text": "#000000" });
  });

  it("aplica no documento, guarda o cache para antes do primeiro paint e limpa ao voltar ao padrão", () => {
    applyAppearance({ out: "#14532d", wallpaper: 10 });
    const root = document.documentElement.style;
    expect(root.getPropertyValue("--c-out")).toBe("#14532d");
    expect(root.getPropertyValue("--c-out-text")).toBe("#ffffff");
    expect(root.getPropertyValue("--wallpaper-i")).toBe("0.10");
    expect(JSON.parse(localStorage.getItem("chathm:chat-vars") ?? "{}")["--c-out"]).toBe("#14532d");

    applyAppearance({});
    expect(root.getPropertyValue("--c-out")).toBe("");
    expect(localStorage.getItem("chathm:chat-vars")).toBeNull();
  });

  it("o valor do servidor sempre prevalece sobre o cache local", () => {
    document.documentElement.style.setProperty("--c-in", "#123456"); // cache aplicado pelo index.html
    setAppearance({});
    expect(document.documentElement.style.getPropertyValue("--c-in")).toBe("");
  });
});
