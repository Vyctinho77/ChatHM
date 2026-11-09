import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AiAccessCard } from "./AiAccessCard";

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false } } });
  const render = () => renderToStaticMarkup(<QueryClientProvider client={qc}><AiAccessCard /></QueryClientProvider>);
  return { qc, render };
}

describe("MCP settings visibility", () => {
  it("keeps the section visible while loading", () => {
    const { qc, render } = setup();
    const html = render();
    expect(html).toContain('id="integracoes-ia"');
    expect(html).toContain('aria-label="Carregando integrações MCP"');
    qc.clear();
  });

  it("shows a retry action instead of disappearing when the settings request fails", async () => {
    const { qc, render } = setup();
    await qc.fetchQuery({ queryKey: ["ai", "settings"], queryFn: async () => { throw new Error("API indisponível"); } }).catch(() => undefined);
    const html = render();
    expect(html).toContain("Não foi possível carregar as integrações.");
    expect(html).toContain("Tentar novamente");
    qc.clear();
  });
});
