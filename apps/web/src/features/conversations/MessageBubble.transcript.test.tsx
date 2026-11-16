import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { MessageBubble } from "./MessageBubble";
import type { Message } from "./types";

const voice = (extra: Record<string, unknown>) =>
  renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <MessageBubble m={{ id: "a1", conversationId: "c", direction: "in", sentAt: "2026-11-12T13:00:00Z", kind: "audio", durationSec: 93, url: "/api/v1/attachments/x", ...extra } as Message} showAuthor={false} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("transcrição da mensagem de voz", () => {
  it("mostra o texto falado sob o áudio", () => {
    const html = voice({ transcript: "Oi, já paguei o boleto.", transcriptState: "done" });
    expect(html).toContain("Transcrição");
    expect(html).toContain("Oi, já paguei o boleto.");
  });

  it("avisa enquanto transcreve e quando o áudio é longo demais, sem inventar texto", () => {
    expect(voice({ transcriptState: "pending" })).toContain("Transcrevendo…");
    expect(voice({ transcriptState: "toolong" })).toContain("longo demais para transcrever");
    for (const state of ["failed", "empty", undefined]) {
      const html = voice({ transcriptState: state });
      expect(html).not.toContain("Transcrição");
      expect(html).not.toContain("Transcrevendo");
    }
  });
});
