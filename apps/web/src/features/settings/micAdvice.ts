/** O que o teste de microfone mediu e o que isso quer dizer em palavras simples. */
export type MicReport = {
  label: string;
  sampleRate?: number; // Hz que o aparelho realmente entregou
  channels?: number;
  noiseSuppression?: boolean;
  echoCancellation?: boolean;
  peak: number; // maior nível de som (0 a 1) durante o teste
};

export type Advice = { tone: "warn" | "info"; text: string };

export function formatKHz(hz: number): string {
  const k = hz / 1000;
  return `${Number.isInteger(k) ? k : k.toFixed(1).replace(".", ",")} kHz`;
}

const BLUETOOTH = /bluetooth|hands-?free|airpods|buds|headset|fone|jabra|bose/i;

/** Avisos sobre o que pode deixar a voz "oca", abafada ou baixa. Vazio = nada de errado à vista. */
export function micAdvice(r: MicReport): Advice[] {
  const out: Advice[] = [];
  if (r.peak < 0.01) {
    out.push({ tone: "warn", text: "Quase nenhum som foi captado. Confira se o microfone certo está selecionado no navegador e se ele não está mudo." });
  }
  if (r.sampleRate && r.sampleRate < 44_100) {
    const bt = BLUETOOTH.test(r.label) ? " Fones Bluetooth no modo de chamada fazem exatamente isso: use o microfone do computador ou um fone com fio." : "";
    out.push({
      tone: "warn",
      text: `O microfone está entregando só ${formatKHz(r.sampleRate)} (o normal é 44,1 ou 48 kHz). Essa qualidade baixa deixa a voz abafada, "oca".${bt}`,
    });
  }
  if (r.peak > 0.95) {
    out.push({ tone: "warn", text: "O som está estourando (saturando). Afaste um pouco o microfone ou reduza o volume de entrada do aparelho." });
  }
  if (!out.length && (r.noiseSuppression || r.echoCancellation)) {
    out.push({ tone: "info", text: "Se a voz soar oca ao ouvir, desligue a redução de ruído acima e teste de novo: em alguns microfones e aparelhos antigos ela deixa o som artificial." });
  }
  return out;
}
