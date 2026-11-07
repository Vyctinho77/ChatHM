/** Reconhece sites, e-mails e telefones no texto das mensagens. */
export type Piece =
  | { type: "text"; text: string }
  | { type: "url"; text: string; href: string }
  | { type: "email"; text: string; href: string }
  | { type: "phone"; text: string; digits: string };

const EMAIL = String.raw`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`;
const URL_FULL = String.raw`(?:https?:\/\/|www\.)[^\s<>"']+`;
// Sem protocolo: só com terminações comuns, para não pegar "nota.pdf" ou "fim.Obrigado".
const URL_BARE = String.raw`\b[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:com\.br|gov\.br|org\.br|net\.br|edu\.br|com|net|org|br|io|app|dev)\b(?:\/[^\s<>"']*)?`;
const PHONE = String.raw`(?<![\w.,/-])(?:\+\d{1,3}[\s.-]?)?(?:\(\d{2}\)[\s.-]?|\d{2}[\s.-]?)?\d{4,5}[\s.-]?\d{4}(?!\d)`;

const TOKEN = new RegExp(`(${EMAIL})|(${URL_FULL})|(${URL_BARE})|(${PHONE})`, "gi");

const TRAILING = /[.,;:!?)\]}"'’”]+$/;

/** CPF/CNPJ/valores/datas não são telefone. */
function isPhone(raw: string, before: string, after: string): string | null {
  // Colado em outro número (ex.: 12345678901234) ou parte de CPF/CNPJ/valor.
  if (/[\d.,/-]$/.test(before) || /^[\d,/]/.test(after) || /^\.\d/.test(after) || /^-\d/.test(after)) return null;
  if (/^\d{3}\.\d{3}\.\d{3}-\d{2}$/.test(raw) || /^\d{2}\.\d{3}\.\d{3}\/?\d{4}-?\d{2}$/.test(raw)) return null;
  const plus = raw.trim().startsWith("+");
  const digits = raw.replace(/\D/g, "");
  if (digits.length < 8 || digits.length > 15) return null;
  const formatted = /[\s().+-]/.test(raw.trim());
  let national = digits;
  if (digits.startsWith("55") && (digits.length === 12 || digits.length === 13)) national = digits.slice(2);
  else if (plus && digits.length >= 10) return digits; // +CC … (internacional)
  if (national.length === 11) {
    // celular com DDD: 9 depois do DDD; sem formatação só vale com essa forma (um CPF puro tem 11 dígitos quaisquer)
    if (!/^[1-9][1-9]9/.test(national)) return null;
  } else if (national.length === 10) {
    if (!/^[1-9][1-9][2-5]/.test(national)) return null;
    if (!formatted) return null; // 10 dígitos soltos: provável outro número
  } else if (national.length === 9) {
    if (!/^9/.test(national) || !formatted) return null; // 98800-1111 sem DDD
  } else if (national.length === 8) {
    if (!/^[2-5]/.test(national) || !/-/.test(raw)) return null; // 3333-4444
  } else return null;
  return digits.length <= 11 && national.length >= 10 ? "55" + national : digits.length <= 9 ? national : digits;
}

export function linkify(text: string): Piece[] {
  const out: Piece[] = [];
  let last = 0;
  const push = (t: string) => t && out.push({ type: "text", text: t });
  for (const m of text.matchAll(TOKEN)) {
    let raw = m[0];
    const start = m.index ?? 0;
    if (start < last) continue;
    if (m[1]) {
      raw = raw.replace(/[.,;:!?]+$/, "");
      push(text.slice(last, start));
      out.push({ type: "email", text: raw, href: `mailto:${raw}` });
      last = start + raw.length;
    } else if (m[2] || m[3]) {
      raw = raw.replace(TRAILING, "");
      // ")" final só sai se não houver "(" no próprio link
      if (raw.includes("(") && m[0].endsWith(")")) raw = m[0].replace(/[.,;:!?]+$/, "");
      if (!raw || raw.length < 4) continue;
      push(text.slice(last, start));
      out.push({ type: "url", text: raw, href: /^https?:\/\//i.test(raw) ? raw : `https://${raw}` });
      last = start + raw.length;
    } else {
      const trimmed = raw.replace(/^\s+|\s+$/g, "");
      const lead = raw.length - raw.trimStart().length;
      const s = start + lead;
      const digits = isPhone(trimmed, text.slice(Math.max(0, s - 2), s), text.slice(s + trimmed.length, s + trimmed.length + 2));
      if (!digits) continue;
      push(text.slice(last, s));
      out.push({ type: "phone", text: trimmed, digits });
      last = s + trimmed.length;
    }
  }
  push(text.slice(last));
  return out.length ? out : [{ type: "text", text }];
}
