import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearNotices, dismissNotice, pushNotice } from "./notices";

beforeEach(async () => {
  vi.useFakeTimers();
  clearNotices();
});
afterEach(() => {
  clearNotices();
  vi.useRealTimers();
});

// Lê o estado pelo mesmo caminho do React (useSyncExternalStore), sem renderizar.
async function snapshot() {
  const { renderToStaticMarkup } = await import("react-dom/server");
  const { createElement } = await import("react");
  const { useNotices } = await import("./notices");
  function Probe() {
    return createElement("p", null, useNotices().map((n) => n.id).join(","));
  }
  return renderToStaticMarkup(createElement(Probe)).replace(/<\/?p>/g, "");
}

describe("notices", () => {
  it("shows a notice once, even if the same event arrives twice", async () => {
    pushNotice({ id: "a", text: "Um" });
    pushNotice({ id: "a", text: "Um" });
    pushNotice({ id: "b", text: "Dois" });
    expect(await snapshot()).toBe("a,b");
  });

  it("disappears by itself after a few seconds and can be closed earlier", async () => {
    pushNotice({ id: "a", text: "Um" });
    pushNotice({ id: "b", text: "Dois" });
    dismissNotice("a");
    expect(await snapshot()).toBe("b");
    vi.advanceTimersByTime(10_001);
    expect(await snapshot()).toBe("");
  });

  it("keeps only the latest few", async () => {
    for (const id of ["1", "2", "3", "4", "5", "6"]) pushNotice({ id, text: id });
    expect(await snapshot()).toBe("3,4,5,6");
  });

  it("keeps a sticky notice until the person acts, and tells only when it was closed with the ×", async () => {
    let closed = 0;
    pushNotice({ id: "v", text: "Nova versão", sticky: true, action: { label: "Atualizar", run: () => undefined }, onDismiss: () => { closed++; } });
    vi.advanceTimersByTime(60_000);
    expect(await snapshot()).toBe("v");
    dismissNotice("v"); // trocado ou limpo pelo app: não conta como "a pessoa fechou"
    expect(closed).toBe(0);
    pushNotice({ id: "v", text: "Nova versão", sticky: true, onDismiss: () => { closed++; } });
    dismissNotice("v", true);
    expect(closed).toBe(1);
    expect(await snapshot()).toBe("");
  });
});
