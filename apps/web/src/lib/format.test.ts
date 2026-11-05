import { describe, expect, it } from "vitest";
import { formatBytes, formatDayDivider, formatDuration, formatListTime, initials } from "./format";

describe("format", () => {
  it("initials", () => {
    expect(initials("Rafael Moreira")).toBe("RM");
    expect(initials("ana")).toBe("A");
    expect(initials("Luciana Prado de Souza")).toBe("LS");
    expect(initials("+55 48 9210-1900")).toBe("#");
  });

  it("formatDuration", () => {
    expect(formatDuration(8)).toBe("0:08");
    expect(formatDuration(75.4)).toBe("1:15");
  });

  it("formatBytes", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(5_200)).toBe("5 KB");
    expect(formatBytes(2_100_000)).toBe("2,0 MB");
  });

  it("datas relativas", () => {
    const now = new Date(2026, 10, 5, 15, 0);
    expect(formatListTime(new Date(2026, 10, 5, 8, 51).toISOString(), now)).toBe("08:51");
    expect(formatListTime(new Date(2026, 10, 4, 20, 0).toISOString(), now)).toBe("Ontem");
    expect(formatDayDivider(new Date(2026, 10, 5, 1, 0).toISOString(), now)).toBe("Hoje");
    expect(formatDayDivider(new Date(2026, 9, 28, 10, 0).toISOString(), now)).toBe("28/10/2026");
  });
});
