import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RoleBadge } from "./RoleBadge";

describe("RoleBadge", () => {
  it("shows every role as a verified seal, with the role name on hover", () => {
    for (const [role, label] of [["admin", "Administrador"], ["manager", "Gestor"], ["agent", "Atendente"]] as const) {
      for (const onBubbleSurface of [true, false]) {
        const html = renderToStaticMarkup(<RoleBadge role={role} onBubbleSurface={onBubbleSurface} />);
        expect(html).toContain(`aria-label="${label}"`);
        expect(html).toContain(`title="${label}"`);
        expect(html).toContain("<svg");
        expect(html).not.toContain(`>${label}<`);
      }
    }
  });

  it("colors the seal by role: agent blue, manager brand green, admin gold", () => {
    expect(renderToStaticMarkup(<RoleBadge role="agent" />)).toContain("text-[#3b9cf0]");
    expect(renderToStaticMarkup(<RoleBadge role="manager" />)).toContain("text-brand");
    expect(renderToStaticMarkup(<RoleBadge role="admin" />)).toContain("text-[#e0aa1c]");
  });
});
