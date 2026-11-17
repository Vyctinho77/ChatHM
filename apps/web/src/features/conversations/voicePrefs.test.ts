import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_VOICE_PREFS, getVoicePrefs, setVoicePrefs } from "./voicePrefs";

function fakeStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return { getItem: (k: string) => data[k] ?? null, setItem: (k: string, v: string) => void (data[k] = v), data };
}

describe("voicePrefs", () => {
  let storage: ReturnType<typeof fakeStorage>;
  beforeEach(() => {
    storage = fakeStorage();
    vi.stubGlobal("localStorage", storage);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("começa com redução de ruído e eco ligados", () => {
    expect(getVoicePrefs()).toEqual(DEFAULT_VOICE_PREFS);
    expect(DEFAULT_VOICE_PREFS).toEqual({ noiseSuppression: true, echoCancellation: true });
  });

  it("guarda a mudança de uma opção sem mexer na outra", () => {
    setVoicePrefs({ noiseSuppression: false });
    expect(getVoicePrefs()).toEqual({ noiseSuppression: false, echoCancellation: true });
    setVoicePrefs({ echoCancellation: false });
    expect(getVoicePrefs()).toEqual({ noiseSuppression: false, echoCancellation: false });
  });

  it("ignora valores guardados que não são válidos", () => {
    storage.data["chathm.voice.v1"] = JSON.stringify({ noiseSuppression: "não", echoCancellation: false });
    expect(getVoicePrefs()).toEqual({ noiseSuppression: true, echoCancellation: false });
    storage.data["chathm.voice.v1"] = "{lixo";
    expect(getVoicePrefs()).toEqual(DEFAULT_VOICE_PREFS);
  });

  it("funciona sem armazenamento disponível", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new Error("bloqueado");
      },
      setItem: () => {
        throw new Error("bloqueado");
      },
    });
    expect(getVoicePrefs()).toEqual(DEFAULT_VOICE_PREFS);
    expect(setVoicePrefs({ noiseSuppression: false }).noiseSuppression).toBe(false);
  });
});
