import { useState } from "react";
import { Play, Volume2, VolumeX } from "lucide-react";
import { Button } from "@/components/ui";
import { notificationTones, previewNotification, updateSoundPreferences, useSoundPreferences, type NotificationTone } from "@/lib/sounds";

export function SoundSettings() {
  const preferences = useSoundPreferences();
  const [previewing, setPreviewing] = useState(false);
  const [error, setError] = useState("");
  return <div className="flex flex-col gap-5">
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span><span className="block font-medium">Sons da conversa</span><span className="mt-1 block text-xs text-muted">Ao enviar e receber mensagens na conversa aberta.</span></span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={preferences.messages} onChange={(e) => updateSoundPreferences({ messages: e.target.checked })} />
    </label>
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span><span className="block font-medium">Sons de notificação</span><span className="mt-1 block text-xs text-muted">Novas mensagens fora da conversa aberta.</span></span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={preferences.notifications} onChange={(e) => updateSoundPreferences({ notifications: e.target.checked })} />
    </label>
    <div className="flex flex-wrap items-end gap-3">
      <label className="flex min-w-40 flex-1 flex-col gap-2 text-sm font-medium">
        Som da notificação
        <select value={preferences.tone} onChange={(e) => { updateSoundPreferences({ tone: e.target.value as NotificationTone }); setError(""); }} className="h-9 rounded-lg border border-line bg-elevated px-3 text-sm font-normal outline-none focus:border-brand-ink">
          {Object.entries(notificationTones).map(([id, tone]) => <option key={id} value={id}>{tone.label}</option>)}
        </select>
      </label>
      <Button variant="secondary" loading={previewing} disabled={preferences.volume === 0} onClick={async () => {
        setPreviewing(true);
        setError("");
        try { if (!(await previewNotification(preferences.tone))) setError("Não foi possível reproduzir o som neste navegador."); }
        finally { setPreviewing(false); }
      }}><Play className="size-4" /> Ouvir</Button>
    </div>
    <label className="flex flex-col gap-2 text-sm">
      <span className="flex items-center justify-between"><span className="flex items-center gap-2 font-medium">{preferences.volume ? <Volume2 className="size-4 text-muted" /> : <VolumeX className="size-4 text-muted" />}Volume</span><span className="text-xs tabular-nums text-muted">{Math.round(preferences.volume * 100)}%</span></span>
      <input type="range" min={0} max={100} step={1} value={Math.round(preferences.volume * 100)} onChange={(e) => updateSoundPreferences({ volume: Number(e.target.value) / 100 })} className="w-full accent-accent" />
    </label>
    {error && <p role="alert" className="text-xs text-danger">{error}</p>}
    <p className="text-xs text-muted">Preferências salvas neste navegador para sua conta.</p>
  </div>;
}
