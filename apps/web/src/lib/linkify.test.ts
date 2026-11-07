import { describe, expect, it } from "vitest";
import { linkify } from "./linkify";

const kinds = (t: string) => linkify(t).filter((p) => p.type !== "text").map((p) => [p.type, p.text]);

describe("linkify", () => {
  it("reconhece e-mails sem a pontuação final", () => {
    expect(kinds("Mande para contato@hmcontabil.com.br, por favor.")).toEqual([["email", "contato@hmcontabil.com.br"]]);
    expect(linkify("a@b.com")[0]).toMatchObject({ type: "email", href: "mailto:a@b.com" });
  });

  it("reconhece sites com e sem protocolo", () => {
    expect(kinds("veja https://www.gov.br/receitafederal/pt-br.")).toEqual([["url", "https://www.gov.br/receitafederal/pt-br"]]);
    expect(kinds("www.hmcontabilidade.com.br e empresa.com.br/nota")).toEqual([["url", "www.hmcontabilidade.com.br"], ["url", "empresa.com.br/nota"]]);
    expect(linkify("www.site.com")[0]).toMatchObject({ href: "https://www.site.com" });
    expect(kinds("(https://exemplo.com/a)")).toEqual([["url", "https://exemplo.com/a"]]);
    expect(kinds("o arquivo nota.pdf e a planilha.xlsx")).toEqual([]);
  });

  it("e-mail não vira também um site", () => {
    expect(kinds("ana@empresa.com.br")).toEqual([["email", "ana@empresa.com.br"]]);
  });

  it("reconhece telefones brasileiros em formatos comuns", () => {
    expect(kinds("Ligue (92) 98800-1111 agora")).toEqual([["phone", "(92) 98800-1111"]]);
    expect(kinds("whats +55 92 98800-1111")).toEqual([["phone", "+55 92 98800-1111"]]);
    expect(kinds("meu número 92988001111")).toEqual([["phone", "92988001111"]]);
    expect(kinds("fixo (92) 3333-4444")).toEqual([["phone", "(92) 3333-4444"]]);
    expect(kinds("só 98800-1111")).toEqual([["phone", "98800-1111"]]);
    expect(linkify("+55 (92) 98800-1111").find((p) => p.type === "phone")).toMatchObject({ digits: "5592988001111" });
  });

  it("não confunde CPF, CNPJ, valores, datas e protocolos com telefone", () => {
    expect(kinds("CPF 123.456.789-01")).toEqual([]);
    expect(kinds("CNPJ 12.345.678/0001-95 e 12345678000195")).toEqual([]);
    expect(kinds("valor R$ 1.234,56 em 05/11/2026")).toEqual([]);
    expect(kinds("protocolo 2026110512345678901234")).toEqual([]);
    expect(kinds("CPF 12345678901")).toEqual([]);
  });

  it("mantém o texto original e devolve só texto quando não há nada", () => {
    expect(linkify("oi, tudo bem?")).toEqual([{ type: "text", text: "oi, tudo bem?" }]);
    const joined = linkify("a x@y.com b (92) 98800-1111 c www.z.com d").map((p) => p.text).join("");
    expect(joined).toBe("a x@y.com b (92) 98800-1111 c www.z.com d");
  });
});
