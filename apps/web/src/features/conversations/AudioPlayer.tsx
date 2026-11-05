import { useEffect, useRef, useState } from "react";
import { Pause, Play } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";

/**
 * Player de mensagem de voz. Com `url` usa <audio>; sem arquivo (dados fictícios)
 * simula a reprodução para manter a interação.
 */
export function AudioPlayer({
  durationSec,
  waveform,
  url,
  outgoing,
}: {
  durationSec: number;
  waveform: number[];
  url?: string;
  outgoing?: boolean;
}) {
  const audio = useRef<HTMLAudioElement>(null);
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
    if (url && audio.current) {
      if (playing) audio.current.pause();
      else void audio.current.play();
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
    <div className="flex w-64 max-w-full items-center gap-3 py-1">
      {url && (
        <audio
          ref={audio}
          src={url}
          preload="metadata"
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
        aria-label={playing ? "Pausar áudio" : "Reproduzir áudio"}
        className="inline-flex size-9 shrink-0 items-center justify-center rounded-full hover:bg-black/10 dark:hover:bg-white/10"
      >
        {playing ? <Pause className="size-5 fill-current" /> : <Play className="size-5 fill-current" />}
      </button>
      <div className="min-w-0 flex-1">
        <div
          role="slider"
          tabIndex={0}
          aria-label="Posição do áudio"
          aria-valuemin={0}
          aria-valuemax={Math.round(durationSec)}
          aria-valuenow={Math.round(pos)}
          className="flex h-7 cursor-pointer items-center gap-[2px]"
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            seek((e.clientX - r.left) / r.width);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowRight") seek(Math.min(1, progress + 0.1));
            if (e.key === "ArrowLeft") seek(Math.max(0, progress - 0.1));
          }}
        >
          {waveform.map((h, i) => (
            <span
              key={i}
              className={cn(
                "w-[3px] shrink-0 rounded-full",
                i / waveform.length < progress
                  ? "bg-read"
                  : outgoing
                    ? "bg-out-fg/45"
                    : "bg-fg/35",
              )}
              style={{ height: `${Math.round(h * 100)}%` }}
            />
          ))}
        </div>
        <span className="text-xs tabular-nums opacity-75">{formatDuration(playing || pos ? pos : durationSec)}</span>
      </div>
    </div>
  );
}
