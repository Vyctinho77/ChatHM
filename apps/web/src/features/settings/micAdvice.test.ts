import { describe, expect, it } from "vitest";
import { formatKHz, micAdvice, type MicReport } from "./micAdvice";

const ok: MicReport = { label: "Microfone interno", sampleRate: 48_000, channels: 1, noiseSuppression: false, echoCancellation: false, peak: 0.4 };

describe("micAdvice", () => {
  it("sem problema e sem processamento ligado, não avisa nada", () => {
    expect(micAdvice(ok)).toEqual([]);
  });

  it("taxa baixa (típica de fone Bluetooth em chamada) explica a voz oca e orienta o fone", () => {
    const a = micAdvice({ ...ok, label: "Headset Hands-Free (Bluetooth)", sampleRate: 16_000 });
    expect(a).toHaveLength(1);
    expect(a[0]!.tone).toBe("warn");
    expect(a[0]!.text).toContain("16 kHz");
    expect(a[0]!.text).toContain("Bluetooth");
  });

  it("taxa baixa em microfone comum não cita Bluetooth", () => {
    expect(micAdvice({ ...ok, label: "USB Mic", sampleRate: 8_000 })[0]!.text).not.toContain("Bluetooth");
  });

  it("avisa quando quase não captou som e quando estourou", () => {
    expect(micAdvice({ ...ok, peak: 0.001 })[0]!.text).toContain("Quase nenhum som");
    expect(micAdvice({ ...ok, peak: 0.99 })[0]!.text).toContain("estourando");
  });

  it("com redução de ruído ligada e nada errado, sugere desligar para comparar", () => {
    const a = micAdvice({ ...ok, noiseSuppression: true });
    expect(a).toHaveLength(1);
    expect(a[0]!.tone).toBe("info");
  });

  it("não sugere desligar quando já há um problema mais sério", () => {
    expect(micAdvice({ ...ok, noiseSuppression: true, sampleRate: 16_000 }).every((x) => x.tone === "warn")).toBe(true);
  });

  it("formata a taxa", () => {
    expect(formatKHz(48_000)).toBe("48 kHz");
    expect(formatKHz(44_100)).toBe("44,1 kHz");
    expect(formatKHz(16_000)).toBe("16 kHz");
  });
});
