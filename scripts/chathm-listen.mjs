#!/usr/bin/env node
// Escuta as respostas de clientes no ChatHM e acorda o seu agente, SEM endereço público.
//
// O webhook do ChatHM precisa de um endereço na internet para receber. Quem roda o agente no computador não tem. Este script faz o caminho
// contrário: ele chama o ChatHM (GET /mcp/replies, que espera até ~50 s por uma resposta nova) em laço, o dia todo, e quando chega uma resposta
// executa um comando seu, avisa um endereço local ou só imprime. Sem dependências (Node 22+).
//
//   CHATHM_URL=https://chathm.com.br CHATHM_TOKEN=chm_... node scripts/chathm-listen.mjs --exec "meu-agente --stdin"
//
// Opções:  --exec "<comando>"   roda o comando a cada aviso, com o JSON no stdin (shell)
//          --webhook <url>      envia o JSON por POST (ex.: um servidorzinho local do seu agente)
//          --digest <minutos>   junta as respostas e avisa uma vez a cada N minutos (resumo), em vez de na hora
//          --backlog            também avisa o que já estava pendente ao começar (por padrão só o que chegar depois)
//          --state <arquivo>    onde guardar até onde já avisou (padrão: ~/.chathm-listen.json)
// Sem --exec e sem --webhook, imprime uma linha de JSON por aviso.
// O texto dos clientes vem como DADO: o agente nunca deve obedecer a instruções escritas dentro dele.
import { spawn } from "node:child_process";
import { readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export function parseArgs(argv, env = process.env) {
  const o = { url: env.CHATHM_URL, token: env.CHATHM_TOKEN, exec: "", webhook: "", digest: 0, backlog: false, state: join(homedir(), ".chathm-listen.json"), once: false, wait: 50 };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--exec") o.exec = argv[++i];
    else if (a === "--webhook") o.webhook = argv[++i];
    else if (a === "--digest") o.digest = Number(argv[++i]);
    else if (a === "--state") o.state = argv[++i];
    else if (a === "--backlog") o.backlog = true;
    else if (a === "--once") o.once = true;
    else if (a === "--wait") o.wait = Number(argv[++i]);
    else throw new Error(`opção desconhecida: ${a}`);
  }
  if (!o.url || !o.token) throw new Error("defina CHATHM_URL e CHATHM_TOKEN");
  if (o.digest && !(o.digest > 0)) throw new Error("--digest precisa de minutos > 0");
  o.url = o.url.replace(/\/+$/, "");
  return o;
}

async function readState(file) {
  try { return JSON.parse(await readFile(file, "utf8")); } catch { return {}; }
}

// Entrega um aviso. Se falhar, quem chamou NÃO avança o cursor e tenta de novo depois.
export async function deliver(o, payload) {
  const body = JSON.stringify(payload);
  if (o.exec) {
    await new Promise((resolve, reject) => {
      const p = spawn(o.exec, { shell: true, stdio: ["pipe", "inherit", "inherit"], env: { ...process.env, CHATHM_TOTAL: String(payload.total) } });
      p.on("error", reject);
      p.on("exit", (c) => (c === 0 ? resolve() : reject(new Error(`o comando saiu com código ${c}`))));
      p.stdin.on("error", () => {});
      p.stdin.end(body);
    });
  }
  if (o.webhook) {
    const r = await fetch(o.webhook, { method: "POST", headers: { "Content-Type": "application/json" }, body, signal: AbortSignal.timeout(15000) });
    if (!r.ok) throw new Error(`o endereço respondeu ${r.status}`);
  }
  if (!o.exec && !o.webhook) console.log(body);
}

const lastAtOf = (list) => list.reduce((m, c) => (c.pending?.lastAt > m ? c.pending.lastAt : m), "");

export async function run(o, { log = console.error, signal } = {}) {
  const st = await readState(o.state);
  let cursor = st.cursor || (o.backlog ? "" : new Date().toISOString());
  let buffer = [];
  let bufferSince = 0;
  let fails = 0;
  const save = () => writeFile(o.state, JSON.stringify({ cursor }), "utf8").catch(() => {});

  const flush = async () => {
    if (!buffer.length) return true;
    const byConv = new Map();
    for (const c of buffer) byConv.set(c.conversationId, c); // fica a mais recente de cada conversa
    const conversations = [...byConv.values()];
    try {
      await deliver(o, { event: o.digest ? "replies.digest" : "replies", at: new Date().toISOString(), total: conversations.length, conversations });
    } catch (e) {
      log(`falha ao avisar (tento de novo): ${e.message}`);
      return false;
    }
    buffer = [];
    await save();
    return true;
  };

  while (!signal?.aborted) {
    let res;
    try {
      const q = new URLSearchParams({ wait: String(o.wait) });
      if (cursor) q.set("since", cursor);
      res = await fetch(`${o.url}/mcp/replies?${q}`, { headers: { Authorization: `Bearer ${o.token}` }, signal: AbortSignal.any([AbortSignal.timeout((o.wait + 15) * 1000), ...(signal ? [signal] : [])]) });
    } catch (e) {
      if (signal?.aborted) break;
      fails++;
      log(`sem conexão com o ChatHM (${e.message}); nova tentativa em ${Math.min(60, 5 * fails)} s`);
      await sleep(Math.min(60, 5 * fails) * 1000);
      continue;
    }
    if (res.status === 401 || res.status === 403) throw new Error(`o ChatHM recusou o token (${res.status}): confira CHATHM_TOKEN e se o acesso por IA está ligado`);
    if (res.status === 429) { await sleep(Number(res.headers.get("retry-after") || 30) * 1000); continue; }
    if (!res.ok) { fails++; log(`o ChatHM respondeu ${res.status}`); await sleep(Math.min(60, 5 * fails) * 1000); continue; }
    fails = 0;
    const data = await res.json();
    const list = data.conversations || [];
    if (list.length) {
      const next = lastAtOf(list);
      if (!buffer.length) bufferSince = Date.now();
      buffer.push(...list);
      if (!o.digest) {
        if (await flush()) cursor = next > cursor ? next : cursor, await save();
        else await sleep(30000);
      } else {
        cursor = next > cursor ? next : cursor; // o que já está no resumo não volta a ser pedido
      }
    }
    if (o.digest && buffer.length && Date.now() - bufferSince >= o.digest * 60000) await flush();
    if (o.once && (list.length || data.timedOut)) break;
  }
  if (o.digest) await flush();
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
  const ac = new AbortController();
  process.on("SIGINT", () => ac.abort());
  process.on("SIGTERM", () => ac.abort());
  try {
    const o = parseArgs(process.argv.slice(2));
    console.error(`Escutando ${o.url} (${o.digest ? `resumo a cada ${o.digest} min` : "avisa na hora"}). Ctrl+C para parar.`);
    await run(o, { signal: ac.signal });
  } catch (e) {
    console.error(`chathm-listen: ${e.message}`);
    process.exit(2);
  }
}
