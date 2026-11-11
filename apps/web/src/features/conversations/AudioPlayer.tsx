import { useEffect, useRef, useState, type ReactNode } from "react";
import { Mic, Pause, Play } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { markHeard, useHeard } from "./audioHeard";

const DEMO = import.meta.env.VITE_DEMO === "1";

const BARS = 40;

/** Barras estáveis quando o provedor não envia a forma de onda. */
function fallbackWave(seed: number, n = BARS) {
  return Array.from({ length: n }, (_, i) => 0.25 + Math.abs(Math.sin(seed * 7.3 + i * 1.7)) * 0.75);
}

/**
 * Reamostra para um número fixo de barras (o WhatsApp envia 64): assim a forma de
 * onda sempre cabe no balão. Cada barra fica com o pico do seu trecho.
 */
function resample(wave: number[], n = BARS) {
  if (wave.length <= n) return wave;
  return Array.from({ length: n }, (_, i) => {
    const from = Math.floor((i * wave.length) / n);
    const to = Math.max(from + 1, Math.floor(((i + 1) * wave.length) / n));
    return Math.max(...wave.slice(from, to));
  });
}

/**
 * Player de mensagem de voz: foto de quem enviou com o ícone de microfone, play e barras finas.
 * Recebido: verde enquanto não ouvido, azul depois de ouvido (guardado neste navegador).
 * Com `url` usa <audio>. Sem arquivo: na demonstração simula a reprodução; no app real o áudio
 * fica indisponível até o download (Fase 4).
 */
export function AudioPlayer({
  id,
  durationSec,
  waveform,
  url,
  outgoing,
  metadata,
  avatarName,
  avatarSrc,
  group,
}: {
  /** Id da mensagem: é por ele que o ChatHM lembra que o áudio já foi ouvido. */
  id: string;
  durationSec: number;
  waveform?: number[];
  url?: string;
  outgoing?: boolean;
  metadata?: ReactNode;
  /** Quem enviou: foto (ou iniciais) ao lado do balão. */
  avatarName: string;
  avatarSrc?: string;
  group?: boolean;
}) {
  const heard = useHeard(id);
  // Enviado: não há como saber se o cliente ouviu (o recibo do WhatsApp mistura "lido" e "ouvido"), então fica neutro.
  const tone = outgoing
    ? { text: "text-out-fg", on: "bg-out-fg", off: "bg-out-fg/40", ring: "ring-out" }
    : heard
      ? { text: "text-audio-heard", on: "bg-audio-heard", off: "bg-audio-heard/35", ring: "ring-in" }
      : { text: "text-audio-new", on: "bg-audio-new", off: "bg-audio-new/35", ring: "ring-in" };
  const audio = useRef<HTMLAudioElement>(null);
  const bars = waveform?.length ? resample(waveform) : fallbackWave(durationSec || 1);
  const unavailable = !url && !DEMO;
  const [playing, setPlaying] = useState(false);
  const [pos, setPos] = useState(0); // segundos

  useEffect(() => {
    if (!playing || url) return;
    const started = performance.now() - pos * 1000;
    let raf = 0;
    const tick = () => {
      const t = (performance.now() - started) / 1000;
      if (t >= durationSec) {
        setPos(0);
        setPlaying(false);
        return;
      }
      setPos(t);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reinicia só ao tocar/pausar
  }, [playing, url, durationSec]);

  function toggle() {
    if (!outgoing) markHeard(id);
    const el = audio.current;
    if (url && el) {
      if (el.paused) {
        // Só um áudio toca por vez.
        document.querySelectorAll("audio").forEach((a) => a !== el && a.pause());
        el.play().catch(() => setPlaying(false));
      } else {
        el.pause();
      }
      return; // o estado "tocando" vem dos eventos do <audio> (onPlay/onPause)
    }
    setPlaying((p) => !p);
  }

  function seek(fraction: number) {
    const t = fraction * durationSec;
    setPos(t);
    if (audio.current) audio.current.currentTime = t;
  }

  const progress = durationSec ? pos / durationSec : 0;

  const avatar = (
    <span
      className="relative mt-0.5 shrink-0 self-start"
      title={outgoing ? undefined : heard ? "Áudio já ouvido" : "Áudio ainda não ouvido"}
    >
      <Avatar name={avatarName} src={avatarSrc} group={group} size="md" />
      <span className={cn("absolute -bottom-0.5 -right-0.5 flex size-[18px] items-center justify-center rounded-full bg-in ring-2", tone.ring)}>
        <Mic className={cn("size-3", outgoing ? "text-out" : tone.text)} strokeWidth={2.75} aria-hidden />
      </span>
      {!outgoing && <span className="sr-only">{heard ? "Áudio já ouvido" : "Áudio ainda não ouvido"}</span>}
    </span>
  );

  return (
    <div className="flex w-60 min-w-0 max-w-full items-start gap-2 py-0.5">
      {!outgoing && avatar}
      {url && (
        <audio
          ref={audio}
          src={url}
          preload="metadata"
          onPlay={() => {
            setPlaying(true);
            if (!outgoing) markHeard(id);
          }}
          onPause={() => setPlaying(false)}
          onError={() => setPlaying(false)}
          onTimeUpdate={(e) => setPos(e.currentTarget.currentTime)}
          onEnded={() => {
            setPlaying(false);
            setPos(0);
          }}
        />
      )}
      <button
        type="button"
        onClick={toggle}
        disabled={unavailable}
        aria-label={playing ? "Pausar áudio" : "Reproduzir áudio"}
        title={unavailable ? "Reprodução disponível quando o download de mídias for ativado (Fase 4)" : undefined}
        className={cn(
          "mt-1 inline-flex size-7 shrink-0 items-center justify-center rounded-full hover:bg-black/10 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-white/10",
          tone.text,
        )}
      >
        {playing ? <Pause className="size-[18px] fill-current" /> : <Play className="size-[18px] fill-current" />}
      </button>
      <div className="min-w-0 flex-1">
        <div
          role="slider"
          tabIndex={0}
          aria-label="Posição do áudio"
          aria-valuemin={0}
          aria-valuemax={Math.round(durationSec)}
          aria-valuenow={Math.round(pos)}
          className="flex h-8 cursor-pointer items-center justify-between overflow-hidden"
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            seek((e.clientX - r.left) / r.width);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowRight") seek(Math.min(1, progress + 0.1));
            if (e.key === "ArrowLeft") seek(Math.max(0, progress - 0.1));
          }}
        >
          {bars.map((h, i) => (
            <span
              key={i}
              className={cn("w-[2px] shrink-0 rounded-full", i / bars.length < progress ? tone.on : tone.off)}
              style={{ height: `${Math.round(18 + h * 70)}%` }}
            />
          ))}
        </div>
        <div className="mt-0.5 flex items-end justify-between gap-2">
          <span className="text-[11px] tabular-nums opacity-75">{formatDuration(playing || pos ? pos : durationSec)}</span>
          {metadata}
        </div>
      </div>
      {outgoing && avatar}
    </div>
  );
}
