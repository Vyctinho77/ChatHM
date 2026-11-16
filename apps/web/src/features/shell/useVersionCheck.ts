import { useEffect } from "react";
import { dismissNotice, pushNotice } from "@/lib/notices";
import { bundleFromHtml, runningBundle, shouldAnnounce } from "@/lib/versionCheck";

const EVERY_MS = 5 * 60_000; // confere de tempos em tempos
const MIN_GAP_MS = 60_000; // e ao voltar para a aba, mas no máximo uma vez por minuto

/** Avisa, uma vez, quando o servidor tem uma versão mais nova do app do que a que esta aba está rodando. */
export function useVersionCheck(enabled: boolean) {
  useEffect(() => {
    const running = runningBundle();
    if (!enabled || !running) return; // desenvolvimento e demonstração não têm versão publicada
    let dismissed: string | null = null;
    let last = 0;
    let stopped = false;

    async function check() {
      if (stopped || document.visibilityState !== "visible" || Date.now() - last < MIN_GAP_MS) return;
      last = Date.now();
      try {
        const res = await fetch("/", { cache: "no-store", credentials: "same-origin" });
        if (!res.ok) return;
        const latest = bundleFromHtml(await res.text());
        if (!shouldAnnounce(running, latest, dismissed)) return;
        pushNotice({
          id: "app-version",
          sticky: true,
          text: "Há uma nova versão do ChatHM.",
          action: { label: "Atualizar", run: () => window.location.reload() },
          onDismiss: () => {
            dismissed = latest;
          },
        });
      } catch {
        /* sem rede agora: tenta na próxima */
      }
    }

    const timer = window.setInterval(() => void check(), EVERY_MS);
    const onVisible = () => void check();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
      dismissNotice("app-version");
    };
  }, [enabled]);
}
