import { useLayoutEffect, useMemo, useRef, type ChangeEvent, type KeyboardEvent, type RefObject } from "react";
import { AccentController } from "./accentController";
import { loadExtendedDictionary } from "./autocorrect";
import { startStatsReporter } from "./autocorrectStats";
import { loadTypoDictionary } from "./typos";
import { ignoredWords, recordUndo } from "./writingPrefs";

/**
 * Corretor de acentos para um <textarea> controlado (interno: não tem tela de configuração). Uso:
 *   const accent = useAccentTextarea({ ref, onValue: (v) => setText(v) });
 *   <textarea ref={ref} value={text} onChange={accent.onChange} onKeyDown={(e) => { if (accent.onKeyDown(e)) return; ... }} {...accent.attrs} />
 * e, ao enviar, `accent.finish(texto)` corrige a última palavra.
 */
export function useAccentTextarea(o: { ref: RefObject<HTMLTextAreaElement | null>; onValue: (value: string, caret: number) => void }) {
  const controller = useMemo(() => new AccentController(() => ({ ignored: ignoredWords() }), (w) => void recordUndo(w)), []);
  const pending = useRef<number | null>(null);

  // O dicionário estendido carrega uma vez, em segundo plano, na primeira vez que um campo de texto aparece.
  useLayoutEffect(() => {
    void loadExtendedDictionary();
    void loadTypoDictionary();
    startStatsReporter();
  }, []);

  // Trocar o texto por código move o cursor para o fim: devolve-o para onde estava.
  useLayoutEffect(() => {
    const el = o.ref.current;
    if (pending.current === null || !el) return;
    el.setSelectionRange(pending.current, pending.current);
    pending.current = null;
  });

  return {
    onChange(e: ChangeEvent<HTMLTextAreaElement>) {
      const el = e.currentTarget;
      const ev = e.nativeEvent as InputEvent;
      const res = controller.input(el.value, el.selectionStart, { inputType: ev.inputType, data: ev.data, isComposing: ev.isComposing, selectionEnd: el.selectionEnd });
      if (res.fixed) pending.current = res.caret;
      o.onValue(res.value, res.caret);
    },
    /** true = o corretor tratou a tecla (desfez a correção); quem chama deve parar. */
    onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>): boolean {
      if (e.key !== "Backspace" || e.nativeEvent.isComposing) return false;
      const el = e.currentTarget;
      const back = controller.backspace(el.value, el.selectionStart, el.selectionEnd);
      if (!back) return false;
      e.preventDefault();
      pending.current = back.caret;
      o.onValue(back.value, back.caret);
      return true;
    },
    finish: (text: string) => controller.finish(text),
    reset: () => controller.reset(),
    attrs: { spellCheck: true, lang: "pt-BR", autoCorrect: "on" } as const,
  };
}
