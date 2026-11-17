/**
 * Preferências do microfone para gravar mensagens de voz. Ficam neste navegador (e não na conta): dependem do aparelho e do microfone
 * que se usa aqui. O padrão (redução de ruído e eco ligados) é o que quase todo mundo quer; quem notar a voz "oca" pode desligar.
 */
export type VoicePrefs = { noiseSuppression: boolean; echoCancellation: boolean };

const KEY = "chathm.voice.v1";
export const DEFAULT_VOICE_PREFS: VoicePrefs = { noiseSuppression: true, echoCancellation: true };

export function getVoicePrefs(): VoicePrefs {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) ?? "null") as Partial<VoicePrefs> | null;
    return {
      noiseSuppression: typeof raw?.noiseSuppression === "boolean" ? raw.noiseSuppression : DEFAULT_VOICE_PREFS.noiseSuppression,
      echoCancellation: typeof raw?.echoCancellation === "boolean" ? raw.echoCancellation : DEFAULT_VOICE_PREFS.echoCancellation,
    };
  } catch {
    return { ...DEFAULT_VOICE_PREFS }; // sem localStorage (janela privativa, bloqueio): usa o padrão
  }
}

export function setVoicePrefs(patch: Partial<VoicePrefs>): VoicePrefs {
  const next = { ...getVoicePrefs(), ...patch };
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* sem armazenamento: vale só até recarregar a página */
  }
  return next;
}
