/**
 * Forma de onda das mensagens de voz, no estilo do WhatsApp: barras que acompanham a energia da fala,
 * com picos nas sílabas e barras baixas nas pausas.
 */

export const BARS = 40;
const MIN = 0.1; // pausa: sobra um tracinho, nunca some

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));

/** Valor no percentil p (0 a 1) de uma lista. */
function percentile(values: number[], p: number) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.round(p * (sorted.length - 1))))] ?? 0;
}

/**
 * Leva qualquer forma de onda (0–1, 0–100, comprimida como a do WhatsApp, que começa em 0,2) para `n` barras com contraste:
 * cada barra pega o trecho dela (pico e média), o chão e o teto vêm dos percentis 8 e 96 para que um estouro isolado
 * não achate as demais, e uma curva leve realça a diferença entre fala e pausa.
 */
export function shapeWave(wave: number[], n = BARS): number[] {
  const src = wave.filter((v) => Number.isFinite(v));
  if (src.length === 0) return speechWave("vazio", n);
  const buckets = Array.from({ length: n }, (_, i) => {
    const from = Math.floor((i * src.length) / n);
    const to = Math.max(from + 1, Math.floor(((i + 1) * src.length) / n));
    const part = src.slice(from, to);
    const peak = Math.max(...part);
    const mean = part.reduce((a, b) => a + b, 0) / part.length;
    return 0.65 * peak + 0.35 * mean;
  });
  const lo = percentile(buckets, 0.08);
  const hi = percentile(buckets, 0.96);
  if (hi - lo < 1e-6) return speechWave(String(buckets[0] ?? 0), n); // sem informação útil (tudo igual)
  return buckets.map((v) => MIN + (1 - MIN) * Math.pow(clamp01((v - lo) / (hi - lo)), 1.15));
}

function hash(s: string) {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return h >>> 0;
}

function prng(seed: number) {
  let a = seed || 1;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * Onda parecida com fala para quando não há dados: rajadas (sílabas e palavras) de força variada separadas por pausas curtas.
 * Determinística pela semente (o id da mensagem): a mesma mensagem tem sempre o mesmo desenho.
 */
export function speechWave(seed: string, n = BARS): number[] {
  const rnd = prng(hash(seed));
  const out: number[] = [];
  while (out.length < n) {
    const len = 2 + Math.floor(rnd() * 4); // 2 a 5 barras por sílaba
    const power = 0.35 + rnd() * 0.65;
    for (let i = 0; i < len && out.length < n; i++) {
      const hill = Math.sin(((i + 0.5) / len) * Math.PI); // sobe e desce dentro da sílaba
      out.push(clamp01(MIN + (power * (0.45 + 0.55 * hill) + (rnd() - 0.5) * 0.22) * (1 - MIN)));
    }
    const gap = rnd() < 0.55 ? 1 : 0; // pausa curta entre palavras
    for (let i = 0; i < gap && out.length < n; i++) out.push(MIN + rnd() * 0.1);
  }
  return out;
}

const decoded = new Map<string, Promise<number[] | null>>();

/** Onda real a partir do arquivo de áudio (quando o provedor não enviou a forma de onda). Uma decodificação por arquivo. */
export function decodeWave(url: string, n = BARS): Promise<number[] | null> {
  let p = decoded.get(url);
  if (!p) {
    p = (async () => {
      let ctx: AudioContext | undefined;
      try {
        const res = await fetch(url, { credentials: "same-origin" });
        if (!res.ok) return null;
        ctx = new AudioContext();
        const buf = await ctx.decodeAudioData(await res.arrayBuffer());
        const data = buf.getChannelData(0);
        const step = Math.max(1, Math.floor(data.length / n));
        const rms = Array.from({ length: n }, (_, i) => {
          let sum = 0;
          const from = i * step;
          const to = Math.min(data.length, from + step);
          for (let j = from; j < to; j++) sum += data[j]! * data[j]!;
          return Math.sqrt(sum / Math.max(1, to - from));
        });
        return shapeWave(rms, n);
      } catch {
        return null;
      } finally {
        if (ctx && ctx.state !== "closed") void ctx.close().catch(() => undefined);
      }
    })();
    decoded.set(url, p);
    if (decoded.size > 200) decoded.delete(decoded.keys().next().value as string);
  }
  return p;
}
