import { loadExtendedDictionary } from "./autocorrect";
import { loadSpellDictionary } from "./spell";
import { loadTypoDictionary } from "./typos";

/**
 * Prepara o corretor sem atrapalhar a tela: os dicionários (3,3 MB de palavras, ~30 mil palavras frequentes, regras) carregam UMA vez, quando o navegador
 * está ocioso (ou em até 2,5 s), um de cada vez, e o vocabulário é montado em fatias. Assim a primeira digitação não encontra um travamento.
 * Chamar de novo devolve a mesma promessa.
 */
let started: Promise<void> | null = null;

type IdleWindow = Window & { requestIdleCallback?: (cb: () => void, o?: { timeout: number }) => number };
const whenIdle = (fn: () => void) => {
  const w = window as IdleWindow;
  if (w.requestIdleCallback) w.requestIdleCallback(fn, { timeout: 2500 });
  else setTimeout(fn, 300);
};
const tick = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

export function warmUpSpelling(): Promise<void> {
  started ??= new Promise<void>((resolve) => {
    whenIdle(() => {
      void (async () => {
        await loadSpellDictionary();
        await tick();
        await loadTypoDictionary();
        await tick();
        await loadExtendedDictionary();
        resolve();
      })();
    });
  });
  return started;
}
