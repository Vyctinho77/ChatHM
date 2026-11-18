import { describe, expect, it } from "vitest";
import { draftTextError, isOldDraft, laneState, laneSummary, MAX_DRAFT_CHARS, needsLiveConfirm, OUTCOME_LABEL, parseDomains, reasonLabel, validateLane, type Lane } from "./lanes";

const lane = (o: Partial<Lane> = {}): Lane => ({
  key: "certificado", label: "Certificado digital", enabled: false, mode: "shadow", canInitiate: false, segmentId: "", handoffSegmentId: "",
  maxTurns: 12, maxConsecutive: 3, contactDaily: 10, humanQuietMinutes: 60, staleHours: 24, requireSources: true, allowedDomains: [], guidelines: "", updatedAt: "", ...o,
});

describe("raias: estado e confirmação", () => {
  it("desligada, em sombra ou valendo", () => {
    expect(laneState(lane())).toBe("off");
    expect(laneState(lane({ enabled: true }))).toBe("shadow");
    expect(laneState(lane({ enabled: true, mode: "live" }))).toBe("live");
    expect(laneState(lane({ enabled: false, mode: "live" }))).toBe("off");
  });
  it("só pede confirmação ao passar a valer de verdade", () => {
    const off = lane(), shadow = lane({ enabled: true }), live = lane({ enabled: true, mode: "live" });
    expect(needsLiveConfirm(off, live)).toBe(true);
    expect(needsLiveConfirm(shadow, live)).toBe(true);
    expect(needsLiveConfirm(live, live)).toBe(false);
    expect(needsLiveConfirm(live, shadow)).toBe(false);
    expect(needsLiveConfirm(off, shadow)).toBe(false);
  });
});

describe("raias: formulário", () => {
  it("domínios: separa por vírgula, espaço ou linha, em minúsculas e sem repetir", () => {
    expect(parseDomains("GOV.br, asaas.com\nasaas.com; iti.gov.br  ")).toEqual(["gov.br", "asaas.com", "iti.gov.br"]);
    expect(parseDomains("")).toEqual([]);
  });
  it("a raia de fábrica é válida", () => expect(validateLane(lane())).toEqual({}));
  it("confere os mesmos limites do servidor", () => {
    const e = validateLane(lane({ label: " ", maxTurns: 0, maxConsecutive: 11, contactDaily: 51, humanQuietMinutes: 1441, staleHours: 721, allowedDomains: ["https://x.com/y"] }));
    expect(Object.keys(e).sort()).toEqual(["allowedDomains", "contactDaily", "humanQuietMinutes", "label", "maxConsecutive", "maxTurns", "staleHours"]);
    expect(validateLane(lane({ maxTurns: 1.5 })).maxTurns).toBeTruthy();
    expect(validateLane(lane({ staleHours: 0, humanQuietMinutes: 0 }))).toEqual({});
  });
  it("resume o que a raia faz", () => {
    expect(laneSummary(lane(), "Certificado")).toBe("Desligada · só responde · fila Certificado · conversas paradas há 24 h");
    expect(laneSummary(lane({ enabled: true, mode: "live", canInitiate: true, staleHours: 0 }))).toBe("Valendo · inicia conversas · qualquer fila");
  });
  it("motivos de passagem para humano em português", () => {
    expect(reasonLabel("pediu_humano")).toBe("Cliente pediu uma pessoa ou reclamou");
    expect(reasonLabel("licenca_especial")).toBe("Licenca especial");
    expect(reasonLabel("")).toBe("Outro motivo");
  });
});

describe("rascunhos da sombra", () => {
  it("avisa quando o rascunho é antigo", () => {
    const now = Date.parse("2026-11-18T12:00:00Z");
    expect(isOldDraft("2026-11-18T10:00:00Z", now)).toBe(false);
    expect(isOldDraft("2026-11-17T11:59:00Z", now)).toBe(true);
  });
  it("confere o texto editado", () => {
    expect(draftTextError("  ")).toBeTruthy();
    expect(draftTextError("Olá!")).toBe("");
    expect(draftTextError("x".repeat(MAX_DRAFT_CHARS))).toBe("");
    expect(draftTextError("x".repeat(MAX_DRAFT_CHARS + 1))).toContain("1500");
  });
  it("descreve o que aconteceu com cada rascunho", () => {
    expect(OUTCOME_LABEL.approved).toBe("Enviado como a IA escreveu");
    expect(OUTCOME_LABEL.edited).toBe("Enviado com edição");
    expect(OUTCOME_LABEL.discarded).toBe("Descartado");
  });
});
