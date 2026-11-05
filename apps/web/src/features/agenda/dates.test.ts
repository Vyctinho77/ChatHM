import { describe, expect, it } from "vitest";
import { addDays, addMonths, fromISO, monthGrid, sameMonth, toISO } from "./dates";

describe("datas da agenda", () => {
  it("ida e volta de ISO sem deslocar o dia", () => {
    expect(toISO(fromISO("2026-11-13"))).toBe("2026-11-13");
    expect(toISO(fromISO("2026-01-01"))).toBe("2026-01-01");
    expect(toISO(fromISO("2026-12-31"))).toBe("2026-12-31");
  });

  it("monthGrid começa no domingo e cobre o mês inteiro", () => {
    for (const [y, m] of [[2026, 10], [2026, 1], [2026, 0], [2027, 1], [2028, 1], [2026, 4]] as const) {
      const days = monthGrid(new Date(y, m, 1));
      expect(days.length % 7).toBe(0);
      expect(days.length).toBeGreaterThanOrEqual(28);
      expect(days.length).toBeLessThanOrEqual(42);
      expect(days[0]!.getDay()).toBe(0);
      const inMonth = days.filter((d) => sameMonth(d, new Date(y, m, 1))).length;
      expect(inMonth).toBe(new Date(y, m + 1, 0).getDate());
      // dias consecutivos, sem buracos nem repetição
      days.slice(1).forEach((d, i) => expect(toISO(d)).toBe(toISO(addDays(days[i]!, 1))));
    }
  });

  it("número de semanas: novembro/2026 (começa no domingo) tem 5; fevereiro/2026 tem 4; agosto/2026 tem 6", () => {
    expect(monthGrid(new Date(2026, 10, 1)).length).toBe(35);
    expect(monthGrid(new Date(2026, 1, 1)).length).toBe(28);
    expect(monthGrid(new Date(2026, 7, 1)).length).toBe(42);
  });

  it("addMonths atravessa o ano", () => {
    expect(toISO(addMonths(new Date(2026, 11, 15), 1))).toBe("2027-01-01");
    expect(toISO(addMonths(new Date(2026, 0, 31), -1))).toBe("2025-12-01");
  });
});
