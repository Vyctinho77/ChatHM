import { useSyncExternalStore } from "react";

/**
 * Preferências de escrita do campo de mensagem. Ficam neste navegador, como as de som e microfone. Mudar aqui vale na hora, em todos os
 * campos abertos (e em outras abas do mesmo navegador).
 */
export type WritingPrefs = {
  /** Põe os acentos ao fechar a palavra. */
  autoAccent: boolean;
  /** Expande abreviações de chat (vc → você). */
  abbreviations: boolean;
  /** Palavras que nunca devem ser corrigidas (a pessoa pediu, ou desfez duas vezes). Minúsculas. */
  ignored: string[];
  /** Quantas vezes a pessoa desfez a correção de cada palavra (ao chegar em 2, a palavra vai para `ignored`). */
  undone: Record<string, number>;
};

const KEY = "chathm.writing.v1";
export const DEFAULT_WRITING_PREFS: WritingPrefs = { autoAccent: true, abbreviations: false, ignored: [], undone: {} };
export const UNDO_LIMIT = 2; // desfazer a mesma palavra tantas vezes = parar de corrigi-la
const MAX_IGNORED = 200;

const word = (w: unknown): string => (typeof w === "string" ? w.trim().toLowerCase() : "");
const validWord = (w: string) => /^\p{L}{1,40}$/u.test(w);

/** Lê (e limpa) o que está salvo: qualquer coisa fora do formato vira o padrão. */
export function parseWritingPrefs(raw: unknown): WritingPrefs {
  const r = (raw && typeof raw === "object" ? raw : {}) as Partial<Record<keyof WritingPrefs, unknown>>;
  const ignored = Array.isArray(r.ignored) ? [...new Set(r.ignored.map(word).filter(validWord))].slice(0, MAX_IGNORED) : [];
  const undone: Record<string, number> = {};
  if (r.undone && typeof r.undone === "object") {
    for (const [k, v] of Object.entries(r.undone as Record<string, unknown>).slice(0, MAX_IGNORED)) {
      const w = word(k);
      if (validWord(w) && typeof v === "number" && v > 0 && v < UNDO_LIMIT) undone[w] = Math.floor(v);
    }
  }
  return {
    autoAccent: typeof r.autoAccent === "boolean" ? r.autoAccent : DEFAULT_WRITING_PREFS.autoAccent,
    abbreviations: typeof r.abbreviations === "boolean" ? r.abbreviations : DEFAULT_WRITING_PREFS.abbreviations,
    ignored,
    undone,
  };
}

let cache: WritingPrefs | null = null;
const listeners = new Set<() => void>();

function read(): WritingPrefs {
  try {
    return parseWritingPrefs(JSON.parse(localStorage.getItem(KEY) ?? "null"));
  } catch {
    return { ...DEFAULT_WRITING_PREFS, ignored: [], undone: {} }; // sem localStorage (janela privativa): usa o padrão
  }
}

export function getWritingPrefs(): WritingPrefs {
  return (cache ??= read());
}

function commit(next: WritingPrefs): WritingPrefs {
  cache = next;
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* sem armazenamento: vale só até recarregar a página */
  }
  listeners.forEach((fn) => fn());
  return next;
}

export function setWritingPrefs(patch: Partial<WritingPrefs>): WritingPrefs {
  return commit(parseWritingPrefs({ ...getWritingPrefs(), ...patch }));
}

/** A pessoa desfez a correção desta palavra. Na segunda vez, a palavra passa a ser ignorada. */
export function recordUndo(w: string): WritingPrefs {
  const lower = word(w);
  if (!validWord(lower)) return getWritingPrefs();
  const cur = getWritingPrefs();
  if (cur.ignored.includes(lower)) return cur;
  const count = (cur.undone[lower] ?? 0) + 1;
  if (count >= UNDO_LIMIT) {
    const { [lower]: _gone, ...rest } = cur.undone;
    void _gone;
    return setWritingPrefs({ ignored: [...cur.ignored, lower], undone: rest });
  }
  return setWritingPrefs({ undone: { ...cur.undone, [lower]: count } });
}

export const ignoreWord = (w: string) => {
  const lower = word(w);
  return validWord(lower) ? setWritingPrefs({ ignored: [...new Set([...getWritingPrefs().ignored, lower])] }) : getWritingPrefs();
};
export const unignoreWord = (w: string) => setWritingPrefs({ ignored: getWritingPrefs().ignored.filter((x) => x !== word(w)) });

function subscribe(fn: () => void) {
  listeners.add(fn);
  const onStorage = (e: StorageEvent) => {
    if (e.key === KEY || e.key === null) {
      cache = null; // outra aba mudou
      fn();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(fn);
    window.removeEventListener("storage", onStorage);
  };
}

/** As preferências de escrita, sempre atuais. */
export function useWritingPrefs(): WritingPrefs {
  return useSyncExternalStore(subscribe, getWritingPrefs, () => DEFAULT_WRITING_PREFS);
}

/** Só para testes: esquece o que está em memória. */
export const resetWritingPrefsCache = () => {
  cache = null;
};
