import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { motion } from "motion/react";
import { useMediaQuery } from "@/lib/theme";

const WIDTH = 328;
const GAP = 8;

/**
 * Card flutuante ancorado no elemento clicado. Em telas largas abre ao lado do item
 * (e inverte de lado se faltar espaço); em telas pequenas vira uma folha na parte de baixo.
 * Fecha com Esc ou clicando fora.
 */
export function FloatingCard({
  anchor,
  label,
  onClose,
  children,
}: {
  anchor: DOMRect;
  label: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const wide = useMediaQuery("(min-width: 768px)");
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);

  // Posiciona depois de medir a altura real do card.
  useLayoutEffect(() => {
    if (!wide) return;
    const el = ref.current;
    if (!el) return;
    const h = el.offsetHeight;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let left = anchor.right + GAP;
    if (left + WIDTH > vw - GAP) left = anchor.left - WIDTH - GAP;
    if (left < GAP) left = Math.max(GAP, Math.min(vw - WIDTH - GAP, anchor.left));
    const top = Math.max(GAP, Math.min(anchor.top - 8, vh - h - GAP));
    setPos({ left, top });
  }, [anchor, wide, children]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const onDown = (e: MouseEvent) => {
      if (wide && ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [onClose, wide]);

  useEffect(() => ref.current?.focus(), []);

  const card = (
    <motion.div
      ref={ref}
      role="dialog"
      aria-label={label}
      tabIndex={-1}
      initial={{ opacity: 0, scale: 0.96, y: wide ? 0 : 24 }}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      transition={{ type: "spring", stiffness: 520, damping: 34 }}
      style={
        wide
          ? { position: "fixed", width: WIDTH, left: pos?.left ?? -9999, top: pos?.top ?? -9999, visibility: pos ? "visible" : "hidden" }
          : undefined
      }
      className={
        wide
          ? "z-50 max-h-[calc(100vh-16px)] overflow-y-auto rounded-xl border border-line bg-panel text-fg shadow-2xl outline-none"
          : "fixed inset-x-0 bottom-0 z-50 max-h-[85vh] overflow-y-auto rounded-t-2xl border-t border-line bg-panel pb-[env(safe-area-inset-bottom)] text-fg shadow-2xl outline-none"
      }
    >
      {children}
    </motion.div>
  );

  return createPortal(
    wide ? (
      card
    ) : (
      <>
        <button type="button" aria-label="Fechar" className="fixed inset-0 z-40 bg-black/40" onClick={onClose} />
        {card}
      </>
    ),
    document.body,
  );
}
