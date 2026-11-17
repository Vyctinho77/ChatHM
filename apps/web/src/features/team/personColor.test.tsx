import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AssigneeChip } from "@/features/conversations/AssigneeChip";
import { PersonName } from "./PersonName";
import { COLOR_KEYS, autoColorKey, colorInkVar, colorVar, isColorKey, resolveColorKey } from "./personColor";

const withPeople = (ui: React.ReactElement, people: { id: string; color: string }[] = []) => {
  const qc = new QueryClient();
  qc.setQueryData(["people"], people);
  return renderToStaticMarkup(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
};

describe("cor do nome das pessoas", () => {
  it("a automática é sempre a mesma para a mesma pessoa e sai da paleta", () => {
    for (const id of ["a", "b", "c", "0f2c1e6a-7b53-4b4e-9c0a-111122223333"]) {
      expect(autoColorKey(id)).toBe(autoColorKey(id));
      expect(COLOR_KEYS).toContain(autoColorKey(id));
    }
    // Com muita gente, a paleta toda aparece (a distribuição não colapsa numa cor só).
    const used = new Set(Array.from({ length: 200 }, (_, i) => autoColorKey(`pessoa-${i}`)));
    expect(used.size).toBeGreaterThanOrEqual(10);
  });

  it("a cor escolhida vale; vazia ou fora da paleta cai na automática", () => {
    expect(resolveColorKey("x", "azul")).toBe("azul");
    expect(resolveColorKey("x", "")).toBe(autoColorKey("x"));
    expect(resolveColorKey("x", "fucsia")).toBe(autoColorKey("x"));
    expect(resolveColorKey("x", undefined)).toBe(autoColorKey("x"));
    expect(isColorKey("verde")).toBe(true);
    expect(isColorKey("#fff")).toBe(false);
    expect(colorVar("azul")).toBe("var(--pc-azul)");
    expect(colorInkVar("azul")).toBe("var(--pc-azul-ink)");
  });

  it("o nome sai colorido, só o texto: sem fundo nem borda (sem balão)", () => {
    const html = withPeople(<PersonName id="p1">Heliton</PersonName>, [{ id: "p1", color: "roxo" }]);
    expect(html).toContain("Heliton");
    expect(html).toContain("color:var(--pc-roxo)");
    expect(html).not.toMatch(/\bbg-|rounded|border|\bring-/);
  });

  it("sem cor escolhida usa a automática; no balão recebido usa o tom de tinta", () => {
    expect(withPeople(<PersonName id="p2">Ana</PersonName>)).toContain(`color:var(--pc-${autoColorKey("p2")})`);
    expect(withPeople(<PersonName id="p1" ink>Heliton</PersonName>, [{ id: "p1", color: "azul" }])).toContain("color:var(--pc-azul-ink)");
  });

  it("quem atende aparece como texto colorido, sem o balão de antes", () => {
    const html = withPeople(<AssigneeChip id="p1" name="Heliton Souza" mine={false} />, [{ id: "p1", color: "laranja" }]);
    expect(html).toContain(">Heliton</span>"); // o texto visível é só o primeiro nome; o nome completo fica na dica
    expect(html).toContain('title="Atendendo: Heliton Souza"');
    expect(html).toContain("color:var(--pc-laranja)");
    expect(html).not.toMatch(/\bbg-|rounded-full/);
    expect(withPeople(<AssigneeChip id="p1" name="Heliton Souza" mine />, [{ id: "p1", color: "laranja" }])).toContain("Você");
  });
});
