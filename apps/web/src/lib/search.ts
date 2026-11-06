// Busca sem acento e sem diferenciar maiúsculas. `fold` preserva o tamanho do texto, então os índices
// encontrados valem para o texto original (usado para destacar o trecho).
const accents: Record<string, string> = {
  á: "a", à: "a", â: "a", ã: "a", ä: "a", é: "e", è: "e", ê: "e", ë: "e", í: "i", ì: "i", î: "i", ï: "i",
  ó: "o", ò: "o", ô: "o", õ: "o", ö: "o", ú: "u", ù: "u", û: "u", ü: "u", ç: "c", ñ: "n",
};

export function fold(s: string): string {
  let out = "";
  for (const ch of s.toLowerCase()) out += accents[ch] ?? ch;
  return out;
}

export const digitsOf = (s: string) => s.replace(/\D/g, "");

/** Procura `term` em `text`; com 3+ dígitos também casa pedaços do telefone. */
export function matches(text: string, term: string): boolean {
  const t = fold(term.trim());
  if (!t) return true;
  if (fold(text).includes(t)) return true;
  const d = digitsOf(term);
  return d.length >= 3 && digitsOf(text).includes(d);
}

export type Part = { text: string; hit: boolean };

/** Divide o texto em trechos, marcando as ocorrências de `term`. */
export function highlightParts(text: string, term: string): Part[] {
  const t = fold(term.trim());
  if (!t) return [{ text, hit: false }];
  const chars = [...text];
  const hay = [...fold(text)];
  if (chars.length !== hay.length) return [{ text, hit: false }];
  const needle = [...t];
  const parts: Part[] = [];
  let last = 0;
  for (let i = 0; i + needle.length <= hay.length; ) {
    let ok = true;
    for (let j = 0; j < needle.length; j++) {
      if (hay[i + j] !== needle[j]) {
        ok = false;
        break;
      }
    }
    if (ok) {
      if (i > last) parts.push({ text: chars.slice(last, i).join(""), hit: false });
      parts.push({ text: chars.slice(i, i + needle.length).join(""), hit: true });
      i += needle.length;
      last = i;
    } else i++;
  }
  if (last < chars.length) parts.push({ text: chars.slice(last).join(""), hit: false });
  return parts;
}

export function countHits(text: string, term: string): number {
  return highlightParts(text, term).filter((p) => p.hit).length;
}
