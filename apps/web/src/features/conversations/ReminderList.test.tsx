import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { meKey, type Me } from "@/features/auth/api";
import { ConversationList } from "./ConversationList";
import { conversationKeys } from "./store.api";
import { reminderKey, type Reminder } from "./reminders";
import type { Conversation } from "./types";

function renderList(filter: string, userId = "me") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const me: Me = { user: { id: userId, name: "Ana", email: "ana@example.test", role: "agent", theme: "dark", mfaEnabled: true }, csrfToken: "", mfaRequired: false, mfaSetupRequired: false };
  const c: Conversation = { id: "chat", contact: { id: "contact", name: "Cliente", phone: "", note: "" }, segment: "geral", status: "in_progress", assignee: { id: userId, name: "Ana" }, tags: [], unreadCount: 0 };
  qc.setQueryData(meKey, me);
  qc.setQueryData(conversationKeys.list, [c]);
  qc.setQueryData<Record<string, Reminder>>([...reminderKey, "me"], { chat: { conversationId: "chat", text: "Aguardando documentos fiscais", color: "amber" } });
  qc.setQueryData<Record<string, Reminder>>([...reminderKey, "other"], {});
  const html = renderToStaticMarkup(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[`/conversas?vista=${filter}`]}><ConversationList onSelect={() => undefined} /></MemoryRouter></QueryClientProvider>);
  qc.clear();
  return html;
}

describe("personal reminder in the conversation list", () => {
  it("shows the saved reminder below the preview only in Mine", () => {
    const html = renderList("mine");
    expect(html).toContain("Aguardando documentos fiscais");
    expect(html).toContain("Editar lembrete de Cliente");
    expect(html.indexOf("Aguardando documentos fiscais")).toBeGreaterThan(html.indexOf("Nova conversa"));
    expect(html).toContain("text-amber-800");
    expect(renderList("all")).not.toContain("Aguardando documentos fiscais");
  });

  it("does not reuse another account's cached reminder", () => {
    const html = renderList("mine", "other");
    expect(html).not.toContain("Aguardando documentos fiscais");
    expect(html).toContain("Adicionar lembrete de Cliente");
  });
});
