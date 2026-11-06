import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { meKey, type Me } from "@/features/auth/api";
import { ConversationList } from "./ConversationList";
import { conversationKeys } from "./store.api";
import type { Conversation } from "./types";

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const me: Me = { user: { id: "me", name: "Ana", email: "ana@example.test", role: "agent", theme: "dark", mfaEnabled: true }, csrfToken: "", mfaRequired: false, mfaSetupRequired: false };
  qc.setQueryData(meKey, me);
  const render = () => renderToStaticMarkup(<QueryClientProvider client={qc}><MemoryRouter><ConversationList onSelect={() => undefined} /></MemoryRouter></QueryClientProvider>);
  return { qc, render };
}

describe("initial page skeleton", () => {
  it("shows a skeleton until data arrives, without flashing an empty state", () => {
    const { qc, render } = setup();
    expect(render()).toContain('aria-label="Carregando conversas"');
    expect(render()).not.toContain("Nenhuma conversa encontrada.");
    qc.setQueryData(conversationKeys.list, []);
    expect(render()).not.toContain('aria-label="Carregando conversas"');
    expect(render()).toContain("Nenhuma conversa encontrada.");
    qc.clear();
  });

  it("preserves existing conversations while a background refresh runs", async () => {
    const { qc, render } = setup();
    const c: Conversation = { id: "chat", contact: { id: "contact", name: "Cliente já carregado", phone: "", note: "" }, segment: "geral", status: "new", tags: [], unreadCount: 0 };
    qc.setQueryData(conversationKeys.list, [c]);
    let complete!: (data: Conversation[]) => void;
    const refresh = qc.fetchQuery({ queryKey: conversationKeys.list, queryFn: () => new Promise<Conversation[]>((resolve) => { complete = resolve; }) });
    expect(qc.getQueryState(conversationKeys.list)?.fetchStatus).toBe("fetching");
    const html = render();
    expect(html).toContain("Cliente já carregado");
    expect(html).not.toContain('aria-label="Carregando conversas"');
    complete([c]);
    await refresh;
    qc.clear();
  });
});
