import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { parseArgs, run } from "./chathm-listen.mjs";

// ChatHM de mentira: devolve as respostas combinadas e anota o que foi pedido.
async function fakeChatHM(script) {
  const calls = [];
  const srv = createServer((req, res) => {
    const u = new URL(req.url, "http://x");
    calls.push({ auth: req.headers.authorization, since: u.searchParams.get("since"), path: u.pathname });
    const out = script(calls.length, u);
    res.writeHead(out.status ?? 200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(out.body ?? {}));
  });
  await new Promise((r) => srv.listen(0, "127.0.0.1", r));
  return { url: `http://127.0.0.1:${srv.address().port}`, calls, close: () => srv.close() };
}
const reply = (id, at, text = "oi") => ({ conversationId: id, pending: { count: 1, lastAt: at }, lastReply: { text } });

test("avisa na hora, manda o token e só pede respostas depois do cursor", async () => {
  const dir = await mkdtemp(join(tmpdir(), "chathm-listen-"));
  const got = join(dir, "got.json");
  const srv = await fakeChatHM((n) =>
    n === 1 ? { body: { total: 1, conversations: [reply("c1", "2026-11-16T10:00:00Z", "Já paguei")] } } : { body: { total: 0, timedOut: true, conversations: [] } });
  try {
    const o = parseArgs(["--exec", `cat > ${got}`, "--state", join(dir, "s.json"), "--backlog", "--once", "--wait", "0"], { CHATHM_URL: srv.url, CHATHM_TOKEN: "chm_abc" });
    await run(o, { log() {} });
    const payload = JSON.parse(await readFile(got, "utf8"));
    assert.equal(payload.event, "replies");
    assert.equal(payload.conversations[0].lastReply.text, "Já paguei");
    assert.equal(srv.calls[0].auth, "Bearer chm_abc");
    assert.equal(srv.calls[0].since, null); // com --backlog, a primeira chamada pede tudo
    assert.equal(JSON.parse(await readFile(join(dir, "s.json"), "utf8")).cursor, "2026-11-16T10:00:00Z");
    // Segunda execução: parte do cursor guardado e não avisa de novo o que já avisou.
    await run(parseArgs(["--exec", `cat > ${got}.2`, "--state", join(dir, "s.json"), "--once", "--wait", "0"], { CHATHM_URL: srv.url, CHATHM_TOKEN: "chm_abc" }), { log() {} });
    assert.equal(srv.calls[1].since, "2026-11-16T10:00:00Z");
  } finally { srv.close(); }
});

test("no modo resumo junta as respostas e avisa uma vez, com a mais recente de cada conversa", async () => {
  const dir = await mkdtemp(join(tmpdir(), "chathm-listen-"));
  const out = join(dir, "digest.json");
  const srv = await fakeChatHM((n) =>
    n === 1 ? { body: { total: 2, conversations: [reply("c1", "2026-11-16T10:00:00Z", "a"), reply("c2", "2026-11-16T10:01:00Z", "b")] } }
      : n === 2 ? { body: { total: 1, conversations: [reply("c1", "2026-11-16T10:02:00Z", "a de novo")] } }
        : { body: { total: 0, timedOut: true, conversations: [] } });
  const ac = new AbortController();
  try {
    const o = parseArgs(["--exec", `cat > ${out}`, "--digest", "0.001", "--state", join(dir, "s.json"), "--backlog", "--wait", "0"], { CHATHM_URL: srv.url, CHATHM_TOKEN: "t" });
    setTimeout(() => ac.abort(), 600);
    await run(o, { log() {}, signal: ac.signal });
    const p = JSON.parse(await readFile(out, "utf8"));
    assert.equal(p.event, "replies.digest");
    assert.equal(p.total, 2);
    assert.equal(p.conversations.find((c) => c.conversationId === "c1").lastReply.text, "a de novo");
  } finally { srv.close(); }
});

test("token recusado para o script com uma mensagem clara, em vez de insistir", async () => {
  const dir = await mkdtemp(join(tmpdir(), "chathm-listen-"));
  const srv = await fakeChatHM(() => ({ status: 401, body: { error: { code: "invalid_token" } } }));
  try {
    const o = parseArgs(["--state", join(dir, "s.json"), "--wait", "0"], { CHATHM_URL: srv.url, CHATHM_TOKEN: "ruim" });
    await assert.rejects(run(o, { log() {} }), /recusou o token/);
    assert.equal(srv.calls.length, 1);
  } finally { srv.close(); }
});

test("sem URL ou token, explica o que falta", () => {
  assert.throws(() => parseArgs([], {}), /CHATHM_URL e CHATHM_TOKEN/);
  assert.throws(() => parseArgs(["--foo"], { CHATHM_URL: "x", CHATHM_TOKEN: "y" }), /desconhecida/);
});
