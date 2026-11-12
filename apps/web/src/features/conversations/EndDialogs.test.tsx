import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { CloseAllDialog, LeaveDialog } from "./EndDialogs";
import type { Conversation } from "./types";

const P = (id: string, name: string) => ({ id, name });
const base: Conversation = { id: "c1", contact: { id: "ct", name: "Mercado Silva", phone: "", note: "" }, segment: "geral", status: "in_progress", tags: [], unreadCount: 0 };
const noop = async () => undefined;
const leave = (c: Conversation, hasReminder = false) => renderToStaticMarkup(<LeaveDialog conversation={c} myId="me" hasReminder={hasReminder} onConfirm={noop} onClose={() => undefined} />);

describe("LeaveDialog", () => {
  it("lets the responsible choose who continues, with the oldest participant already chosen", () => {
    const html = leave({ ...base, assignee: P("me", "Eu"), participants: [P("bia", "Beatriz Dois"), P("caio", "Carlos Tres")] });
    expect(html).toContain("Quem continua como responsável?");
    expect(html).toContain('role="radiogroup"');
    expect(html).toContain("Beatriz Dois");
    expect(html).toContain("Carlos Tres");
    expect(html.match(/checked=""/g)).toHaveLength(1);
    expect(html.indexOf('checked=""')).toBeLessThan(html.indexOf("Carlos Tres")); // a primeira vem marcada
    expect(html).toContain("Encerrar minha parte");
    expect(html).not.toContain("lembrete");
  });

  it("offers to take the personal reminder along only when there is one", () => {
    const c = { ...base, assignee: P("me", "Eu"), participants: [P("bia", "Beatriz Dois")] };
    expect(leave(c, true)).toContain("Levar o meu lembrete para Beatriz Dois");
    expect(leave(c, false)).not.toContain("Levar o meu lembrete");
  });

  it("tells a participant who stays, without asking anything", () => {
    const html = leave({ ...base, assignee: P("ana", "Ana"), participants: [P("me", "Eu"), P("bia", "Beatriz Dois")] });
    expect(html).toContain("Você sai deste atendimento. Ele continua com");
    expect(html).toContain("Ana e Beatriz Dois");
    expect(html).not.toContain("radiogroup");
  });

  it("says the atendimento goes back to the queue when nobody stays", () => {
    expect(leave({ ...base, participants: [P("me", "Eu")] })).toContain("volta à fila");
  });
});

describe("CloseAllDialog", () => {
  const render = (others: { id: string; name: string }[], canLeave = false) => renderToStaticMarkup(<CloseAllDialog others={others} canLeave={canLeave} onConfirm={noop} onClose={() => undefined} />);

  it("names who is still in the atendimento and what happens to the customer", () => {
    const html = render([P("ana", "Ana")]);
    expect(html).toContain("Encerrar para todos?");
    expect(html).toContain("Ana");
    expect(html).toContain("está neste atendimento");
    expect(html).toContain("a conversa sai da lista dela");
    expect(html).toContain("o cliente que escrever depois volta para a fila");
  });

  it("uses the plural for more people and reminds about the lighter option only to those who can use it", () => {
    const html = render([P("ana", "Ana"), P("bia", "Beatriz"), P("caio", "Carlos")], true);
    expect(html).toContain("Ana, Beatriz e Carlos");
    expect(html).toContain("estão neste atendimento");
    expect(html).toContain("a conversa sai da lista delas");
    expect(html).toContain("Encerrar minha parte");
    expect(render([P("ana", "Ana")], false)).not.toContain("Encerrar minha parte");
  });
});
