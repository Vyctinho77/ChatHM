import { useEffect, useMemo } from "react";
import { useMatch, useNavigate } from "react-router";
import { CornerUpRight, Mic, Pause, Play, X } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { useQueryClient } from "@tanstack/react-query";
import { isHeard, useHeard } from "./audioHeard";
import { messagesKey, nextUnheardTrack } from "./audioQueue";
import { seekTrack, setNextTrackResolver, stopAudio, toggleTrack, useCurrentAudio, type Track } from "./audioEngine";
import type { Message } from "./types";

const BARS = 26;

/** As mesmas barras do balão (o balão manda as suas), reduzidas para caber na pílula. */
function miniWave(wave: number[] | undefined): number[] {
  if (!wave?.length) return Array.from({ length: BARS }, (_, i) => 0.35 + 0.4 * Math.abs(Math.sin(i * 1.7)));
  return Array.from({ length: BARS }, (_, i) => wave[Math.min(wave.length - 1, Math.floor((i * wave.length) / BARS))] ?? 0.3);
}

/**
 * Controle do áudio em segundo plano: uma pílula discreta no alto, no meio, quando há um áudio tocando (ou pausado) de uma
 * conversa que não está aberta — para ouvir até o fim enquanto se atende outra pessoa. Segue o visual do balão de voz: foto de
 * quem enviou com o microfone, play e as mesmas ondas (verde enquanto não ouvido, azul depois). Dentro da própria conversa
 * quem comanda é o balão. Some sozinha quando o áudio acaba sem outro para tocar.
 */
export function AudioBar() {
  const current = useCurrentAudio();
  const open = useMatch("/conversas/:id");
  const navigate = useNavigate();

  const qc = useQueryClient();

  // Ao sair do app (sair da conta) o áudio não fica tocando sozinho.
  useEffect(() => () => stopAudio(), []);

  // Quando um áudio acaba, toca o próximo recebido e ainda não ouvido da mesma conversa, estando ela aberta ou não.
  useEffect(() => {
    setNextTrackResolver((finished) => nextUnheardTrack(qc.getQueryData<Message[]>(messagesKey(finished.conversationId)), finished, isHeard));
    return () => setNextTrackResolver(undefined);
  }, [qc]);

  if (!current || open?.params.id === current.track.conversationId) return null;
  return <Pill {...current} onOpen={(t) => void navigate(`/conversas/${t.conversationId}?msg=${encodeURIComponent(t.id)}`)} />;
}

function Pill({ track, playing, pos, failed, onOpen }: { track: Track; playing: boolean; pos: number; failed: boolean; onOpen: (t: Track) => void }) {
  const heard = useHeard(track.id);
  const bars = useMemo(() => miniWave(track.wave), [track.wave]);
  const progress = track.durationSec ? Math.min(1, pos / track.durationSec) : 0;
  // Mesmas cores do balão: verde enquanto não ouvido, azul depois (enviado por nós: azul).
  const tone = track.outgoing || heard ? { text: "text-audio-heard", on: "bg-audio-heard", off: "bg-audio-heard/35" } : { text: "text-audio-new", on: "bg-audio-new", off: "bg-audio-new/35" };
  const name = track.avatarName ?? track.label;

  return (
    // Linha reservada no alto (empurra a tela): a pílula não cobre o cabeçalho nem a caixa de mensagem.
    <div role="region" aria-label="Áudio em reprodução" className="flex shrink-0 justify-center px-3 pb-1 pt-2">
      <div className="flex w-full max-w-[21rem] items-center gap-1.5 rounded-full border border-line bg-panel py-1 pl-1.5 pr-1.5 text-fg shadow-sm" title={`Áudio de ${track.label}`}>
        <span className="relative shrink-0">
          <Avatar name={name} src={track.avatarSrc} group={track.group} size="sm" />
          <span className="absolute -bottom-0.5 -right-0.5 flex size-[14px] items-center justify-center rounded-full bg-panel ring-2 ring-panel">
            <Mic className={cn("size-2.5", tone.text)} strokeWidth={2.75} aria-hidden />
          </span>
        </span>
        <button
          type="button"
          onClick={() => toggleTrack(track)}
          aria-label={playing ? "Pausar áudio" : "Reproduzir áudio"}
          className={cn("inline-flex size-7 shrink-0 items-center justify-center rounded-full hover:bg-black/10 dark:hover:bg-white/10", tone.text)}
        >
          {playing ? <Pause className="size-4 fill-current" /> : <Play className="size-4 fill-current" />}
        </button>
        <div
          role="slider"
          tabIndex={0}
          aria-label={`Posição do áudio de ${track.label}`}
          aria-valuemin={0}
          aria-valuemax={Math.round(track.durationSec)}
          aria-valuenow={Math.round(pos)}
          className="flex h-6 min-w-0 flex-1 cursor-pointer items-center justify-between overflow-hidden"
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            seekTrack(track, ((e.clientX - r.left) / r.width) * track.durationSec);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowRight") seekTrack(track, Math.min(track.durationSec, pos + 5));
            if (e.key === "ArrowLeft") seekTrack(track, Math.max(0, pos - 5));
          }}
        >
          {bars.map((h, i) => (
            <span key={i} className={cn("w-[2px] shrink-0 rounded-full", i / bars.length < progress ? tone.on : tone.off)} style={{ height: `${Math.round(14 + h * 72)}%` }} />
          ))}
        </div>
        <span className={cn("w-8 shrink-0 text-right text-[11px] tabular-nums", failed ? "text-danger" : "opacity-75")}>
          {failed ? "Erro" : formatDuration(playing || pos ? pos : track.durationSec)}
        </span>
        <button
          type="button"
          onClick={() => onOpen(track)}
          aria-label="Ir para a conversa"
          title="Ir para a conversa"
          className="inline-flex size-6 shrink-0 items-center justify-center rounded-full text-muted hover:bg-black/10 hover:text-fg dark:hover:bg-white/10"
        >
          <CornerUpRight className="size-3.5" />
        </button>
        <button
          type="button"
          onClick={stopAudio}
          aria-label="Fechar"
          title="Fechar"
          className="inline-flex size-6 shrink-0 items-center justify-center rounded-full text-muted hover:bg-black/10 hover:text-fg dark:hover:bg-white/10"
        >
          <X className="size-3.5" />
        </button>
      </div>
    </div>
  );
}
