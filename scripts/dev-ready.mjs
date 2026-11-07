import { setTimeout as delay } from 'node:timers/promises';

/** Espera a API confirmar que também consegue acessar o banco. */
export async function waitForApi(baseURL, { timeoutMs = 120_000, intervalMs = 300, signal } = {}) {
  const deadline = performance.now() + timeoutMs;
  const url = new URL('/readyz', baseURL);
  while (performance.now() < deadline) {
    signal?.throwIfAborted();
    try {
      const remaining = Math.max(1, Math.ceil(deadline - performance.now()));
      const timeout = AbortSignal.timeout(Math.min(1500, remaining));
      const response = await fetch(url, { signal: signal ? AbortSignal.any([signal, timeout]) : timeout });
      if (response.status === 200 && (await response.json()).status === 'ready') return;
    } catch {
      signal?.throwIfAborted();
      // Conexão recusada é normal enquanto o Go compila e executa as migrações.
    }
    const remaining = deadline - performance.now();
    if (remaining > 0) await delay(Math.min(intervalMs, remaining), undefined, { signal });
  }
  throw new Error('[dev] A API não ficou pronta em tempo. Confira os logs [api] antes de iniciar o frontend.');
}
