import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WebhookCard } from "./WebhookCard";

const base = { enabled: true, url: "https://hooks.exemplo.com.br/chathm", onlyAiReplies: true, includePreview: false, hasSecret: true, consecutiveFailures: 0, pending: 0 };

function render(webhook?: object, settings: object = { enabled: true }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, retryOnMount: false } } });
  if (webhook) qc.setQueryData(["ai", "webhook"], webhook);
  qc.setQueryData(["ai", "settings"], settings);
  const html = renderToStaticMarkup(<QueryClientProvider client={qc}><WebhookCard /></QueryClientProvider>);
  qc.clear();
  return html;
}

describe("Aviso automático (webhook)", () => {
  it("mantém a seção visível enquanto carrega", () => {
    const html = render();
    expect(html).toContain('id="aviso-automatico"');
  });

  it("mostra a configuração, o teste e a ajuda de assinatura, sem nunca mostrar um segredo", () => {
    const html = render({ ...base, lastDeliveryAt: "2026-11-12T12:00:00Z", lastStatus: "200", pending: 2 });
    expect(html).toContain("Aviso automático (webhook)");
    expect(html).toContain("https://hooks.exemplo.com.br/chathm");
    expect(html).toContain("Enviar teste");
    expect(html).toContain("Trocar segredo");
    expect(html).toContain("2 aguardando envio");
    expect(html).toContain("X-ChatHM-Signature");
    expect(html).not.toContain("whsec_");
  });

  it("sem segredo ainda não oferece teste nem troca; o texto da mensagem não vai por padrão", () => {
    const html = render({ ...base, enabled: false, url: "", hasSecret: false });
    expect(html).not.toContain("Enviar teste");
    expect(html).not.toContain("Trocar segredo");
    expect(html).toContain("Incluir um trecho da mensagem");
    expect(html).toContain("o texto da mensagem não vai");
  });

  it("avisa quando o acesso por IA está desligado e quando o webhook se desligou sozinho", () => {
    const html = render({ ...base, enabled: false, disabledReason: "Desligado sozinho depois de 20 avisos seguidos sem entrega." }, { enabled: false });
    expect(html).toContain("O acesso por IA está desligado");
    expect(html).toContain("Desligado sozinho depois de 20 avisos");
  });
});
