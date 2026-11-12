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

describe("MessageBubble with a kind this version does not know", () => {
  it("says to update the page instead of drawing an empty bubble, and still shows the time", () => {
    const html = bubble({ kind: "enquete", text: "Qual horário?" });
    expect(html).toContain("Mensagem de um tipo novo: atualize a página para ver");
    expect(html).toMatch(/\d{2}:\d{2}/);
    expect(html).not.toContain("Qual horário?"); // não tenta adivinhar o conteúdo
  });

  it("does not show it for the kinds it does know", () => {
    for (const m of [{ kind: "text", text: "Oi" }, { kind: "location", lat: 1, lng: 2 }, { kind: "conversation", sharedConversationId: "x" }, { kind: "unsupported", label: "Enquete" }]) {
      expect(bubble(m)).not.toContain("tipo novo");
    }
  });
});
