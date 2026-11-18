import { ignoredWords } from "./writingPrefs";
import { applyFix, fixTail, fixWordEndingAt, isTrigger, undoFix, type CorrectOptions, type Fix, type Rejected } from "./autocorrect";

/**
 * O "motor" do corretor para um campo de texto, sem React: recebe o que a pessoa digitou e devolve o texto (possivelmente corrigido) e onde
 * fica o cursor. Quem usa (useAccentTextarea) só liga isto aos eventos do campo.
 */
export type InputInfo = { inputType: string; data: string | null; isComposing: boolean; selectionEnd: number };
export type AccentResult = { value: string; caret: number; fixed: boolean };
type Last = Fix & { caret: number; delimiter: string };

export class AccentController {
  private last: Last | null = null;
  private rejected: Rejected = null;

  /** `options` é lida a cada digitação (as preferências podem mudar com o campo aberto); `onUndo` avisa quando a pessoa desfaz uma correção. */
  constructor(private options: () => CorrectOptions, private onUndo?: (word: string) => void) {}

  /** O campo mudou (`caret` já é a posição depois da digitação). Fechou uma palavra? Corrige. */
  input(value: string, caret: number, ev: InputInfo): AccentResult {
    this.last = null;
    const o = this.options();
    if (!ev.isComposing && (ev.inputType === "insertText" || ev.inputType === "insertLineBreak") && ev.selectionEnd === caret) {
      const typed = ev.inputType === "insertLineBreak" ? "\n" : ev.data ?? "";
      if (isTrigger(typed) && caret > 0 && value.charAt(caret - 1) === typed) {
        const fix = fixWordEndingAt(value, caret - 1, this.rejected, o);
        if (fix) {
          const done = applyFix(value, fix, caret);
          this.last = { ...fix, caret: done.caret, delimiter: typed };
          return { value: done.text, caret: done.caret, fixed: true };
        }
      }
    }
    return { value, caret, fixed: false };
  }

  /** Backspace logo depois de uma correção devolve a palavra como foi digitada. Null = não é o caso (o campo apaga normalmente). */
  backspace(value: string, selStart: number, selEnd: number): { value: string; caret: number } | null {
    const fix = this.last;
    if (!fix || selStart !== fix.caret || selEnd !== fix.caret) return null;
    if (value.charAt(fix.caret - 1) !== fix.delimiter || value.slice(fix.start, fix.start + fix.to.length) !== fix.to) return null;
    const back = undoFix(value, fix, fix.caret);
    this.rejected = { start: fix.start, word: fix.from.toLowerCase() };
    this.last = null;
    this.onUndo?.(fix.from);
    return { value: back.text, caret: back.caret };
  }

  /** O texto na hora de enviar: corrige a última palavra, que ainda não foi "fechada". */
  finish(text: string): string {
    return fixTail(text, this.rejected, this.options());
  }

  /** Enviou ou trocou de conversa: esquece a última correção. */
  reset() {
    this.last = null;
    this.rejected = null;
  }
}

/** Para quem envia um texto sem passar pelo campo (legenda, lembrete): corrige a última palavra com o que o corretor já aprendeu. */
export function finishWithPrefs(text: string): string {
  return fixTail(text, undefined, { ignored: ignoredWords() });
}
