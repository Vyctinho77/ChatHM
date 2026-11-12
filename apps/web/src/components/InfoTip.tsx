import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Info } from "lucide-react";
import { cn } from "@/lib/cn";

const WIDTH = 320; // largura máxima do balão (px)
const GAP = 8;
const MARGIN = 12;

/**
 * Botão "i" que abre um balão com a explicação. Serve para tirar texto de apoio da tela: o que importa fica visível e quem
 * tem dúvida abre o "i". Abre por clique/toque ou Enter (nada depende de passar o mouse), fecha com Esc, clique fora ou
 * rolagem, e devolve o foco ao botão. O balão fica no <body> (posição fixa) para não ser cortado por listas, <details> ou
 * janelas com overflow, e é mantido dentro da tela no celular.
 */
export function InfoTip({
  title,
  children,
  label,
  className,
}: {
  /** Título curto do balão (também nomeia o botão para leitores de tela). */
  title: string;
  children: ReactNode;
  /** Texto do botão para leitores de tela; padrão: "Saiba mais: {title}". */
  label?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number; width: number } | null>(null);
  const [host, setHost] = useState<HTMLElement | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const id = useId();

  // Posição: abaixo do botão (ou acima, se não couber), alinhada para caber na largura da tela.
  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const r = trigger.current.getBoundingClientRect();
    const width = Math.min(WIDTH, window.innerWidth - MARGIN * 2);
    const left = Math.min(Math.max(MARGIN, r.left + r.width / 2 - width / 2), window.innerWidth - width - MARGIN);
    setPos({ left, top: r.bottom + GAP, width });
  }, [open]);
  useLayoutEffect(() => {
    if (!open || !pos || !panel.current || !trigger.current) return;
    const h = panel.current.offsetHeight;
    const r = trigger.current.getBoundingClientRect();
    if (pos.top + h > window.innerHeight - MARGIN && r.top - GAP - h > MARGIN) setPos({ ...pos, top: r.top - GAP - h });
  }, [open, pos]);

  useEffect(() => {
    if (!open) return;
    panel.current?.focus({ preventScroll: true });
    const close = () => setOpen(false);
    const outside = (e: PointerEvent) => {
      const t = e.target as Node;
      if (!panel.current?.contains(t) && !trigger.current?.contains(t)) close();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation(); // dentro de uma janela, fecha só o balão
      close();
      trigger.current?.focus({ preventScroll: true });
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", key, true);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", close, true);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", key, true);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [open]);

  return (
    <>
      <button
        ref={trigger}
        type="button"
        aria-label={label ?? `Saiba mais: ${title}`}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setPos(null);
          // Dentro de uma janela (<dialog>) o balão precisa ficar dentro dela, senão a janela (camada superior) o cobre.
          setHost(trigger.current?.closest("dialog") ?? document.body);
          setOpen((v) => !v);
        }}
        className={cn(
          "inline-flex size-6 shrink-0 items-center justify-center rounded-full text-muted outline-none transition-colors hover:bg-hover hover:text-brand-ink focus-visible:ring-2 focus-visible:ring-brand motion-reduce:transition-none",
          open && "bg-hover text-brand-ink",
          className,
        )}
      >
        <Info className="size-4" aria-hidden />
      </button>
      {open && host &&
        createPortal(
          <div
            ref={panel}
            id={id}
            role="dialog"
            aria-label={title}
            tabIndex={-1}
            style={{ left: pos?.left ?? 0, top: pos?.top ?? 0, width: pos?.width ?? WIDTH, visibility: pos ? "visible" : "hidden" }}
            className="fixed z-[70] max-h-[min(24rem,calc(100dvh-1.5rem))] overflow-y-auto rounded-xl border border-line bg-panel p-4 text-fg shadow-xl outline-none"
          >
            <p className="text-[13px] font-semibold">{title}</p>
            <div className="mt-1.5 space-y-2 text-xs leading-relaxed text-muted [&_code]:rounded [&_code]:bg-elevated [&_code]:px-1 [&_strong]:font-medium [&_strong]:text-fg">{children}</div>
          </div>,
          host,
        )}
    </>
  );
}

/** Título de seção com o "i" ao lado. */
export function HeadingWithInfo({ children, info, infoTitle, as: Tag = "h2", className }: { children: ReactNode; info: ReactNode; infoTitle: string; as?: "h2" | "h3"; className?: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <Tag className={className ?? "text-sm font-semibold"}>{children}</Tag>
      <InfoTip title={infoTitle}>{info}</InfoTip>
    </div>
  );
}
