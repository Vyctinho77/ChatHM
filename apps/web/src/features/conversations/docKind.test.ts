import { describe, expect, it } from "vitest";
import { IMAGE_TYPES, docKind, docKindLabel, officeApp } from "./docKind";

describe("tipo de documento", () => {
  it("reconhece Word, Excel, PowerPoint e PDF pela extensão, sem depender de maiúsculas", () => {
    for (const n of ["a.doc", "B.DOCX", "c.docm", "d.rtf", "e.odt"]) expect(docKind(n)).toBe("word");
    for (const n of ["a.xls", "B.XLSX", "c.xlsm", "d.csv", "e.ods"]) expect(docKind(n)).toBe("excel");
    for (const n of ["a.ppt", "B.PPTX", "c.ppsx"]) expect(docKind(n)).toBe("powerpoint");
    expect(docKind("guia.PDF")).toBe("pdf");
  });

  it("cai no tipo MIME quando o nome não ajuda (WhatsApp às vezes manda o arquivo sem extensão)", () => {
    expect(docKind("relatorio", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")).toBe("word");
    expect(docKind("balanco", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")).toBe("excel");
    expect(docKind("balanco", "application/vnd.ms-excel")).toBe("excel");
    expect(docKind("apresentacao", "application/vnd.openxmlformats-officedocument.presentationml.presentation")).toBe("powerpoint");
    expect(docKind("guia", "application/pdf")).toBe("pdf");
  });

  it("a extensão manda mais que um MIME genérico, e o resto é 'outro'", () => {
    expect(docKind("planilha.xlsx", "application/octet-stream")).toBe("excel");
    expect(docKind("notas.txt", "text/plain")).toBe("other");
    expect(docKind("backup.zip", "application/zip")).toBe("other");
    expect(docKind("semextensao")).toBe("other");
    expect(docKind("termina.", "")).toBe("other");
  });

  it("só Word, Excel e PowerPoint abrem no aplicativo do computador; PDF abre no visualizador do app", () => {
    expect(officeApp).toEqual({ pdf: null, word: "Word", excel: "Excel", powerpoint: "PowerPoint", other: null });
    expect(docKindLabel("word")).toBe("Word");
    expect(docKindLabel("other", "txt")).toBe("TXT");
    expect(docKindLabel("other")).toBe("Arquivo");
  });

  it("só jpeg, png e webp vão como foto; gif e heic vão como documento", () => {
    expect(IMAGE_TYPES).toEqual(["image/jpeg", "image/png", "image/webp"]);
  });
});

describe("documento sem nome", () => {
  it("não quebra quando o WhatsApp não envia o nome do arquivo", () => {
    expect(docKind(undefined, "application/pdf")).toBe("pdf");
    expect(docKind(undefined)).toBe("other");
  });
});
