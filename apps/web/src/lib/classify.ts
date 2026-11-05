// Versão em TypeScript do motor de classificação, usada SOMENTE pela demonstração
// (a API real classifica no backend Go, em backend/internal/segments). Mantenha as
// regras idênticas; classify.test.ts repete os casos de aceite do backend.

const STOPWORDS = new Set([
  "a", "o", "as", "os", "um", "uma", "uns", "umas", "de", "do", "da", "dos", "das", "no", "na", "nos", "nas",
  "em", "ao", "aos", "para", "pra", "pro", "por", "com", "e", "que", "meu", "minha", "meus", "minhas", "seu",
  "sua", "seus", "suas", "me", "te", "se",
]);

function singular(w: string): string {
  const n = w.length;
  const has = (suffix: string) => n > 4 && w.endsWith(suffix);
  if (has("oes") || has("aes")) return w.slice(0, n - 3) + "ao";
  if (has("ais")) return w.slice(0, n - 3) + "al";
  if (has("eis")) return w.slice(0, n - 3) + "el";
  if (has("ns")) return w.slice(0, n - 2) + "m";
  if (has("res") || has("zes")) return w.slice(0, n - 2);
  if (n > 3 && w.endsWith("s") && !w.endsWith("ss")) return w.slice(0, n - 1);
  return w;
}

export function tokens(text: string): string[] {
  const clean = text
    .toLowerCase()
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .replace(/[^\p{L}\p{N}]+/gu, " ");
  return clean
    .split(" ")
    .filter((w) => w && !STOPWORDS.has(w))
    .map(singular);
}

export const normalize = (text: string) => tokens(text).join(" ");

export type DemoKeyword = { segmentId: string; segmentKey: string; phrase: string; normalized: string; priority: number };
export type Match = { segmentId: string; segmentKey: string; phrase: string; priority: number };
export type Classification = {
  segmentId?: string;
  segmentKey?: string;
  reason: "keyword" | "conflict" | "none";
  matches: Match[];
};

function containsSeq(hay: string[], needle: string[]) {
  for (let i = 0; i + needle.length <= hay.length; i++) {
    if (needle.every((w, j) => hay[i + j] === w)) return true;
  }
  return false;
}

export function classify(text: string, keywords: DemoKeyword[]): Classification {
  const t = tokens(text);
  const res: Classification = { reason: "none", matches: [] };
  if (t.length === 0) return res;

  const best = new Map<string, number>();
  const keys = new Map<string, string>();
  for (const k of keywords) {
    const kt = k.normalized.split(" ").filter(Boolean);
    if (kt.length === 0 || !containsSeq(t, kt)) continue;
    res.matches.push({ segmentId: k.segmentId, segmentKey: k.segmentKey, phrase: k.phrase, priority: k.priority });
    best.set(k.segmentId, Math.max(best.get(k.segmentId) ?? -1, k.priority));
    keys.set(k.segmentId, k.segmentKey);
  }
  if (best.size === 0) return res;
  res.matches.sort((a, b) => b.priority - a.priority);

  const top = Math.max(...best.values());
  const winners = [...best].filter(([, p]) => p === top).map(([id]) => id);
  if (winners.length === 1) {
    const id = winners[0]!;
    return { ...res, segmentId: id, segmentKey: keys.get(id), reason: "keyword" };
  }
  return { ...res, reason: "conflict" };
}
