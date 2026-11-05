import { useSyncExternalStore } from "react";
import { applyTheme, getStoredTheme, type Theme } from "@/lib/theme";

// Fonte única do tema na aplicação. Persistido localmente (aplicado antes do
// primeiro paint, ver index.html) e, quando logado, sincronizado com o servidor.
let current: Theme = getStoredTheme();
const listeners = new Set<() => void>();

export function setTheme(t: Theme) {
  if (t === current) return;
  current = t;
  applyTheme(t);
  listeners.forEach((l) => l());
}

export function useTheme(): Theme {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}
