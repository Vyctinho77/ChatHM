// @menção: achar o "@..." que a pessoa digita, filtrar quem mencionar, inserir o nome e, na hora de enviar, descobrir
// quem foi citado no texto. O servidor confere tudo de novo (pessoa ativa, nome igual ao escrito e presente no texto).
import { fold } from "@/lib/search";

/** Uma pessoa citada: `name` é o texto escrito depois do @ (o nome atual dela). */
export type MentionRef = { id: string; name: string };
export type MentionPerson = { id: string; name: string; subtitle?: string };
export type Segment = { text: string; mention?: MentionRef };

/** Ids da lista de sugestões (o campo de mensagem aponta para o item escolhido, para leitores de tela). */
export const mentionListId = "mention-list";
export const mentionOptionId = (id: string) => `mention-option-${id}`;

const MAX_QUERY = 30;
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
// "foo@ana" (e-mail) não é menção: antes do @ só pode haver início, espaço ou pontuação que abre.
const before = "(?<![\\p{L}\\p{N}_.])";
const after = "(?![\\p{L}\\p{N}_])";

/** O "@..." que está sendo digitado na posição do cursor; null quando o cursor não está numa menção. */
export function findMentionQuery(text: string, caret: number): { start: number; query: string } | null {
  const head = text.slice(0, caret);
  const at = head.lastIndexOf("@");
  if (at < 0) return null;
  if (at > 0 && !/[\s([{]/.test(head[at - 1]!)) return null;
  const query = head.slice(at + 1);
  // Nomes compostos têm até três palavras; passando disso (ou de uma quebra de linha) a pessoa já escreve outra coisa.
  if (query.length > MAX_QUERY || /[\n@]/.test(query) || (query.match(/\s/g)?.length ?? 0) > 2) return null;
  return { start: at, query };
}

/** Quem combina com o que foi digitado (sem acento, por início de qualquer palavra do nome); nunca a própria pessoa. */
export function filterPeople<T extends MentionPerson>(people: T[], query: string, myId: string, limit = 6): T[] {
  const q = fold(query).trimStart();
  const typed = q.trimEnd();
  const hits = people.filter((p) => {
    if (p.id === myId) return false;
    const name = fold(p.name);
    return !typed || name.startsWith(q) || name.split(/\s+/).some((w) => w.startsWith(q)) || name.includes(" " + q);
  });
  const rank = (p: T) => (fold(p.name).startsWith(typed) ? 0 : 1);
  return hits.sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name, "pt-BR")).slice(0, limit);
}

/** Troca o "@consulta" pelo nome escolhido (mais um espaço) e diz onde o cursor fica. */
export function applyMention(text: string, start: number, caret: number, name: string): { text: string; caret: number } {
  const head = text.slice(0, start);
  const tail = text.slice(caret).replace(/^ /, "");
  const insert = `@${name} `;
  return { text: head + insert + tail, caret: head.length + insert.length };
}

/**
 * Quem o texto cita: cada "@Nome" que for o nome exato de alguém, com o nome mais longo vencendo ("@Ana Paula" não cita
 * a "Ana"). Nomes repetidos usam quem foi escolhido na lista (`picked`: nome → id), senão a primeira pessoa.
 */
export function extractMentions(text: string, people: MentionPerson[], picked?: Map<string, string>): MentionRef[] {
  const byName = new Map<string, MentionPerson[]>();
  for (const p of people) byName.set(p.name, [...(byName.get(p.name) ?? []), p]);
  let work = text;
  const seen = new Set<string>();
  const out: MentionRef[] = [];
  for (const name of [...byName.keys()].sort((a, b) => b.length - a.length)) {
    let hit = false;
    work = work.replace(new RegExp(`${before}@${escape(name)}${after}`, "gu"), (m) => {
      hit = true;
      return " ".repeat(m.length); // não deixa um nome curto casar dentro de um comprido
    });
    if (!hit) continue;
    const candidates = byName.get(name)!;
    const id = candidates.find((c) => c.id === picked?.get(name))?.id ?? candidates[0]!.id;
    if (!seen.has(id)) {
      seen.add(id);
      out.push({ id, name });
    }
  }
  return out;
}

/** Divide o texto nos trechos de menção e no resto, para desenhar cada um do seu jeito. */
export function splitMentions(text: string, mentions: MentionRef[] | undefined): Segment[] {
  if (!mentions?.length) return [{ text }];
  const byName = new Map(mentions.map((m) => [m.name, m]));
  const names = [...byName.keys()].sort((a, b) => b.length - a.length).map(escape).join("|");
  const re = new RegExp(`${before}@(${names})${after}`, "gu");
  const out: Segment[] = [];
  let last = 0;
  for (const m of text.matchAll(re)) {
    const i = m.index!;
    if (i > last) out.push({ text: text.slice(last, i) });
    out.push({ text: m[0], mention: byName.get(m[1]!) });
    last = i + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out.length ? out : [{ text }];
}
