import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Mic, Pause, Play } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { markHeard, useHeard } from "./audioHeard";
import { decodeWave, shapeWave, speechWave } from "./waveform";

const DEMO = import.meta.env.VITE_DEMO === "1";

/**
 * Player de mensagem de voz: foto de quem enviou com o ícone de microfone, play e barras finas, tudo
 * centralizado na altura do meio da foto. Recebido: foto à esquerda, verde enquanto não ouvido, azul depois
 * de ouvido (guardado neste navegador). Enviado: foto à direita, cor neutra.
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
  const audio = useRef<HTMLAudioElement>(null);
  const waveBox = useRef<HTMLDivElement>(null);
  const heard = useHeard(id);

  // Onda: a do provedor (realçada) → a decodificada do arquivo quando o áudio aparece na tela → uma parecida com fala.
  const [decoded, setDecoded] = useState<number[] | null>(null);
  const needsDecode = !waveform?.length && !!url;
  useEffect(() => {
    const el = waveBox.current;
    if (!needsDecode || !url || !el || typeof IntersectionObserver === "undefined") return;
    let cancelled = false;
    const io = new IntersectionObserver((entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      io.disconnect();
      void decodeWave(url).then((w) => {
        if (!cancelled && w) setDecoded(w);
      });
    });
    io.observe(el);
    return () => {
      cancelled = true;
      io.disconnect();
    };
  }, [needsDecode, url]);
  const bars = useMemo(() => (waveform?.length ? shapeWave(waveform) : (decoded ?? speechWave(id))), [waveform, decoded, id]);

  const unavailable = !url && !DEMO;
  const [playing, setPlaying] = useState(false);
  const [pos, setPos] = useState(0); // segundos

  // Enviado: não há como saber se o cliente ouviu (o recibo do WhatsApp mistura "lido" e "ouvido"), então fica neutro.
  const tone = outgoing
    ? { text: "text-out-fg", on: "bg-out-fg", off: "bg-out-fg/40", ring: "ring-out" }
    : heard
      ? { text: "text-audio-heard", on: "bg-audio-heard", off: "bg-audio-heard/35", ring: "ring-in" }
      : { text: "text-audio-new", on: "bg-audio-new", off: "bg-audio-new/35", ring: "ring-in" };

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

  // Foto, play e ondas têm 40 px de altura e começam na mesma linha: o centro de cada um é o meio da foto.
  const avatar = (
    <span className="relative h-10 shrink-0" title={outgoing ? undefined : heard ? "Áudio já ouvido" : "Áudio ainda não ouvido"}>
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
      <div className="flex h-10 shrink-0 items-center">
        <button
          type="button"
          onClick={toggle}
          disabled={unavailable}
          aria-label={playing ? "Pausar áudio" : "Reproduzir áudio"}
          title={unavailable ? "Reprodução disponível quando o download de mídias for ativado (Fase 4)" : undefined}
          className={cn(
            "inline-flex size-8 items-center justify-center rounded-full hover:bg-black/10 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-white/10",
            tone.text,
          )}
        >
          {playing ? <Pause className="size-[18px] fill-current" /> : <Play className="size-[18px] fill-current" />}
        </button>
      </div>
      <div className="min-w-0 flex-1">
        <div
          ref={waveBox}
          role="slider"
          tabIndex={0}
          aria-label="Posição do áudio"
          aria-valuemin={0}
          aria-valuemax={Math.round(durationSec)}
          aria-valuenow={Math.round(pos)}
          className="flex h-10 cursor-pointer items-center justify-between overflow-hidden"
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
              style={{ height: `${Math.round(10 + h * 72)}%` }}
            />
          ))}
        </div>
        <div className="flex items-end justify-between gap-2">
          <span className="text-[11px] tabular-nums opacity-75">{formatDuration(playing || pos ? pos : durationSec)}</span>
          {metadata}
        </div>
      </div>
      {outgoing && avatar}
    </div>
  );
}
