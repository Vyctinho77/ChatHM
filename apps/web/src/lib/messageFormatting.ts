import { linkify } from "./linkify";

export type FormattedPart = { type: "text"; text: string } | { type: "bold" | "italic"; children: FormattedPart[] };

const word = /[\p{L}\p{N}\p{M}_]/u;
const marker = (char: string) => char === "*" || char === "_";

/** Marcadores do WhatsApp: *negrito* e _itálico_. Nunca interpreta HTML. */
export function formatMessage(text: string): FormattedPart[] {
  // Marcadores em caminhos de URLs e endereços de e-mail são texto literal.
  const protectedChars = new Uint8Array(text.length);
  let offset = 0;
  for (const piece of linkify(text)) {
    if (piece.type !== "text") {
      let start = offset;
      let end = offset + piece.text.length;
      while (start < end && marker(text[start]!)) start++;
      // Um fechamento externo pode estar colado ao final do link: *https://site.com*.
      while (end > offset && marker(text[end - 1]!)) end--;
      protectedChars.fill(1, start, end);
    }
    offset += piece.text.length;
  }

  const pairs = new Map<number, number>();
  const stack: { char: string; index: number }[] = [];
  for (let i = 0; i < text.length; i++) {
    const char = text[i]!;
    if (!marker(char) || protectedChars[i]) continue;
    const before = text[i - 1] ?? "", after = text[i + 1] ?? "";
    if (before === char || after === char) continue;
    let slashes = 0;
    for (let j = i - 1; text[j] === "\\"; j--) slashes++;
    if (slashes % 2) continue;
    const top = stack.at(-1);
    if (top?.char === char && i > top.index + 1 && /\S/.test(before) && !word.test(after)) {
      pairs.set(top.index, i);
      stack.pop();
    } else if (after && /\S/.test(after) && !word.test(before) && !stack.some((s) => s.char === char)) {
      stack.push({ char, index: i });
    }
  }

  function parts(start: number, end: number): FormattedPart[] {
    const out: FormattedPart[] = [];
    let plain = start;
    for (let i = start; i < end; i++) {
      const closing = pairs.get(i);
      if (closing === undefined || closing >= end) continue;
      if (plain < i) out.push({ type: "text", text: text.slice(plain, i) });
      out.push({ type: text[i] === "*" ? "bold" : "italic", children: parts(i + 1, closing) });
      i = closing;
      plain = closing + 1;
    }
    if (plain < end) out.push({ type: "text", text: text.slice(plain, end) });
    return out;
  }
  return parts(0, text.length);
}
