import { correctWordLayer, type CorrectOptions } from "./autocorrect";
import { correctTypoLayer, fold, getTypoVocabulary } from "./typos";

/**
 * Verificador de palavras do campo de mensagem: sublinha (em vermelho, ondulado) as palavras que não existem em português e, com o botão direito
 * em cima delas, mostra um cartão com sugestões. Funciona igual em qualquer navegador (não depende do corretor do navegador).
 *
 * "Existe" = está no filtro de Bloom com todas as formas do dicionário VERO pt-BR (ptValid.bin, ~3 MB, carregado sob demanda e guardado pelo
 * navegador) ou na lista de palavras frequentes. Nome próprio (inicial maiúscula no meio da frase), sigla (TUDO MAIÚSCULO), e-mail, link,
 * número, risada e termos do escritório não são sublinhados.
 */

// --- filtro de Bloom (mesmo formato e mesmas funções de scripts/gen-pt-bloom.py) ---
type Bloom = { k: number; m: number; bits: Uint8Array };
let bloom: Bloom | null = null;
let loading: Promise<void> | null = null;

export function setBloom(bytes: Uint8Array | null) {
  if (!bytes) return void (bloom = null);
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  bloom = { k: bytes[0]!, m: view.getUint32(1, true), bits: bytes.subarray(5) };
}
export const isSpellDictionaryLoaded = () => bloom !== null;

/** Carrega o dicionário de palavras (um arquivo à parte). Chamar de novo não baixa de novo. */
export function loadSpellDictionary(): Promise<void> {
  loading ??= fetch(new URL("./ptValid.bin", import.meta.url))
    .then((r) => (r.ok ? r.arrayBuffer() : Promise.reject(new Error(String(r.status)))))
    .then((buf) => setBloom(new Uint8Array(buf)))
    .catch(() => {
      loading = null; // sem rede agora: tenta de novo na próxima vez
    });
  return loading;
}

const enc = new TextEncoder();

/** h1 = FNV-1a 32 bits; h2 = djb2 com xor, ímpar. */
export function hashes(word: string): [number, number] {
  let h1 = 2166136261;
  let h2 = 5381;
  for (const b of enc.encode(word)) {
    h1 = Math.imul(h1 ^ b, 16777619) >>> 0;
    h2 = (((h2 << 5) + h2) ^ b) >>> 0;
  }
  return [h1, (h2 | 1) >>> 0];
}

function inBloom(b: Bloom, word: string): boolean {
  const [h1, h2] = hashes(word);
  for (let i = 0; i < b.k; i++) {
    const p = ((h1 + Math.imul(i, h2)) >>> 0) % b.m;
    if (!(b.bits[p >> 3]! & (1 << (p & 7)))) return false;
  }
  return true;
}

/** Só o filtro de Bloom (para testes): a palavra passa? */
export const bloomHas = (word: string): boolean => (bloom ? inBloom(bloom, word) : false);

/** A palavra (minúscula, com ou sem acento como foi digitada) existe? Sem o dicionário carregado, tudo "existe" (nada é sublinhado). */
export function isKnownWord(lower: string): boolean {
  if (!bloom) return true;
  return inBloom(bloom, lower.normalize("NFC")) || getTypoVocabulary()?.has(lower) === true;
}

// --- o que sublinhar ---
const LAUGH = /^(k+|(rs)+|(ha)+h?|(he)+h?|(hi)+|(ah)+h?|(eh)+|(oh)+|(ue)+|(u+h)|(a+h+))$/;
const BLOCKED_BEFORE = "@/\\:#._-{<[+=~$%&*'’";
const DOMAIN = new Set(
  `cnpj cnpjs cpfs nfse nfce nfe icms simples presumido esocial fgts caged cnae cfop darf dctf sped irpj irpf csll cofins whatsapp asaas boleto token pix
pdf pdfs xml xmls gov certificado digital ecnpj ecpf mei das gps dirf rais inss iss ipi pis ncm cest efd ecf ecd`.split(/\s+/).filter(Boolean),
);
const MIN_LEN = 3;

