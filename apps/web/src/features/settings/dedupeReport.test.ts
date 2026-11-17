import { describe, expect, it } from "vitest";
import { describeReport, type DedupeReport } from "./dedupeReport";

const base: DedupeReport = { applied: false, contactsRenamed: 0, contactsMerged: 0, contactsSkipped: 0, duplicateMessages: 0, filesRemoved: 0, conversationsRemoved: 0 };

describe("describeReport", () => {
  it("sem nada a corrigir, não há frases", () => {
    expect(describeReport(base)).toEqual([]);
    expect(describeReport({ ...base, contactsSkipped: 3 })).toEqual([]); // só "pulados": nada a aplicar
  });

  it("descreve mensagens repetidas com os arquivos e usa o plural certo", () => {
    expect(describeReport({ ...base, duplicateMessages: 1541, filesRemoved: 986 })[0]).toBe("1.541 mensagens repetidas (986 arquivos repetidos): fica uma cópia de cada.");
    expect(describeReport({ ...base, duplicateMessages: 1 })[0]).toBe("1 mensagem repetida: fica uma cópia de cada.");
  });

  it("descreve contatos unidos, com número e conversas vazias", () => {
    const l = describeReport({ ...base, contactsMerged: 216, contactsRenamed: 136, conversationsRemoved: 2 });
    expect(l[0]).toContain("216 contatos duplicados");
    expect(l[1]).toContain("136 contatos sem telefone");
    expect(l[2]).toContain("2 conversas");
  });
});
