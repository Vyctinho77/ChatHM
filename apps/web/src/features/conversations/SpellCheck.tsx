import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { createPortal } from "react-dom";
import type { Unknown } from "./spell";

/** A camada por trás do texto: o mesmo texto, transparente, com as palavras que não existem sublinhadas. */
export function SpellBackdrop({ text, unknown, backdrop, className }: { text: string; unknown: Unknown[]; backdrop: RefObject<HTMLDivElement | null>; className: string }) {
  const parts: React.ReactNode[] = [];
  let at = 0;
  unknown.forEach((u, i) => {
    parts.push(text.slice(at, u.start));
    parts.push(<span key={u.start} data-spell={i} className="underline decoration-red-500 decoration-wavy decoration-1 underline-offset-[3px]">{text.slice(u.start, u.end)}</span>);
    at = u.end;
  });
  parts.push(text.slice(at));
  return (
    <div ref={backdrop} aria-hidden="true" className={`pointer-events-none absolute inset-0 overflow-hidden whitespace-pre-wrap break-words text-transparent ${className}`}>
      {parts}
      {" "}
    </div>
  );
}

/** O cartão do botão direito: a palavra, as sugestões e "Ignorar". */
export function SpellCard({ x, y, word, options, onPick, onIgnore, onClose }: { x: number; y: number; word: string; options: string[]; onPick: (to: string) => void; onIgnore: () => void; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });

  // Fica dentro da tela.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({ left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)), top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) });
    el.querySelector<HTMLElement>("button")?.focus();
  }, [x, y, options.length]);

  useEffect(() => {
    const onDown = (e: Event) => {
      if (!box.current?.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const items = [...(box.current?.querySelectorAll<HTMLElement>("button") ?? [])];
        const i = items.indexOf(document.activeElement as HTMLElement);
        items[(i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length]?.focus();
      }
    };
    document.addEventListener("mousedown", onDown, true);
    document.addEventListener("keydown", onKey, true);
    window.addEventListener("blur", onClose);
    window.addEventListener("resize", onClose);
    window.addEventListener("scroll", onClose, true);
    return () => {
      document.removeEventListener("mousedown", onDown, true);
      document.removeEventListener("keydown", onKey, true);
      window.removeEventListener("blur", onClose);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("scroll", onClose, true);
    };
  }, [onClose]);

  return createPortal(
    <div ref={box} role="menu" aria-label={`Sugestões para ${word}`} style={{ left: pos.left, top: pos.top }} className="fixed z-[70] w-60 overflow-hidden rounded-xl border border-line bg-panel py-1 text-[13px] shadow-xl">
      <p className="truncate px-3 pb-1 pt-1.5 text-[11px] text-muted">“{word}” não está no dicionário</p>
      {options.length === 0 && <p className="px-3 py-1.5 text-muted">Sem sugestões</p>}
      {options.map((o) => (
        <button key={o} type="button" role="menuitem" onClick={() => onPick(o)} className="block w-full px-3 py-1.5 text-left font-medium hover:bg-hover focus-visible:bg-hover focus-visible:outline-none">{o}</button>
      ))}
      <div className="my-1 border-t border-line/60" />
      <button type="button" role="menuitem" onClick={onIgnore} className="block w-full px-3 py-1.5 text-left text-muted hover:bg-hover hover:text-fg focus-visible:bg-hover focus-visible:outline-none">Ignorar esta palavra</button>
    </div>,
    document.body,
  );
}
