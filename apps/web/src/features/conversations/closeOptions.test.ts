import { describe, expect, it } from "vitest";
import { endHint, endOptions, joinNames, otherPeople } from "./closeOptions";
import type { Conversation } from "./types";

const ME = "me";
const person = (id: string) => ({ id, name: id.toUpperCase() });
const conv = (over: Partial<Conversation> = {}) => ({ status: "in_progress" as const, assignee: person(ME), participants: [], ...over });

describe("endOptions", () => {
  it("gives the sole responsible one direct action that closes the atendimento", () => {
    expect(endOptions(conv(), { myId: ME, supervisor: false })).toMatchObject({ actions: ["all"], direct: true, others: [] });
  });

  it("gives the responsible both choices when other people are in the atendimento", () => {
    const o = endOptions(conv({ participants: [person("bia"), person("caio")] }), { myId: ME, supervisor: false });
    expect(o.actions).toEqual(["leave", "all"]);
    expect(o.direct).toBe(false);
    expect(o.others.map((p) => p.id)).toEqual(["bia", "caio"]);
  });

  it("gives a participant only the way out", () => {
    const o = endOptions(conv({ assignee: person("ana"), participants: [person(ME), person("bia")] }), { myId: ME, supervisor: false });
    expect(o.actions).toEqual(["leave"]);
    expect(o.others.map((p) => p.id)).toEqual(["ana", "bia"]);
  });

  it("gives an administrator who is not in the atendimento only 'for everyone', which asks for confirmation", () => {
    const o = endOptions(conv({ assignee: person("ana") }), { myId: ME, supervisor: true });
    expect(o).toMatchObject({ actions: ["all"], direct: false });
    expect(o.others.map((p) => p.id)).toEqual(["ana"]);
  });

  it("gives an administrator who is also a participant both choices", () => {
    expect(endOptions(conv({ assignee: person("ana"), participants: [person(ME)] }), { myId: ME, supervisor: true }).actions).toEqual(["leave", "all"]);
  });

  it("gives a participant of a queue conversation (no responsible) the way out", () => {
    const o = endOptions(conv({ assignee: undefined, participants: [person(ME)] }), { myId: ME, supervisor: false });
    expect(o).toMatchObject({ actions: ["leave"], direct: false, others: [] });
  });

  it("gives nothing to someone who is not in the atendimento and does not supervise, or when it is closed", () => {
    expect(endOptions(conv({ assignee: person("ana") }), { myId: ME, supervisor: false }).actions).toEqual([]);
    expect(endOptions(conv({ status: "closed" }), { myId: ME, supervisor: true }).actions).toEqual([]);
  });
});

describe("helpers", () => {
  it("lists the other people without me or duplicates of the responsible", () => {
    expect(otherPeople({ assignee: person("ana"), participants: [person(ME), person("bia")] }, ME).map((p) => p.id)).toEqual(["ana", "bia"]);
    expect(otherPeople({ assignee: person(ME), participants: [] }, ME)).toEqual([]);
  });

  it("joins names in Portuguese", () => {
    expect(joinNames([])).toBe("");
    expect(joinNames(["Ana"])).toBe("Ana");
    expect(joinNames(["Ana", "Bia"])).toBe("Ana e Bia");
    expect(joinNames(["Ana", "Bia", "Caio"])).toBe("Ana, Bia e Caio");
  });

  it("describes what each action does", () => {
    expect(endHint("leave", [{ name: "Ana" }, { name: "Bia" }])).toBe("Você sai; o atendimento continua com Ana e Bia");
    expect(endHint("leave", [])).toContain("volta à fila");
    expect(endHint("all", [{ name: "Ana" }])).toBe("Também para Ana; o cliente volta à fila");
  });
});
