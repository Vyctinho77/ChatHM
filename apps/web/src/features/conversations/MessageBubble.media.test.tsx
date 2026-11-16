import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { MessageBubble } from "./MessageBubble";
import type { Message } from "./types";

const bubble = (m: Record<string, unknown>) =>
  renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <MessageBubble m={{ id: "m1", conversationId: "c", direction: "in", sentAt: "2026-11-12T13:00:00Z", ...m } as Message} showAuthor={false} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("mídia sem arquivo: o que a tela diz", () => {
  it("histórico sem arquivo continua dizendo que não foi importado", () => {
    expect(bubble({ kind: "audio", durationSec: 5, historical: true })).toContain("Áudio do histórico · arquivo não importado");
    expect(bubble({ kind: "image", historical: true })).toContain("Imagem do histórico · arquivo não importado");
  });

  it("avisa quando está pedindo o arquivo ao celular e quando ele não está mais disponível", () => {
    for (const m of [{ kind: "audio", durationSec: 5 }, { kind: "image" }, { kind: "document", fileName: "guia.pdf", mimeType: "application/pdf" }]) {
      expect(bubble({ ...m, historical: true, mediaState: "retrying" })).toContain("pedindo o arquivo ao celular");
      expect(bubble({ ...m, historical: true, mediaState: "expired" })).toContain("não está mais disponível");
    }
  });
});
