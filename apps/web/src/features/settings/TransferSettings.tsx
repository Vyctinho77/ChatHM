import { useState } from "react";
import { getShareReminderAlways, setShareReminderAlways } from "@/features/conversations/shareReminderPref";

export function TransferSettings({ userId }: { userId: string }) {
  const [always, setAlways] = useState(() => getShareReminderAlways(userId));
  return <div className="flex flex-col gap-3">
    <label className="flex cursor-pointer items-start justify-between gap-4 text-sm">
      <span><span className="block font-medium">Levar meu lembrete ao transferir, sem perguntar</span><span className="mt-1 block text-xs text-muted">Desligado: o ChatHM pergunta antes de compartilhar o lembrete com quem recebe a conversa.</span></span>
      <input type="checkbox" className="mt-1 size-4 shrink-0" checked={always} onChange={(e) => { setAlways(e.target.checked); setShareReminderAlways(userId, e.target.checked); }} />
    </label>
    <p className="text-xs text-muted">Preferência salva neste navegador para sua conta.</p>
  </div>;
}
