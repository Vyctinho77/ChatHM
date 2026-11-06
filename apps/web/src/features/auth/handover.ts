import { useSyncExternalStore } from "react";

/** Resultado da entrega da instância (fica só na memória desta aba até ser copiado e fechado). */
export type HandoverResult = { code: string; url: string };

let current: HandoverResult | null = null;
const listeners = new Set<() => void>();

export function setHandover(r: HandoverResult | null) {
  current = r;
  listeners.forEach((l) => l());
}

export function useHandover(): HandoverResult | null {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => current,
  );
}
