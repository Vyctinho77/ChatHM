import { useEffect, useRef, useState } from "react";
import { Pause, Play } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";

const DEMO = import.meta.env.VITE_DEMO === "1";

const BARS = 32;

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
 * Player de mensagem de voz. Com `url` usa <audio>. Sem arquivo: na demonstração
 * simula a reprodução; no app real o áudio fica indisponível até o download (Fase 4).
 */
export function AudioPlayer({
  durationSec,
  waveform,
  url,
  outgoing,
}: {
  durationSec: number;
  waveform?: number[];
  url?: string;
  outgoing?: boolean;
}) {
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

  return (
    <div className="flex w-60 min-w-0 max-w-full items-center gap-2.5 py-0.5">
      {url && (
        <audio
          ref={audio}
          src={url}
          preload="metadata"
          onPlay={() => setPlaying(true)}
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
        className="inline-flex size-8 shrink-0 items-center justify-center rounded-full hover:bg-black/10 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-white/10"
      >
        {playing ? <Pause className="size-4 fill-current" /> : <Play className="size-4 fill-current" />}
      </button>
      <div className="min-w-0 flex-1">
        <div
          role="slider"
          tabIndex={0}
          aria-label="Posição do áudio"
          aria-valuemin={0}
          aria-valuemax={Math.round(durationSec)}
          aria-valuenow={Math.round(pos)}
          className="flex h-6 cursor-pointer items-center gap-[2px] overflow-hidden"
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
              className={cn(
                "min-w-0 max-w-[3px] flex-1 rounded-full",
                i / bars.length < progress
                  ? "bg-read"
                  : outgoing
                    ? "bg-out-fg/45"
                    : "bg-fg/35",
              )}
              style={{ height: `${Math.round(h * 100)}%` }}
            />
          ))}
        </div>
        <span className="text-[11px] tabular-nums opacity-75">{formatDuration(playing || pos ? pos : durationSec)}</span>
      </div>
    </div>
  );
}
