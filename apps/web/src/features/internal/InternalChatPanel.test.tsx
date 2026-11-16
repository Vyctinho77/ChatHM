import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api";
import { conversationKeys } from "@/features/conversations/store.api";
import type { Conversation } from "@/features/conversations/types";
import { ShareConversationDialog } from "./ShareConversationDialog";
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

function seeded(unread = 0, messages: Page["messages"] = [], mentions = 0) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(meKey, me);
  qc.setQueryData<InternalRoom>(roomKey(ME), { id: "room", name: "Escritório", unread, mentions, lastReadSeq: 0, lastSeq: messages.length });
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

describe("mentions and shared conversations in the panel", () => {
  const mentioning = [
    fromServer(raw("1", "caio", "Caio", "agent", "@Victor e @Bia, vejam o caso.", 1, "2026-11-12T13:00:00Z", { mentions: [{ id: ME, name: "Victor" }, { id: "bia", name: "Bia" }] }), ME),
    fromServer(raw("2", ME, "Victor", "admin", "Já vi, @Caio.", 2, "2026-11-12T13:01:00Z", { mentions: [{ id: "caio", name: "Caio" }] }), ME),
  ];

  it("highlights each @mention, with more emphasis on the ones about me", () => {
    const html = wrap(seeded(1, mentioning, 1), <InternalChatPanel />);
    expect(html).toMatch(/bg-amber-300\/70[^>]*>@Victor</); // sobre mim
    expect(html).toMatch(/bg-\[#5f6e24\]\/15[^>]*>@Bia</); // sobre outra pessoa
    expect(html).toMatch(/bg-white\/20[^>]*>@Caio</); // na minha mensagem (balão verde)
    expect(html).not.toContain(">@Victor e @Bia");
  });

  it("offers to share a customer conversation and to mute the sounds", () => {
    const html = wrap(seeded(), <InternalChatPanel />);
    expect(html).toContain("Silenciar os sons do chat interno");
  });

  const card = (id = "m1", extra: Record<string, unknown> = {}) =>
    fromServer(raw(id, "caio", "Caio", "agent", "", 1, "2026-11-12T13:00:00Z", { kind: "conversation", sharedConversationId: "conv-1", caption: "Alguém viu algo parecido?", text: undefined, ...extra }), ME);

  it("shows the customer on the card for someone who can see the conversation, with a link to open it", () => {
    const qc = seeded(0, [card()]);
    qc.setQueryData(["internal-chat", "shared", ME, "conv-1"], { id: "conv-1", status: "in_progress", assignee: "Ana", contact: { name: "Mercado Silva", phone: "5548999990000", saved: true, kind: "person" } });
    const html = wrap(qc, <InternalChatPanel />);
    expect(html).toContain("Mercado Silva");
    expect(html).toContain("Em atendimento por Ana");
    expect(html).toContain('href="/conversas/conv-1"');
    expect(html).toContain("Alguém viu algo parecido?");
  });

  it("shows no customer data for someone without access, and no link", () => {
    const qc = seeded(0, [card()]);
    // O app tenta de novo ao montar uma consulta que deu erro (para rechecar o acesso); aqui o estado semeado precisa valer.
    qc.setDefaultOptions({ queries: { retry: false, retryOnMount: false } });
    qc.getQueryCache().build(qc, { queryKey: ["internal-chat", "shared", ME, "conv-1"] }).setState({ status: "error", error: new ApiError(404, "not_found", "x"), data: undefined });
    const html = wrap(qc, <InternalChatPanel />);
    expect(html).toContain("Conversa de um cliente");
    expect(html).toContain("Você não tem acesso a esta conversa.");
    expect(html).not.toContain("/conversas/conv-1");
    expect(html).not.toContain("Mercado");
  });

  it("does not offer to open a closed conversation", () => {
    const qc = seeded(0, [card()]);
    qc.setQueryData(["internal-chat", "shared", ME, "conv-1"], { id: "conv-1", status: "closed", contact: { name: "Mercado Silva", phone: "", saved: true, kind: "person" } });
    const html = wrap(qc, <InternalChatPanel />);
    expect(html).toContain("Atendimento encerrado");
    expect(html).not.toContain("Abrir conversa");
  });

  it("uses the phone as the card title for an unsaved contact, like the conversation list", () => {
    const qc = seeded(0, [card()]);
    qc.setQueryData(["internal-chat", "shared", ME, "conv-1"], { id: "conv-1", status: "new", contact: { name: "Mercado Silva", phone: "+55 48 99999-0000", saved: false, kind: "person" } });
    expect(wrap(qc, <InternalChatPanel />)).toContain("+55 48 99999-0000");
  });
});

describe("ShareConversationDialog", () => {
  const conv = (id: string, name: string, over: Partial<Conversation> = {}): Conversation => ({ id, contact: { id: "c" + id, name, phone: "5548", note: "", saved: true, kind: "person" }, segment: "geral", status: "in_progress", tags: [], unreadCount: 0, ...over });

  it("lists the conversations the person can see and starts with nothing chosen", () => {
    const qc = seeded();
    qc.setQueryData(conversationKeys.list, [conv("1", "Mercado Silva", { assignee: { id: "ana", name: "Ana" } }), conv("2", "Clínica Sol", { status: "new" }), conv("3", "Padaria Doce", { status: "closed" })]);
    const html = wrap(qc, <ShareConversationDialog onShare={() => undefined} onClose={() => undefined} />);
    for (const text of ["Mercado Silva", "Clínica Sol", "Padaria Doce", "Com Ana", "Aguardando atendimento", "Encerrada", "Comentário (opcional)"]) expect(html).toContain(text);
    expect(html).toContain("só para quem já tem acesso");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Compartilhar</);
  });

  it("shows the name next to the phone when the contact is not saved, so the conversation is recognisable", () => {
    const qc = seeded();
    qc.setQueryData(conversationKeys.list, [conv("1", "Mercado Silva", { contact: { id: "c1", name: "Mercado Silva", phone: "+55 48 99999-1111", note: "", saved: false, kind: "person" } })]);
    const html = wrap(qc, <ShareConversationDialog onShare={() => undefined} onClose={() => undefined} />);
    expect(html).toContain("+55 48 99999-1111");
    expect(html).toContain("Mercado Silva · ");
  });

  it("says when there is nothing to share", () => {
    const qc = seeded();
    qc.setQueryData(conversationKeys.list, []);
    expect(wrap(qc, <ShareConversationDialog onShare={() => undefined} onClose={() => undefined} />)).toContain("Nenhuma conversa encontrada.");
  });
});

describe("Sidebar mention badge", () => {
  it("marks the tab when someone mentioned me, and says so to screen readers", () => {
    const withMention = wrap(seeded(4, [], 2), <Sidebar me={me} />);
    expect(withMention).toContain("Chat interno: 4 mensagens não lidas, 2 mencionando você");
    expect(withMention).toContain(">@<");
    const plain = wrap(seeded(4, [], 0), <Sidebar me={me} />);
    expect(plain).toContain("Chat interno: 4 mensagens não lidas");
    expect(plain).not.toContain("mencionando você");
    expect(plain).not.toContain(">@<");
  });
});
