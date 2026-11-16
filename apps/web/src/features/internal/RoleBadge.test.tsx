import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RoleBadge } from "./RoleBadge";

describe("RoleBadge", () => {
  it("shows the administrator as a verified seal, with the role name on hover", () => {
    for (const onBubbleSurface of [true, false]) {
      const html = renderToStaticMarkup(<RoleBadge role="admin" onBubbleSurface={onBubbleSurface} />);
      expect(html).toContain('aria-label="Administrador"');
      expect(html).toContain('title="Administrador"');
      expect(html).toContain("<svg");
      expect(html).not.toContain(">Administrador<");
    }
  });

  it("keeps the text label for managers and agents", () => {
    expect(renderToStaticMarkup(<RoleBadge role="manager" />)).toContain(">Gestor<");
    expect(renderToStaticMarkup(<RoleBadge role="agent" onBubbleSurface />)).toContain(">Atendente<");
  });
});
