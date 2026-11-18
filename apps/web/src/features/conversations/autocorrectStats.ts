import { post } from "@/lib/api";
import { DEMO } from "@/lib/env";
import type { Fix, Layer } from "./autocorrect";

/**
 * Medição do corretor, para melhorar a lista de palavras com uso real. O que é contado: por camada do corretor e por par "palavra digitada →
 * palavra corrigida", quantas vezes a correção foi aplicada e quantas a pessoa desfez com Backspace. NADA mais: nem o texto da mensagem,
 * nem a frase em volta, nem a conversa, nem o cliente, nem quem digitou. Só palavras soltas (letras, até 40) e contadores, enviados em
 * lotes de tempos em tempos ao servidor do próprio escritório (POST /api/v1/writing/stats). Não há tela: quem lê é o administrador
 * (GET /api/v1/writing/stats ou o banco). Ver docs/corretor.md.
 */
export type StatsEntry = { layer: Layer; from: string; to: string; applied: number; undone: number };

const MAX_BATCH = 200; // o mesmo limite do servidor
const MAX_PENDING = 500; // sem rede por muito tempo: não cresce sem limite
const FLUSH_EVERY_MS = 5 * 60 * 1000;

const pending = new Map<string, StatsEntry>();

function bump(fix: Fix, field: "applied" | "undone") {
  const from = fix.from.toLowerCase();
  const to = fix.to.toLowerCase();
  if (!/^\p{L}{1,40}$/u.test(from) || !/^\p{L}{1,40}$/u.test(to)) return;
  const key = `${fix.layer}|${from}|${to}`;
  const cur = pending.get(key);
  if (cur) cur[field] += 1;
  else if (pending.size < MAX_PENDING) pending.set(key, { layer: fix.layer, from, to, applied: 0, undone: 0, [field]: 1 });
}

export const recordApplied = (fix: Fix) => bump(fix, "applied");
export const recordUndone = (fix: Fix) => bump(fix, "undone");

/** O que ainda não foi enviado (para testes e diagnóstico). */
export const pendingStats = (): StatsEntry[] => [...pending.values()].map((e) => ({ ...e }));
export const resetStats = () => pending.clear();

const defaultSend = (entries: StatsEntry[]) => post<void>("/writing/stats", { entries });

let flushing = false;

/** Envia um lote do que está pendente. Se falhar, o lote volta para a fila e tenta de novo na próxima vez. */
export async function flushStats(send: (entries: StatsEntry[]) => Promise<unknown> = defaultSend): Promise<void> {
  if (flushing || pending.size === 0) return;
  flushing = true;
  const batch = [...pending.entries()].slice(0, MAX_BATCH);
  batch.forEach(([key]) => pending.delete(key));
  try {
    await send(batch.map(([, e]) => e));
  } catch {
    for (const [key, e] of batch) {
      const cur = pending.get(key);
      if (cur) {
        cur.applied += e.applied;
        cur.undone += e.undone;
      } else if (pending.size < MAX_PENDING) pending.set(key, e);
    }
  } finally {
    flushing = false;
  }
}

let started = false;

/** Liga o envio em segundo plano (de 5 em 5 minutos e quando a aba some). Chamar várias vezes é seguro. */
export function startStatsReporter() {
  if (started || DEMO || typeof window === "undefined") return;
  started = true;
  window.setInterval(() => void flushStats(), FLUSH_EVERY_MS);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") void flushStats();
  });
}
