import { describe, expect, it } from "vitest";
import { shapeWave, speechWave } from "./waveform";

describe("forma de onda", () => {
  it("realça o contraste de uma onda comprimida como a do WhatsApp (0,2 a 1)", () => {
    const flat = Array.from({ length: 64 }, (_, i) => 0.55 + 0.1 * Math.sin(i)); // varia pouco
    const out = shapeWave(flat);
    expect(out).toHaveLength(40);
    expect(Math.max(...out) - Math.min(...out)).toBeGreaterThan(0.7);
    expect(Math.min(...out)).toBeGreaterThanOrEqual(0.1);
    expect(Math.max(...out)).toBeLessThanOrEqual(1);
  });

  it("um pico isolado não achata o resto", () => {
    const wave = Array.from({ length: 64 }, (_, i) => (i === 10 ? 1 : 0.2 + (i % 7) * 0.05));
    const out = shapeWave(wave);
    const tall = out.filter((v) => v > 0.5).length;
    expect(tall).toBeGreaterThan(5);
  });

  it("aceita escalas diferentes (0–100) e valores inválidos", () => {
    const out = shapeWave([0, 40, 100, 20, Number.NaN, 80, 5, 60]);
    expect(out.every((v) => v >= 0.1 && v <= 1)).toBe(true);
  });

  it("sem dados úteis cai numa onda de fala em vez de uma linha reta", () => {
    expect(new Set(shapeWave([])).size).toBeGreaterThan(5);
    expect(new Set(shapeWave(Array(64).fill(0.5))).size).toBeGreaterThan(5);
  });

  it("onda de fala é determinística, variada e tem pausas", () => {
    const a = speechWave("msg-1");
    expect(a).toEqual(speechWave("msg-1"));
    expect(a).not.toEqual(speechWave("msg-2"));
    expect(a).toHaveLength(40);
    expect(Math.max(...a) - Math.min(...a)).toBeGreaterThan(0.5);
    expect(a.every((v) => v >= 0.1 && v <= 1)).toBe(true);
  });
});
