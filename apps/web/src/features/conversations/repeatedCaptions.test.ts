import { describe, expect, it } from "vitest";
import { repeatedCaptionIds, SAME_BATCH_MS } from "./repeatedCaptions";
import type { Message } from "./types";

const at = (sec: number) => new Date(Date.UTC(2026, 10, 17, 14, 35, 0) + sec * 1000).toISOString();
const doc = (id: string, sec: number, caption: string | undefined, extra: Record<string, unknown> = {}) =>
  ({ id, conversationId: "c1", direction: "in", sentAt: at(sec), kind: "document", fileName: `${id}.pdf`, caption, authorName: "Lemi", ...extra }) as unknown as Message;
const img = (id: string, sec: number, caption: string | undefined, extra: Record<string, unknown> = {}) =>
  ({ id, conversationId: "c1", direction: "in", sentAt: at(sec), kind: "image", caption, authorName: "Lemi", ...extra }) as unknown as Message;
const text = (id: string, sec: number, body: string) => ({ id, conversationId: "c1", direction: "in", sentAt: at(sec), kind: "text", text: body, authorName: "Lemi" }) as unknown as Message;

const CAP = "Bom dia\n\nSegue em anexo as guias de PIS e COFINS";

describe("repeatedCaptionIds", () => {
  it("dois PDFs seguidos com a mesma legenda: a legenda fica só no último", () => {
    expect([...repeatedCaptionIds([doc("cofins", 0, CAP), doc("pis", 2, CAP)])]).toEqual(["cofins"]);
  });

  it("vários arquivos: a legenda aparece só no último da sequência", () => {
    const ids = repeatedCaptionIds([doc("a", 0, CAP), img("b", 1, CAP), doc("c", 3, CAP), doc("d", 4, CAP)]);
    expect([...ids].sort()).toEqual(["a", "b", "c"]);
  });

  it("ignora diferenças de espaço e quebra de linha na comparação", () => {
    expect(repeatedCaptionIds([doc("a", 0, "Segue   em anexo\n"), doc("b", 1, " Segue em anexo")]).has("a")).toBe(true);
  });

  it("legendas diferentes, ou sem legenda, não mudam nada", () => {
    expect(repeatedCaptionIds([doc("a", 0, "Guia PIS"), doc("b", 1, "Guia COFINS")]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, undefined), doc("b", 1, undefined)]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, ""), doc("b", 1, "")]).size).toBe(0);
  });

  it("só agrupa arquivos seguidos, no mesmo lote (poucos segundos), do mesmo autor e do mesmo sentido", () => {
    expect(repeatedCaptionIds([doc("a", 0, CAP), text("t", 1, "ok"), doc("b", 2, CAP)]).size).toBe(0); // outra mensagem no meio
    expect(repeatedCaptionIds([doc("a", 0, CAP), doc("b", SAME_BATCH_MS / 1000 + 5, CAP)]).size).toBe(0); // muito tempo depois
    expect(repeatedCaptionIds([doc("a", 0, CAP), doc("b", 1, CAP, { authorName: "Outra pessoa" })]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, CAP), doc("b", 1, CAP, { direction: "out" })]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, CAP), doc("b", 1, CAP, { conversationId: "c2" })]).size).toBe(0);
  });

  it("mensagem apagada não entra no agrupamento, e texto com o mesmo corpo não é arquivo", () => {
    expect(repeatedCaptionIds([doc("a", 0, CAP, { deleted: true }), doc("b", 1, CAP)]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, CAP), doc("b", 1, CAP, { deleted: true })]).size).toBe(0);
    expect(repeatedCaptionIds([text("a", 0, CAP), text("b", 1, CAP)]).size).toBe(0);
  });

  it("lista vazia ou de uma mensagem", () => {
    expect(repeatedCaptionIds([]).size).toBe(0);
    expect(repeatedCaptionIds([doc("a", 0, CAP)]).size).toBe(0);
  });
});
