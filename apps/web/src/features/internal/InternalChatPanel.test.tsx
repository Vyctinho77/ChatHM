import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { meKey, type Me } from "@/features/auth/api";
import { Sidebar } from "@/features/shell/Sidebar";
import { fromServer, messagesKey, roomKey, type InternalRoom, type Page } from "./api";
import { InternalChatPanel } from "./InternalChatPanel";

const ME = "me";
const me: Me = { user: { id: ME, name: "Victor", email: "v@example.test", role: "admin", theme: "dark", mfaEnabled: true }, csrfToken: "", mfaRequired: false, mfaSetupRequired: false };
const raw = (id: string, authorId: string, authorName: string, authorRole: string, text: string, seq: number, sentAt: string, extra: Record<string, unknown> = {}) =>
  ({ id, conversationId: "room", seq, kind: "text", text, sentAt, authorId, authorName, authorRole, authorActive: true, ...extra });

function seeded(unread = 0, messages: Page["messages"] = []) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(meKey, me);
  qc.setQueryData<InternalRoom>(roomKey(ME), { id: "room", name: "Escritório", unread, lastReadSeq: 0, lastSeq: messages.length });
  qc.setQueryData<Page>(messagesKey(ME), { messages, hasMore: false });
  return qc;
}
const wrap = (qc: QueryClient, el: React.ReactElement) => renderToStaticMarkup(<QueryClientProvider client={qc}><MemoryRouter>{el}</MemoryRouter></QueryClientProvider>);

describe("InternalChatPanel", () => {
  const conversation = [
    fromServer(raw("1", "bia", "Bia", "manager", "Alguém sabe o prazo da DCTF?", 1, "2026-11-12T13:00:00Z"), ME),
    fromServer(raw("2", "bia", "Bia", "manager", "Preciso para hoje.", 2, "2026-11-12T13:01:00Z"), ME),
    fromServer(raw("3", "caio", "Caio", "agent", "Dia 15.", 3, "2026-11-12T13:02:00Z", { quoted: { id: "1", authorId: "bia", author: "Bia", kind: "text", text: "Alguém sabe o prazo da DCTF?" } }), ME),
    fromServer(raw("4", ME, "Victor", "admin", "Obrigado, Caio!", 4, "2026-11-12T13:03:00Z"), ME),
    fromServer(raw("5", "caio", "Caio", "agent", "", 5, "2026-11-12T13:04:00Z", { deleted: true }), ME),
  ];

  it("shows who wrote each message with their role, only on the first of a sequence", () => {
    const html = wrap(seeded(0, conversation), <InternalChatPanel />);
    expect(html).toContain("Chat interno");
    expect(html).toContain("Alguém sabe o prazo da DCTF?");
    expect(html).toContain("Gestor");
    expect(html).toContain("Atendente");
    expect(html).toContain("Obrigado, Caio!");
    expect(html).toContain("Esta mensagem foi apagada");
    // Duas mensagens seguidas da mesma pessoa: o nome dela aparece uma vez só.
    const twoInARow = wrap(seeded(0, conversation.slice(0, 2)), <InternalChatPanel />);
    expect(twoInARow.match(/>Bia</g)).toHaveLength(1);
    expect(twoInARow).toContain("Preciso para hoje.");
  });

  it("shows the quoted message with its author", () => {
    const html = wrap(seeded(0, conversation), <InternalChatPanel />);
    expect(html).toContain("Dia 15.");
    expect(html.match(/>Bia</g)).toHaveLength(2); // o nome na primeira mensagem e o da citação na resposta do Caio
  });

  it("never says \"Você apagou\" for a message someone else (a manager) may have deleted", () => {
    const mineDeleted = [fromServer(raw("9", ME, "Victor", "admin", "", 9, "2026-11-12T13:10:00Z", { deleted: true }), ME)];
    const html = wrap(seeded(0, mineDeleted), <InternalChatPanel />);
    expect(html).toContain("Esta mensagem foi apagada");
    expect(html).not.toContain("Você apagou");
  });

  it("draws my own messages on the right, without delivery ticks", () => {
    const html = wrap(seeded(0, conversation), <InternalChatPanel />);
    expect(html).toContain("justify-end");
    expect(html).not.toContain("Entregue");
    expect(html).not.toContain("Lida");
  });

  it("shows a welcome when the room is empty", () => {
    const html = wrap(seeded(), <InternalChatPanel />);
    expect(html).toContain("Nenhuma mensagem ainda");
  });

  it("shows the back button only on small screens (when given)", () => {
    const qc = seeded();
    expect(wrap(qc, <InternalChatPanel />)).not.toContain("Voltar para a lista");
    expect(wrap(qc, <InternalChatPanel onBack={() => undefined} />)).toContain("Voltar para a lista");
  });
});

describe("Sidebar entry", () => {
  const side = (unread: number) => wrap(seeded(unread), <Sidebar me={me} />);

  it("lists the internal chat below Envios automáticos for everyone", () => {
    const html = side(0);
    expect(html.indexOf("Envios automáticos")).toBeGreaterThan(-1);
    expect(html.indexOf("Chat interno")).toBeGreaterThan(html.indexOf("Envios automáticos"));
    const agent = renderToStaticMarkup(<QueryClientProvider client={seeded()}><MemoryRouter><Sidebar me={{ ...me, user: { ...me.user, role: "agent" } }} /></MemoryRouter></QueryClientProvider>);
    expect(agent).toContain("Chat interno");
    expect(agent).not.toContain("Envios automáticos"); // só administradores
  });

  it("shows the unread badge and names it for screen readers", () => {
    const html = side(3);
    expect(html).toContain("Chat interno: 3 mensagens não lidas");
    expect(side(1)).toContain("Chat interno: 1 mensagem não lida");
    expect(side(0)).not.toContain("não lida");
    expect(side(150)).toContain("99+");
  });
});