export type Unknown = { start: number; end: number; word: string };

/**
 * As palavras de `text` que não existem. `caret`: a palavra em que o cursor está ainda está sendo digitada e não é sublinhada.
 * `ignored`: palavras que a pessoa mandou não corrigir.
 */
export function findUnknownWords(text: string, caret: number, ignored: ReadonlySet<string> = new Set()): Unknown[] {
  if (!bloom) return [];
  const out: Unknown[] = [];
  for (const m of text.matchAll(/\p{L}+/gu)) {
    const word = m[0];
    const start = m.index!;
    const end = start + word.length;
    if (word.length < MIN_LEN || (caret >= start && caret <= end)) continue;
    const before = text.charAt(start - 1);
    const after = text.charAt(end);
    if ((before !== "" && BLOCKED_BEFORE.includes(before)) || /\p{N}/u.test(before) || /\p{N}/u.test(after) || after === "@" || after === "-" || (after === "." && /\p{L}/u.test(text.charAt(end + 1)))) continue;
    const lower = word.toLowerCase();
    const upper = word === word.toUpperCase();
    const capital = word === lower.charAt(0).toUpperCase() + lower.slice(1);
    if (upper || (!capital && word !== lower)) continue; // sigla ou maiúsculas misturadas (iPhone)
    if (capital && !/(^|[.!?…]\s+|\n)\s*$/.test(text.slice(0, start))) continue; // nome próprio no meio da frase
    if (LAUGH.test(lower) || DOMAIN.has(lower) || ignored.has(lower) || isKnownWord(lower)) continue;
    out.push({ start, end, word });
  }
  return out;
}

// --- sugestões ---
const ALPHA = "abcdefghijklmnopqrstuvwxyz";

function edits1(f: string): Set<string> {
  const out = new Set<string>();
  for (let i = 0; i <= f.length; i++) {
    if (i < f.length) out.add(f.slice(0, i) + f.slice(i + 1));
    if (i < f.length - 1) out.add(f.slice(0, i) + f[i + 1] + f[i] + f.slice(i + 2));
    for (const c of ALPHA) {
      if (i < f.length) out.add(f.slice(0, i) + c + f.slice(i + 1));
      out.add(f.slice(0, i) + c + f.slice(i));
    }
  }
  out.delete(f);
  return out;
}

const withCase = (like: string, w: string) => (like !== like.toLowerCase() && like === like.charAt(0).toUpperCase() + like.slice(1).toLowerCase() ? w.charAt(0).toUpperCase() + w.slice(1) : w);

/** Até `limit` sugestões para uma palavra que não existe: primeiro a correção que o corretor faria sozinho; depois as palavras frequentes a 1 e a 2 enganos. */
export function suggest(word: string, limit = 5, opts: CorrectOptions = {}): string[] {
  const out: string[] = [];
  const push = (w: string | undefined) => {
    if (w && w.toLowerCase() !== word.toLowerCase() && !out.includes(w)) out.push(w);
  };
  push(correctWordLayer(word, opts)?.to);
  push(correctTypoLayer(word, { sentenceStart: true })?.to);
  const vocab = getTypoVocabulary();
  if (vocab && out.length < limit) {
    const f = fold(word.toLowerCase());
    const rankOf = (w: string) => vocab.rank.get(w) ?? Infinity;
    const near = (set: Iterable<string>) => {
      const found: string[] = [];
      for (const e of set) for (const w of vocab.variants.get(e) ?? []) found.push(w);
      return found.sort((a, b) => rankOf(a) - rankOf(b));
    };
    const one = edits1(f);
    for (const w of near(one)) {
      if (out.length >= limit) break;
      push(withCase(word, w));
    }
    if (out.length < limit && f.length >= 4) {
      const two = new Set<string>();
      for (const e of one) for (const e2 of edits1(e)) if (vocab.variants.has(e2) && !one.has(e2)) two.add(e2);
      for (const w of near(two)) {
        if (out.length >= limit) break;
        push(withCase(word, w));
      }
    }
  }
  return out.slice(0, limit);
}
