export type DedupeReport = {
  applied: boolean;
  contactsRenamed: number;
  contactsMerged: number;
  contactsSkipped: number;
  duplicateMessages: number;
  filesRemoved: number;
  conversationsRemoved: number;
};

export const plural = (n: number, one: string, many: string) => `${n.toLocaleString("pt-BR")} ${n === 1 ? one : many}`;

/** O que a correção encontrou (ou fez), em frases simples. Vazio = nada a corrigir. */
export function describeReport(r: DedupeReport): string[] {
  const out: string[] = [];
  if (r.duplicateMessages) {
    out.push(
      `${plural(r.duplicateMessages, "mensagem repetida", "mensagens repetidas")}${r.filesRemoved ? ` (${plural(r.filesRemoved, "arquivo repetido", "arquivos repetidos")})` : ""}: fica uma cópia de cada.`,
    );
  }
  if (r.contactsMerged) out.push(`${plural(r.contactsMerged, "contato duplicado", "contatos duplicados")}: unido${r.contactsMerged === 1 ? "" : "s"} ao contato do número, com as conversas juntas.`);
  if (r.contactsRenamed) out.push(`${plural(r.contactsRenamed, "contato", "contatos")} sem telefone: ganha${r.contactsRenamed === 1 ? "" : "m"} o número que o WhatsApp já informou.`);
  if (r.conversationsRemoved) out.push(`${plural(r.conversationsRemoved, "conversa", "conversas")} que ficar${r.conversationsRemoved === 1 ? "ia" : "iam"} vazia${r.conversationsRemoved === 1 ? "" : "s"} sai${r.conversationsRemoved === 1 ? "" : "m"}.`);
  return out;
}
