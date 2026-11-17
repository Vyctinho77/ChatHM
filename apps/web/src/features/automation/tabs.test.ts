import { describe, expect, it } from "vitest";
import { currentTab, visibleTabs } from "./tabs";

const keys = (o?: Parameters<typeof visibleTabs>[0]) => visibleTabs(o).map(([k]) => k);

describe("abas de Atendimento por IA", () => {
  it("sem nada para aprovar nem acompanhar, só as abas fixas (e Cobranças não existe mais)", () => {
    expect(keys({ pending: 0, queued: 0, sentToday: 0, failed: 0 })).toEqual(["assuntos", "controles", "docs"]);
    expect(keys()).toEqual(["assuntos", "controles", "docs"]);
    expect(keys()).not.toContain("cobrancas");
  });

  it("pedidos aguardando aprovação mostram 'Para aprovar' e a fila", () => {
    expect(keys({ pending: 2, queued: 0, sentToday: 0, failed: 0 })).toEqual(["aprovar", "fila", "assuntos", "controles", "docs"]);
  });

  it("fila, envio de hoje ou falha mostram o acompanhamento, sem a aba de aprovar", () => {
    for (const o of [{ pending: 0, queued: 1, sentToday: 0, failed: 0 }, { pending: 0, queued: 0, sentToday: 3, failed: 0 }, { pending: 0, queued: 0, sentToday: 0, failed: 1 }]) {
      expect(keys(o)).toEqual(["fila", "assuntos", "controles", "docs"]);
    }
  });

  it("a aba escolhida que sumiu cai em Assuntos, ou em Para aprovar quando há pedidos", () => {
    const none = { pending: 0, queued: 0, sentToday: 0, failed: 0 };
    expect(currentTab(null, none)).toBe("assuntos");
    expect(currentTab("aprovar", none)).toBe("assuntos");
    expect(currentTab("fila", none)).toBe("assuntos");
    expect(currentTab("controles", none)).toBe("controles");
    expect(currentTab(null, { ...none, pending: 1 })).toBe("aprovar");
    expect(currentTab("docs", { ...none, pending: 1 })).toBe("docs");
  });
});
