import { describe, expect, it } from "vitest";
import { findAccessKeys, formatAccessKey, isValidAccessKey } from "./pdfAccessKey";

/** Monta uma chave válida calculando o dígito verificador por conta própria (independente do código testado). */
function makeKey(prefix43: string): string {
  const w = [2, 3, 4, 5, 6, 7, 8, 9];
  let sum = 0;
  prefix43
    .split("")
    .reverse()
    .forEach((d, i) => {
      sum += Number(d) * w[i % 8]!;
    });
  const r = sum % 11;
  return prefix43 + String(r < 2 ? 0 : 11 - r);
}

// UF 35, AAMM 2611, CNPJ, modelo 55, série 001, número, tipo emissão 1, código numérico.
const BASE = "3526111234567800019555001000000123" + "1" + "00000045";
const KEY = makeKey(BASE.padEnd(43, "0").slice(0, 43));

describe("isValidAccessKey", () => {
  it("aceita uma chave com UF, modelo e dígito verificador corretos", () => {
    expect(KEY).toHaveLength(44);
    expect(isValidAccessKey(KEY)).toBe(true);
  });
  it("recusa dígito verificador errado, UF inexistente, modelo desconhecido e tamanho errado", () => {
    const wrongDv = KEY.slice(0, 43) + String((Number(KEY[43]) + 1) % 10);
    expect(isValidAccessKey(wrongDv)).toBe(false);
    expect(isValidAccessKey(makeKey("99" + KEY.slice(2, 43)))).toBe(false);
    expect(isValidAccessKey(makeKey(KEY.slice(0, 20) + "99" + KEY.slice(22, 43)))).toBe(false);
    expect(isValidAccessKey(KEY.slice(1))).toBe(false);
  });
});

describe("findAccessKeys", () => {
  it("acha a chave impressa em grupos de 4 no meio do texto", () => {
    const text = `CHAVE DE ACESSO ${formatAccessKey(KEY)} Consulta de autenticidade`;
    expect(findAccessKeys(text)).toEqual([KEY]);
  });
  it("acha a chave sem espaços e sem repetir", () => {
    expect(findAccessKeys(`${KEY} outro ${KEY}`)).toEqual([KEY]);
  });
  it("ignora números longos que não são chave", () => {
    expect(findAccessKeys("protocolo 135260000012345 cnpj 12345678000195")).toEqual([]);
    expect(findAccessKeys("1".repeat(44))).toEqual([]);
  });
  it("acha a chave mesmo colada a outro número", () => {
    expect(findAccessKeys(`${KEY} 123456`)).toEqual([KEY]);
  });
});
