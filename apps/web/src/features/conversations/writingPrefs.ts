/** Preferências de escrita do campo de mensagem. Ficam neste navegador, como as de som e microfone. */
export type WritingPrefs = { autoAccent: boolean };

const KEY = "chathm.writing.v1";
export const DEFAULT_WRITING_PREFS: WritingPrefs = { autoAccent: true };

export function getWritingPrefs(): WritingPrefs {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "null") as Partial<WritingPrefs> | null;
    return { autoAccent: typeof raw?.autoAccent === "boolean" ? raw.autoAccent : DEFAULT_WRITING_PREFS.autoAccent };
  } catch {
    return { ...DEFAULT_WRITING_PREFS }; // sem localStorage (janela privativa): usa o padrão
  }
}

export function setWritingPrefs(patch: Partial<WritingPrefs>): WritingPrefs {
  const next = { ...getWritingPrefs(), ...patch };
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* sem armazenamento: vale só até recarregar a página */
  }
  return next;
}
