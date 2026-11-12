import { describe, expect, it } from "vitest";
import { applyMention, extractMentions, filterPeople, findMentionQuery, splitMentions, type MentionPerson } from "./mentions";

const P = (id: string, name: string): MentionPerson => ({ id, name });
const people = [P("ana", "Ana"), P("anap", "Ana Paula"), P("bia", "Beatriz Dois"), P("joao", "João Álvaro"), P("me", "Victor")];

describe("findMentionQuery", () => {
  it("finds the @ being typed at the caret", () => {
    expect(findMentionQuery("@", 1)).toEqual({ start: 0, query: "" });
    expect(findMentionQuery("oi @be", 6)).toEqual({ start: 3, query: "be" });
    expect(findMentionQuery("oi @Beatriz D", 13)).toEqual({ start: 3, query: "Beatriz D" });
    expect(findMentionQuery("(@jo", 4)).toEqual({ start: 1, query: "jo" });
    expect(findMentionQuery("@ana e mais", 4)).toEqual({ start: 0, query: "ana" }); // só vale o que está antes do cursor
  });

  it("ignores e-mails, finished words, line breaks and long runs", () => {
    expect(findMentionQuery("ana@empresa", 11)).toBeNull();
    expect(findMentionQuery("sem arroba", 10)).toBeNull();
    expect(findMentionQuery("@ana\nlinha nova", 15)).toBeNull();
    expect(findMentionQuery("@um dois tres quatro", 20)).toBeNull();
    expect(findMentionQuery("@" + "a".repeat(31), 32)).toBeNull();
    expect(findMentionQuery("@ana @", 6)).toEqual({ start: 5, query: "" }); // vale o último @
  });
});

describe("filterPeople", () => {
  it("matches by the start of any word, without accents, and never offers myself", () => {
    expect(filterPeople(people, "", "me").map((p) => p.id)).toEqual(["ana", "anap", "bia", "joao"]);
    expect(filterPeople(people, "alv", "me").map((p) => p.id)).toEqual(["joao"]);
    expect(filterPeople(people, "joao a", "me").map((p) => p.id)).toEqual(["joao"]);
    expect(filterPeople(people, "dois", "me").map((p) => p.id)).toEqual(["bia"]);
    expect(filterPeople(people, "vic", "me")).toEqual([]);
    expect(filterPeople(people, "zzz", "me")).toEqual([]);
  });

  it("puts names that start with the text first and caps the list", () => {
    expect(filterPeople([P("1", "Maria Ana"), P("2", "Ana")], "ana", "x").map((p) => p.id)).toEqual(["2", "1"]);
    const many = Array.from({ length: 20 }, (_, i) => P(String(i), `Pessoa ${i}`));
    expect(filterPeople(many, "", "x")).toHaveLength(6);
  });
});

describe("applyMention", () => {
  it("replaces the typed @ with the name and a space, keeping the rest", () => {
    expect(applyMention("oi @be", 3, 6, "Beatriz Dois")).toEqual({ text: "oi @Beatriz Dois ", caret: 17 });
    expect(applyMention("@be pode ver?", 0, 3, "Beatriz Dois")).toEqual({ text: "@Beatriz Dois pode ver?", caret: 14 });
  });
});

describe("extractMentions", () => {
  it("finds who the text mentions, once each", () => {
    expect(extractMentions("@Beatriz Dois e @João Álvaro, vejam. @Beatriz Dois de novo", people)).toEqual([
      { id: "bia", name: "Beatriz Dois" },
      { id: "joao", name: "João Álvaro" },
    ]);
  });

  it("lets the longest name win and matches only whole names", () => {
    expect(extractMentions("@Ana Paula, oi", people)).toEqual([{ id: "anap", name: "Ana Paula" }]);
    expect(extractMentions("@Ana, oi", people)).toEqual([{ id: "ana", name: "Ana" }]);
    expect(extractMentions("@Ana e @Ana Paula", people).map((m) => m.id).sort()).toEqual(["ana", "anap"]);
    expect(extractMentions("@Anabela", people)).toEqual([]);
    expect(extractMentions("mande para ana@empresa.com", people)).toEqual([]);
    expect(extractMentions("sem ninguém", people)).toEqual([]);
  });

  it("uses the person chosen in the list when two people share a name", () => {
    const twins = [P("a1", "Ana Lima"), P("a2", "Ana Lima")];
    expect(extractMentions("@Ana Lima", twins)).toEqual([{ id: "a1", name: "Ana Lima" }]);
    expect(extractMentions("@Ana Lima", twins, new Map([["Ana Lima", "a2"]]))).toEqual([{ id: "a2", name: "Ana Lima" }]);
  });

  it("escapes names with special characters", () => {
    expect(extractMentions("@Dr. (Silva) [HM]+", [P("x", "Dr. (Silva) [HM]+")])).toEqual([{ id: "x", name: "Dr. (Silva) [HM]+" }]);
  });
});

describe("splitMentions", () => {
  it("splits the text around each mention", () => {
    const mentions = [{ id: "bia", name: "Beatriz Dois" }];
    expect(splitMentions("Oi @Beatriz Dois, pode ver?", mentions)).toEqual([
      { text: "Oi " },
      { text: "@Beatriz Dois", mention: mentions[0] },
      { text: ", pode ver?" },
    ]);
  });

  it("returns the text untouched without mentions or matches", () => {
    expect(splitMentions("Oi a todos", undefined)).toEqual([{ text: "Oi a todos" }]);
    expect(splitMentions("Oi a todos", [{ id: "x", name: "Bia" }])).toEqual([{ text: "Oi a todos" }]);
    expect(splitMentions("", [{ id: "x", name: "Bia" }])).toEqual([{ text: "" }]);
  });

  it("prefers the longest name and ignores look-alikes inside words", () => {
    const m = [{ id: "ana", name: "Ana" }, { id: "anap", name: "Ana Paula" }];
    expect(splitMentions("@Ana Paula e @Ana", m).filter((s) => s.mention).map((s) => s.mention!.id)).toEqual(["anap", "ana"]);
    expect(splitMentions("@Anabela", m)).toEqual([{ text: "@Anabela" }]);
  });
});
