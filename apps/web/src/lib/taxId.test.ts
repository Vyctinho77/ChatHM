import { describe, expect, it } from "vitest";
import { formatTaxId } from "./taxId";

describe("formatTaxId", () => {
  it("coloca a pontuação de CPF e de CNPJ", () => {
    expect(formatTaxId("52998224725")).toBe("529.982.247-25");
    expect(formatTaxId("11222333000181")).toBe("11.222.333/0001-81");
  });
  it("deixa outros valores como vieram", () => {
    expect(formatTaxId("123")).toBe("123");
    expect(formatTaxId(undefined)).toBe("");
  });
});
