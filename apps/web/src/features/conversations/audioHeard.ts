import { useSyncExternalStore } from "react";

/**
 * Mensagens de voz recebidas que esta pessoa já ouviu (verde = não ouvida, azul = ouvida).
 * Fica só neste navegador (localStorage): o WhatsApp não informa ao ChatHM quem ouviu o quê.
 */
const KEY = "chathm:heard-audios";
const MAX = 3000;

let cache: Set<string> | null = null;
const listeners = new Set<() => void>();

function load(): Set<string> {
  if (cache) return cache;
  try {
    const raw = localStorage.getItem(KEY);
    const list: unknown = raw ? JSON.parse(raw) : [];
    cache = new Set(Array.isArray(list) ? list.filter((x): x is string => typeof x === "string") : []);
  } catch {
    cache = new Set();
  }
  return cache;
}

function notify() {
  listeners.forEach((l) => l());
}

export function markHeard(id: string) {
  const set = load();
  if (!id || set.has(id)) return;
  set.add(id);
  try {
    localStorage.setItem(KEY, JSON.stringify([...set].slice(-MAX)));
  } catch {
    // sem armazenamento (modo privado): vale só até recarregar a página
  }
  notify();
}

export function isHeard(id: string) {
  return load().has(id);
}

/** Só para os testes. */
export function resetHeardCache() {
  cache = null;
  notify();
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  const onStorage = (e: StorageEvent) => {
    if (e.key === KEY || e.key === null) {
      cache = null; // outra aba ouviu um áudio
      cb();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("storage", onStorage);
  };
}

export function useHeard(id: string) {
  return useSyncExternalStore(subscribe, () => isHeard(id), () => false);
}
