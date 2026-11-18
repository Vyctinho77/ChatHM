/**
 * O que o corretor aprendeu neste navegador (não há tela de configuração: é interno). Quando a pessoa desfaz duas vezes a correção da mesma
 * palavra (nomes, siglas, termos do escritório), o corretor deixa de corrigi-la por IGNORE_DAYS dias; depois volta a tentar, para um engano
 * antigo não valer para sempre.
 */
export type WritingPrefs = {
  /** palavra (minúscula) → quando foi aprendida (ms). */
  ignored: Record<string, number>;
  /** quantas vezes a pessoa desfez a correção de cada palavra (ao chegar em UNDO_LIMIT, vai para `ignored`). */
  undone: Record<string, number>;
};

const KEY = "chathm.writing.v2";
export const UNDO_LIMIT = 2;
export const IGNORE_DAYS = 90;
const DAY = 24 * 3600 * 1000;
const MAX_WORDS = 200;

const word = (w: unknown): string => (typeof w === "string" ? w.trim().toLowerCase() : "");
const validWord = (w: string) => /^\p{L}{1,40}$/u.test(w);

const entries = (v: unknown): [string, number][] =>
  v && typeof v === "object" && !Array.isArray(v) ? Object.entries(v as Record<string, unknown>).slice(0, MAX_WORDS).flatMap(([k, n]) => {
    const w = word(k);
    return validWord(w) && typeof n === "number" && Number.isFinite(n) && n > 0 ? [[w, n] as [string, number]] : [];
  }) : [];

/** Lê (e limpa) o que está salvo: qualquer coisa fora do formato é descartada; o que expirou também. */
export function parseWritingPrefs(raw: unknown, now = Date.now()): WritingPrefs {
  const r = (raw && typeof raw === "object" ? raw : {}) as { ignored?: unknown; undone?: unknown };
  const ignored = Object.fromEntries(entries(r.ignored).filter(([, at]) => now - at < IGNORE_DAYS * DAY && at <= now + DAY));
  const undone = Object.fromEntries(entries(r.undone).filter(([w, n]) => n < UNDO_LIMIT && !(w in ignored)).map(([w, n]) => [w, Math.floor(n)]));
  return { ignored, undone };
}

let cache: WritingPrefs | null = null;

export function getWritingPrefs(): WritingPrefs {
  if (cache) return cache;
  try {
    cache = parseWritingPrefs(JSON.parse(localStorage.getItem(KEY) ?? "null"));
  } catch {
    cache = { ignored: {}, undone: {} }; // sem localStorage (janela privativa): aprende só até recarregar
  }
  return cache;
}

function commit(next: WritingPrefs): WritingPrefs {
  cache = next;
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* sem armazenamento: vale só até recarregar a página */
  }
  return next;
}

/** As palavras que o corretor não deve corrigir agora. */
export const ignoredWords = (now = Date.now()): Set<string> => new Set(Object.keys(parseWritingPrefs(getWritingPrefs(), now).ignored));

/** A pessoa desfez a correção desta palavra. Na segunda vez, ela passa a ser ignorada. */
export function recordUndo(w: string, now = Date.now()): WritingPrefs {
  const lower = word(w);
  const cur = getWritingPrefs();
  if (!validWord(lower) || lower in cur.ignored) return cur;
  const count = (cur.undone[lower] ?? 0) + 1;
  if (count >= UNDO_LIMIT) {
    const { [lower]: _gone, ...rest } = cur.undone;
    void _gone;
    return commit({ ignored: { ...cur.ignored, [lower]: now }, undone: rest });
  }
  return commit({ ...cur, undone: { ...cur.undone, [lower]: count } });
}

/** A pessoa mandou nunca corrigir nem sublinhar esta palavra (cartão do botão direito). Vale por IGNORE_DAYS dias, como o aprendizado. */
export function ignoreWord(w: string, now = Date.now()): WritingPrefs {
  const lower = word(w);
  const cur = getWritingPrefs();
  if (!validWord(lower)) return cur;
  const { [lower]: _gone, ...undone } = cur.undone;
  void _gone;
  return commit({ ignored: { ...cur.ignored, [lower]: now }, undone });
}

/** Só para testes: esquece o que está em memória. */
export const resetWritingPrefsCache = () => {
  cache = null;
};
