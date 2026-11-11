import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { isHeard, markHeard, resetHeardCache } from "./audioHeard";

function fakeStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return {
    data,
    getItem: (k: string) => (k in data ? data[k] : null),
    setItem: (k: string, v: string) => void (data[k] = v),
  };
}

describe("áudios ouvidos", () => {
  beforeEach(() => {
    vi.stubGlobal("localStorage", fakeStorage());
    vi.stubGlobal("window", { addEventListener: vi.fn(), removeEventListener: vi.fn() });
    resetHeardCache();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("começa como não ouvido e lembra depois de ouvir", () => {
    expect(isHeard("a1")).toBe(false);
    markHeard("a1");
    expect(isHeard("a1")).toBe(true);
    expect(isHeard("a2")).toBe(false);
    resetHeardCache(); // recarrega do armazenamento
    expect(isHeard("a1")).toBe(true);
  });

  it("ignora armazenamento corrompido e id vazio", () => {
    vi.stubGlobal("localStorage", fakeStorage({ "chathm:heard-audios": "{não é json" }));
    resetHeardCache();
    expect(isHeard("a1")).toBe(false);
    markHeard("");
    expect(isHeard("")).toBe(false);
  });

  it("guarda no máximo 3000 e descarta os mais antigos", () => {
    const store = fakeStorage();
    vi.stubGlobal("localStorage", store);
    resetHeardCache();
    for (let i = 0; i < 3005; i++) markHeard(`m${i}`);
    const saved = JSON.parse(store.data["chathm:heard-audios"] ?? "[]") as string[];
    expect(saved).toHaveLength(3000);
    expect(saved).toContain("m3004");
    expect(saved).not.toContain("m0");
  });

  it("segue funcionando sem armazenamento", () => {
    vi.stubGlobal("localStorage", { getItem: () => { throw new Error("bloqueado"); }, setItem: () => { throw new Error("bloqueado"); } });
    resetHeardCache();
    markHeard("a1");
    expect(isHeard("a1")).toBe(true);
  });
});
