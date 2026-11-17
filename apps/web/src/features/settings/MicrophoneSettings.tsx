import { useEffect, useRef, useState } from "react";
import { Mic, Square } from "lucide-react";
import { Alert, Button } from "@/components/ui";
import { cn } from "@/lib/cn";
import { openVoiceCapture, recordingError, type VoiceCapture } from "@/features/conversations/voiceCapture";
import { getVoicePrefs, setVoicePrefs, type VoicePrefs } from "@/features/conversations/voicePrefs";
import { formatKHz, micAdvice, type MicReport } from "./micAdvice";

const TEST_SECONDS = 5;

type Phase = "idle" | "recording" | "done";

/** O navegador informa true/false (ou, no eco, textos como "all" e "remote-only"): vira ligado/desligado. */
const asOnOff = (v: unknown): boolean | undefined => (v === undefined ? undefined : v !== false && v !== "none");

/**
 * Microfone: liga/desliga a redução de ruído e o cancelamento de eco do navegador (a voz "oca" costuma vir daí) e faz um teste de
 * 5 segundos: mostra o nível do som, toca de volta e informa o que o aparelho realmente entregou (taxa, canais, processamento).
 */
export function MicrophoneSettings() {
  const supported = typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== "undefined";
  const [prefs, setPrefs] = useState<VoicePrefs>(getVoicePrefs);
  const [phase, setPhase] = useState<Phase>("idle");
  const [level, setLevel] = useState(0);
  const [left, setLeft] = useState(TEST_SECONDS);
  const [report, setReport] = useState<MicReport | null>(null);
  const [audioUrl, setAudioUrl] = useState<string | null>(null);
  const [error, setError] = useState("");
  const session = useRef<{ stop: () => void; abort: () => void } | null>(null);
  const urlRef = useRef<string | null>(null);

  useEffect(
    () => () => {
      session.current?.abort();
      if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    },
    [],
  );

  function change(patch: Partial<VoicePrefs>) {
    setPrefs(setVoicePrefs(patch));
  }

  function clearResult() {
    if (urlRef.current) URL.revokeObjectURL(urlRef.current);
    urlRef.current = null;
    setAudioUrl(null);
    setReport(null);
  }

  async function start() {
    if (session.current) return;
    setError("");
    clearResult();
    let capture: VoiceCapture;
    try {
      capture = await openVoiceCapture();
    } catch (e) {
      setError(recordingError(e));
      return;
    }
    const track = capture.input.getAudioTracks()[0];
    const settings = track?.getSettings() ?? {};
    let ctx: AudioContext | undefined;
    let analyser: AnalyserNode | undefined;
    try {
      ctx = new AudioContext();
      analyser = ctx.createAnalyser();
      analyser.fftSize = 1024;
      ctx.createMediaStreamSource(capture.stream).connect(analyser);
    } catch {
      analyser = undefined; // sem medidor: o teste grava e toca do mesmo jeito
    }
    const mimeType = ["audio/webm;codecs=opus", "audio/ogg;codecs=opus"].find((t) => MediaRecorder.isTypeSupported(t));
    const recorder = new MediaRecorder(capture.stream, { audioBitsPerSecond: 64_000, ...(mimeType ? { mimeType } : {}) });
    const chunks: Blob[] = [];
    let peak = 0;
    let aborted = false;
    const buf = new Float32Array(analyser?.fftSize ?? 1);
    const meter = setInterval(() => {
      if (!analyser) return;
      analyser.getFloatTimeDomainData(buf);
      let sum = 0;
      for (const v of buf) sum += v * v;
      const rms = Math.sqrt(sum / buf.length);
      peak = Math.max(peak, rms * 2.2); // pico aproximado da fala (a fala tem pico ~2x o RMS)
      setLevel(Math.min(1, rms * 4));
    }, 80);
    const startedAt = performance.now();
    const countdown = setInterval(() => setLeft(Math.max(0, TEST_SECONDS - (performance.now() - startedAt) / 1000)), 200);
    const finish = () => {
      clearInterval(meter);
      clearInterval(countdown);
      clearTimeout(timer);
      capture.dispose();
      if (ctx && ctx.state !== "closed") void ctx.close().catch(() => undefined);
      session.current = null;
      setLevel(0);
    };
    recorder.ondataavailable = (e) => {
      if (e.data.size) chunks.push(e.data);
    };
    recorder.onstop = () => {
      finish();
      if (aborted) return;
      const url = URL.createObjectURL(new Blob(chunks, { type: recorder.mimeType }));
      urlRef.current = url;
      setAudioUrl(url);
      setReport({
        label: track?.label ?? "",
        sampleRate: settings.sampleRate,
        channels: settings.channelCount,
        noiseSuppression: asOnOff(settings.noiseSuppression),
        echoCancellation: asOnOff(settings.echoCancellation),
        peak: analyser ? peak : 0.5, // sem medidor, não dá para julgar o nível
      });
      setPhase("done");
    };
    recorder.onerror = () => {
      aborted = true;
      finish();
      setPhase("idle");
      setError("A gravação do teste foi interrompida. Confira o microfone e tente de novo.");
    };
    const timer = setTimeout(() => recorder.state !== "inactive" && recorder.stop(), TEST_SECONDS * 1000);
    session.current = {
      stop: () => recorder.state !== "inactive" && recorder.stop(),
      abort: () => {
        aborted = true;
        if (recorder.state !== "inactive") recorder.stop();
        else finish();
      },
    };
    setLeft(TEST_SECONDS);
    setPhase("recording");
    recorder.start();
  }

  const advice = report ? micAdvice(report) : [];
  const toggles: { key: keyof VoicePrefs; title: string; text: string }[] = [
    {
      key: "noiseSuppression",
      title: "Redução de ruído do navegador",
      text: "Tira chiado e barulho de fundo. Em alguns microfones e aparelhos antigos deixa a voz mais “oca”: se for o seu caso, desligue.",
    },
    {
      key: "echoCancellation",
      title: "Cancelamento de eco",
      text: "Evita que o som do alto-falante volte pelo microfone. Se você usa fone de ouvido, pode desligar.",
    },
  ];

  if (!supported) return <p className="text-sm text-muted">Este navegador não permite gravar áudio.</p>;

  return (
    <div className="flex flex-col gap-5">
      {toggles.map((t) => (
        <label key={t.key} className="flex cursor-pointer items-start justify-between gap-4 text-sm">
          <span>
            <span className="block font-medium">{t.title}</span>
            <span className="mt-1 block text-xs text-muted">{t.text}</span>
          </span>
          <input type="checkbox" className="mt-1 size-4 shrink-0" checked={prefs[t.key]} disabled={phase === "recording"} onChange={(e) => change({ [t.key]: e.target.checked })} />
        </label>
      ))}

      <div className="flex flex-col gap-3 rounded-lg border border-line p-3">
        <div className="flex flex-wrap items-center gap-3">
          {phase === "recording" ? (
            <Button onClick={() => session.current?.stop()}>
              <Square className="size-4 fill-current" /> Parar
            </Button>
          ) : (
            <Button variant="secondary" onClick={() => void start()}>
              <Mic className="size-4" /> {phase === "done" ? "Testar de novo" : `Testar microfone (${TEST_SECONDS} s)`}
            </Button>
          )}
          {phase === "recording" && (
            <span className="text-xs text-muted" role="status" aria-live="polite">
              Fale normalmente… {Math.ceil(left)} s
            </span>
          )}
        </div>

        <div className="h-2 overflow-hidden rounded-full bg-hover" aria-hidden={phase !== "recording"} role="meter" aria-label="Nível do som" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(level * 100)}>
          <div className={cn("h-full rounded-full bg-brand transition-[width] duration-75", level > 0.9 && "bg-danger")} style={{ width: `${Math.round(level * 100)}%` }} />
        </div>

        {error && <Alert>{error}</Alert>}

        {audioUrl && (
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted">Ouça como ficou (é assim que o cliente vai ouvir):</span>
            <audio controls src={audioUrl} className="h-10 w-full" />
          </div>
        )}

        {report && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
            <dt className="text-muted">Microfone</dt>
            <dd className="min-w-0 truncate" title={report.label}>{report.label || "não informado"}</dd>
            <dt className="text-muted">Qualidade entregue</dt>
            <dd>{report.sampleRate ? formatKHz(report.sampleRate) : "não informada"}{report.channels ? `, ${report.channels === 1 ? "mono" : `${report.channels} canais`}` : ""}</dd>
            <dt className="text-muted">Redução de ruído</dt>
            <dd>{report.noiseSuppression === undefined ? "não informado" : report.noiseSuppression ? "ligada" : "desligada"}</dd>
            <dt className="text-muted">Cancelamento de eco</dt>
            <dd>{report.echoCancellation === undefined ? "não informado" : report.echoCancellation ? "ligado" : "desligado"}</dd>
          </dl>
        )}

        {advice.map((a) => (
          <Alert key={a.text} tone={a.tone === "warn" ? "danger" : "info"}>
            {a.text}
          </Alert>
        ))}
        {report && !advice.length && <p className="text-xs text-muted">Tudo certo com o microfone: o som chegou limpo e na qualidade normal.</p>}
      </div>
      <p className="text-xs text-muted">As opções ficam salvas neste navegador. Valem para as próximas gravações.</p>
    </div>
  );
}
