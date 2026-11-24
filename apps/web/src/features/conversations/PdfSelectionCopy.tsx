import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/cn";
import { copyPopupPlace, lastLineBox, type CopyPopupPlace } from "./pdfSelectionCopy";

type Pop = CopyPopupPlace & { text: string };

/**
 * Botão "Copiar" que aparece junto do texto selecionado no PDF (como no Edge), em vez de precisar do botão direito.
 * Só para mouse e caneta: no toque o próprio celular já mostra "Copiar". Some ao clicar fora, rolar, apertar Esc ou limpar a seleção.
 */
export function SelectionCopyButton({ root, copy }: { root: HTMLElement | null; copy: (text: string) => Promise<boolean> }) {
  const [pop, setPop] = useState<Pop | null>(null);
  const [done, setDone] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (!root) return;
    const hide = () => {
      window.clearTimeout(timer.current);
      setPop(null);
      setDone(false);
    };
    const show = () => {
      const sel = document.getSelection();
      if (!sel || sel.isCollapsed || sel.rangeCount === 0) return hide();
      const range = sel.getRangeAt(0);
      const text = sel.toString().trim();
      if (!text || !root.contains(range.commonAncestorContainer)) return hide();
      const last = lastLineBox(Array.from(range.getClientRects()));
      if (!last) return hide();
      setDone(false);
      setPop({ ...copyPopupPlace(last, window.innerWidth, root.getBoundingClientRect().top), text });
    };
    // O arrasto termina no pointerup; a seleção só está completa um instante depois.
    const onUp = (e: PointerEvent) => {
      if (e.pointerType === "touch" || e.button !== 0) return;
      window.setTimeout(show, 0);
    };
    const onDown = (e: PointerEvent) => {
      if (e.target instanceof Node && button.current?.contains(e.target)) return; // o clique no botão não derruba a seleção
      hide();
    };
    const onChange = () => {
      const sel = document.getSelection();
      if (!sel || sel.isCollapsed) hide();
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && hide();
    root.addEventListener("pointerdown", onDown);
    window.addEventListener("pointerup", onUp);
    root.addEventListener("scroll", hide);
    window.addEventListener("resize", hide);
    document.addEventListener("selectionchange", onChange);
    document.addEventListener("keydown", onKey);
    return () => {
      window.clearTimeout(timer.current);
      root.removeEventListener("pointerdown", onDown);
      window.removeEventListener("pointerup", onUp);
      root.removeEventListener("scroll", hide);
      window.removeEventListener("resize", hide);
      document.removeEventListener("selectionchange", onChange);
      document.removeEventListener("keydown", onKey);
    };
  }, [root]);

  if (!pop) return null;
  return createPortal(
    <button
      ref={button}
      type="button"
      // Não tira o foco nem a seleção do texto ao apertar.
      onPointerDown={(e) => e.preventDefault()}
      onClick={async () => {
        if (await copy(pop.text)) {
          setDone(true);
          window.clearTimeout(timer.current);
          timer.current = window.setTimeout(() => {
            setPop(null);
            setDone(false);
          }, 900);
        }
      }}
      aria-label="Copiar texto selecionado"
      style={{ left: pop.x, top: pop.y }}
      className={cn(
        "fixed z-[110] flex -translate-x-1/2 items-center gap-2 rounded-lg border border-line bg-panel px-3 py-2 text-[13px] font-medium text-fg shadow-xl hover:bg-hover",
        pop.below ? "" : "-translate-y-full",
      )}
    >
      {done ? <Check className="size-4 text-emerald-500" /> : <Copy className="size-4" />}
      {done ? "Copiado" : "Copiar"}
    </button>,
    document.body,
  );
}
