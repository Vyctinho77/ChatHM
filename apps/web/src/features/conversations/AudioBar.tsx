import { useEffect } from "react";
import { useMatch, useNavigate } from "react-router";
import { CornerUpRight, Mic, Pause, Play, X } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { stopAudio, toggleTrack, seekTrack, useCurrentAudio } from "./audioEngine";

/**
 * Controle do áudio em segundo plano (faixa no alto do app): aparece quando há um áudio tocando (ou pausado) de uma conversa que não está aberta,
 * para ouvir até o fim enquanto se atende outra pessoa. Dentro da própria conversa quem comanda é o balão.
 */
export function AudioBar() {
  const current = useCurrentAudio();
  const open = useMatch("/conversas/:id");
  const navigate = useNavigate();

  // Ao sair do app (sair da conta) o áudio não fica tocando sozinho.
  useEffect(() => () => stopAudio(), []);

  if (!current || open?.params.id === current.track.conversationId) return null;
  const { track, playing, pos, failed } = current;
  const progress = track.durationSec ? Math.min(1, pos / track.durationSec) : 0;

  return (
    // Faixa fixa no alto (empurra a tela para baixo): não cobre a caixa de mensagem nem o cabeçalho da conversa aberta.
    <div role="region" aria-label="Áudio em reprodução" className="shrink-0 border-b border-line bg-panel text-fg">
      <div className="mx-auto flex w-full max-w-xl items-center gap-2 px-3 py-1.5">
        <Mic className="size-4 shrink-0 text-audio-new" aria-hidden />
        <button
          type="button"
          onClick={() => toggleTrack(track)}
          aria-label={playing ? "Pausar áudio" : "Reproduzir áudio"}
          className="inline-flex size-8 shrink-0 items-center justify-center rounded-full hover:bg-black/10 dark:hover:bg-white/10"
        >
          {playing ? <Pause className="size-[18px] fill-current" /> : <Play className="size-[18px] fill-current" />}
        </button>
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs font-medium">{failed ? "Não foi possível tocar este áudio" : `Áudio de ${track.label}`}</p>
          <div
            role="slider"
            tabIndex={0}
            aria-label="Posição do áudio"
            aria-valuemin={0}
            aria-valuemax={Math.round(track.durationSec)}
            aria-valuenow={Math.round(pos)}
            className="group flex h-3 cursor-pointer items-center"
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              seekTrack(track, ((e.clientX - r.left) / r.width) * track.durationSec);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowRight") seekTrack(track, Math.min(track.durationSec, pos + 5));
              if (e.key === "ArrowLeft") seekTrack(track, Math.max(0, pos - 5));
            }}
          >
            <span className="h-1 w-full overflow-hidden rounded-full bg-line">
              <span className={cn("block h-full rounded-full bg-audio-new")} style={{ width: `${progress * 100}%` }} />
            </span>
          </div>
        </div>
        <span className="shrink-0 text-[11px] tabular-nums text-muted">{formatDuration(playing || pos ? pos : track.durationSec)}</span>
        <button
          type="button"
          onClick={() => void navigate(`/conversas/${track.conversationId}?msg=${encodeURIComponent(track.id)}`)}
          aria-label="Ir para a conversa"
          title="Ir para a conversa"
          className="inline-flex size-7 shrink-0 items-center justify-center rounded-full text-muted hover:bg-black/10 hover:text-fg dark:hover:bg-white/10"
        >
          <CornerUpRight className="size-4" />
        </button>
        <button
          type="button"
          onClick={stopAudio}
          aria-label="Fechar"
          title="Fechar"
          className="inline-flex size-7 shrink-0 items-center justify-center rounded-full text-muted hover:bg-black/10 hover:text-fg dark:hover:bg-white/10"
        >
          <X className="size-4" />
        </button>
      </div>
    </div>
  );
}
