import { afterEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { clearNotices, pushNotice } from "@/lib/notices";
import { NoticeHost } from "./NoticeHost";

afterEach(() => clearNotices());
const render = () => renderToStaticMarkup(<MemoryRouter><NoticeHost /></MemoryRouter>);

describe("NoticeHost", () => {
  it("renders nothing without notices", () => {
    expect(render()).toBe("");
  });

  it("shows the new-version notice as one line with a single Update button and a close button", () => {
    pushNotice({ id: "app-version", sticky: true, text: "Há uma nova versão do ChatHM.", action: { label: "Atualizar", run: () => undefined } });
    const html = render();
    expect(html).toContain("Há uma nova versão do ChatHM.");
    expect(html.match(/>Atualizar</g)).toHaveLength(1);
    expect(html).toContain('aria-label="Fechar aviso"');
    expect(html).toContain('role="status"'); // anunciado por leitores de tela
    expect(html).not.toContain("<h"); // sem título nem texto extra
  });

  it("keeps the link style for notices that point to a conversation", () => {
    pushNotice({ id: "n1", text: "Ana passou o atendimento para você.", link: { to: "/conversas/c1", label: "Abrir" } });
    const html = render();
    expect(html).toContain('href="/conversas/c1"');
    expect(html).not.toContain("<button type=\"button\" class=\"inline-flex");
  });
});
