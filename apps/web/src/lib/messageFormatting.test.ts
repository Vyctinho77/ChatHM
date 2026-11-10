import { describe, expect, it } from "vitest";
import { formatMessage } from "./messageFormatting";

const plain = (text: string) => ({ type: "text", text });

describe("WhatsApp message formatting", () => {
  it("formats bold and italic, alone or together, preserving punctuation", () => {
    expect(formatMessage("Olá, *José*! _Tudo bem?_" )).toEqual([
      plain("Olá, "), { type: "bold", children: [plain("José")] }, plain("! "),
      { type: "italic", children: [plain("Tudo bem?")] },
    ]);
    expect(formatMessage("*_Os dois_*" )).toEqual([{ type: "bold", children: [{ type: "italic", children: [plain("Os dois")] }] }]);
    expect(formatMessage("_Texto *importante* aqui_" )).toEqual([{ type: "italic", children: [plain("Texto "), { type: "bold", children: [plain("importante")] }, plain(" aqui")] }]);
  });

  it("preserves whitespace, emojis and line breaks", () => {
    expect(formatMessage("🙂 *Bom dia*\n\n_linha 1\nlinha 2_" )).toEqual([
      plain("🙂 "), { type: "bold", children: [plain("Bom dia")] }, plain("\n\n"),
      { type: "italic", children: [plain("linha 1\nlinha 2")] },
    ]);
  });

  it("keeps incomplete markers, math, identifiers and HTML literal", () => {
    for (const text of ["*sem fim", "_sem fim", "* espaço*", "_espaço _", "nome_do_cliente", "ação_importante_aqui", "2 * 3 * 4", "a*b*c", "**texto**", "\\*literal\\*", "<script>alert('x')</script>"]) {
      expect(formatMessage(text)).toEqual([plain(text)]);
    }
  });

  it("does not consume underscores inside links or emails", () => {
    for (const text of ["https://exemplo.com/_arquivo_", "https://exemplo.com/a_b_c", "nome_sobrenome@empresa.com", "_nome_@empresa.com"]) {
      expect(formatMessage(text)).toEqual([plain(text)]);
    }
    expect(formatMessage("*https://exemplo.com/a_b_c*" )).toEqual([{ type: "bold", children: [plain("https://exemplo.com/a_b_c")] }]);
    expect(formatMessage("_nome_sobrenome@empresa.com_" )).toEqual([{ type: "italic", children: [plain("nome_sobrenome@empresa.com")] }]);
  });
});
