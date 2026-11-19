import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { applyPresence, presenceKey, type Presence } from "./presence";
import { applyRealtimeEvent } from "@/features/realtime/useRealtime";

describe("presença da equipe", () => {
  it("entrar e sair atualizam a lista, sem repetir e em ordem estável", () => {
    let p = applyPresence(undefined, { userId: "b", online: true });
    expect(p).toEqual({ online: ["b"] });
    p = applyPresence(p, { userId: "a", online: true });
    p = applyPresence(p, { userId: "a", online: true }); // repetido
    expect(p).toEqual({ online: ["a", "b"] });
    p = applyPresence(p, { userId: "b", online: false });
    expect(p).toEqual({ online: ["a"] });
    expect(applyPresence(p, { userId: "zzz", online: false })).toEqual({ online: ["a"] }); // quem nunca esteve
  });

  it("o evento em tempo real atualiza o que a tela lê", () => {
    const qc = new QueryClient();
    qc.setQueryData<Presence>(presenceKey, { online: ["ana"] });
    applyRealtimeEvent(qc, { type: "presence", data: { userId: "bia", online: true } });
    expect(qc.getQueryData<Presence>(presenceKey)).toEqual({ online: ["ana", "bia"] });
    applyRealtimeEvent(qc, { type: "presence", data: { userId: "ana", online: false } });
    expect(qc.getQueryData<Presence>(presenceKey)).toEqual({ online: ["bia"] });
    qc.clear();
  });

  it("um evento antes de carregar a lista já cria a lista", () => {
    const qc = new QueryClient();
    applyRealtimeEvent(qc, { type: "presence", data: { userId: "bia", online: true } });
    expect(qc.getQueryData<Presence>(presenceKey)).toEqual({ online: ["bia"] });
    qc.clear();
  });
});
