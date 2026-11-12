/**
 * Separa o que a pessoa colou ou digitou em palavras-chave: uma por linha, ou separadas por vírgula ou ponto e vírgula.
 * Tira espaços e linhas vazias e descarta repetidas (sem diferenciar maiúsculas). Cada palavra pode ter várias palavras
 * ("nota fiscal") e um "*" no fim para pegar variações ("emit*").
 */
export function parsePhrases(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of text.split(/[\n\r,;]+/)) {
    const phrase = raw.replace(/\s+/g, " ").trim();
    const key = phrase.toLowerCase();
    if (!phrase || seen.has(key)) continue;
    seen.add(key);
    out.push(phrase);
  }
  return out;
}

/** O que o servidor aceita no curinga: "*" só no fim de uma palavra, com 3+ letras antes dele. */
export function validWildcards(phrase: string): boolean {
  return phrase.split(/\s+/).every((w) => {
    const i = w.indexOf("*");
    return i < 0 || (i === w.length - 1 && w.slice(0, i).length >= 3);
  });
}
