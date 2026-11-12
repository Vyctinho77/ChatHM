import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { InfoTip } from "./InfoTip";
import { Toggle } from "@/features/mcp/AiAccessCard";

describe("InfoTip", () => {
  it("mostra só o botão: a explicação fica fora da tela até a pessoa abrir", () => {
    const html = renderToStaticMarkup(<InfoTip title="Acesso por IA">Texto longo da explicação</InfoTip>);
    expect(html).toContain('aria-label="Saiba mais: Acesso por IA"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).not.toContain("Texto longo da explicação");
  });

  it("o interruptor mantém o nome acessível e leva o botão de ajuda fora do rótulo clicável", () => {
    const html = renderToStaticMarkup(<Toggle checked onChange={() => {}} label="Ocultar telefones" info={{ body: "Exibe os quatro últimos dígitos." }} />);
    expect(html).toContain('role="switch"');
    expect(html).toContain('aria-label="Ocultar telefones"');
    expect(html).toContain('aria-label="Saiba mais: Ocultar telefones"');
    expect(html).not.toContain("quatro últimos dígitos");
    // o botão "i" não está dentro de nenhum <label>, senão tocar nele ligaria/desligaria a opção
    expect(/<label[^>]*>(?:(?!<\/label>).)*<button/s.test(html)).toBe(false);
  });
});
