import { describe, expect, it } from "vitest";
import { bundleFromHtml, runningBundle, shouldAnnounce } from "./versionCheck";

const html = (hash: string) => `<!doctype html><html><head><script type="module" crossorigin src="/assets/index-${hash}.js"></script><link rel="stylesheet" href="/assets/index-AbC123.css"></head><body></body></html>`;

describe("bundleFromHtml", () => {
  it("finds the main bundle of an index.html, ignoring the stylesheet", () => {
    expect(bundleFromHtml(html("C-hp7yv1"))).toBe("/assets/index-C-hp7yv1.js");
    expect(bundleFromHtml(html("ByIO4_EC"))).toBe("/assets/index-ByIO4_EC.js");
  });

  it("returns null when there is no bundle (an error page, an empty response)", () => {
    expect(bundleFromHtml("<html><body>502 Bad Gateway</body></html>")).toBeNull();
    expect(bundleFromHtml("")).toBeNull();
  });
});

describe("runningBundle", () => {
  const doc = (srcs: string[]) => ({ querySelectorAll: () => srcs.map((src) => ({ getAttribute: () => src })) as unknown as NodeListOf<HTMLScriptElement> });

  it("reads the bundle this tab loaded", () => {
    expect(runningBundle(doc(["/assets/pdf-YwugTw7Y.js", "/assets/index-C-hp7yv1.js"]))).toBe("/assets/index-C-hp7yv1.js");
  });

  it("is null in development, where scripts have no hash", () => {
    expect(runningBundle(doc(["/src/main.tsx", "/@vite/client"]))).toBeNull();
    expect(runningBundle(doc([]))).toBeNull();
  });
});

describe("shouldAnnounce", () => {
  const A = "/assets/index-AAAA.js";
  const B = "/assets/index-BBBB.js";
  const C = "/assets/index-CCCC.js";

  it("announces only when the server has a different bundle", () => {
    expect(shouldAnnounce(A, B, null)).toBe(true);
    expect(shouldAnnounce(A, A, null)).toBe(false);
  });

  it("stays quiet when it cannot tell (development, failed check)", () => {
    expect(shouldAnnounce(null, B, null)).toBe(false);
    expect(shouldAnnounce(A, null, null)).toBe(false);
  });

  it("respects a dismissal for that version, but announces a newer one", () => {
    expect(shouldAnnounce(A, B, B)).toBe(false);
    expect(shouldAnnounce(A, C, B)).toBe(true);
  });
});
