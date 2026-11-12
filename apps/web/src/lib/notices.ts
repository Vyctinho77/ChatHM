import { useSyncExternalStore } from "react";

// Avisos curtos na tela ("fulano encerrou o atendimento para todos"): somem sozinhos, e dá para fechar antes.
export type Notice = { id: string; text: string; link?: { to: string; label: string } };

const SHOW_MS = 10_000;
const MAX = 4;
let notices: Notice[] = [];
const listeners = new Set<() => void>();
const timers = new Map<string, ReturnType<typeof setTimeout>>();
const notify = () => listeners.forEach((l) => l());

export function dismissNotice(id: string) {
  clearTimeout(timers.get(id));
  timers.delete(id);
  if (notices.some((n) => n.id === id)) {
    notices = notices.filter((n) => n.id !== id);
    notify();
  }
}

/** O mesmo `id` (por exemplo, o do evento) não aparece duas vezes. */
export function pushNotice(n: Notice) {
  if (notices.some((x) => x.id === n.id)) return;
  notices = [...notices, n].slice(-MAX);
  timers.set(n.id, setTimeout(() => dismissNotice(n.id), SHOW_MS));
  notify();
}

export function clearNotices() {
  for (const t of timers.values()) clearTimeout(t);
  timers.clear();
  notices = [];
  notify();
}

export function useNotices() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => {
        listeners.delete(l);
      };
    },
    () => notices,
    () => notices,
  );
}
