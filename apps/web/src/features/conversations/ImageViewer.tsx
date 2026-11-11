import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { motion } from "motion/react";
import { ChevronLeft, ChevronRight, Download, RotateCcw, RotateCw, X, ZoomIn, ZoomOut } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { cn } from "@/lib/cn";
import { previewUrl } from "@/lib/media";
import { formatDayDivider, formatTime } from "@/lib/format";
import type { Message } from "./types";

export type ViewableImage = Extract<Message, { kind: "image" }> & { url: string };

const MIN_ZOOM = 1;
const MAX_ZOOM = 4;

/** "Hoje às 20:18", "Ontem às 09:02" ou "04/11/2026 às 15:40". */
const stamp = (iso: string) => `${formatDayDivider(iso)} às ${formatTime(iso)}`;

/**
 * Visualizador de imagens em tela cheia: navega entre todas as imagens da conversa
 * (setas, miniaturas, teclado), amplia, arrasta, gira e baixa — sem sair do app.
 * Girar só muda como a imagem aparece aqui; o arquivo original não é alterado.
 */
export function ImageViewer({
  images,
  startId,
  contactName,
  onClose,
}: {
  images: ViewableImage[];
  startId: string;
  contactName: string;
  onClose: () => void;
}) {
  const [id, setId] = useState(startId);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const thumbs = useRef<HTMLDivElement>(null);
  // Giro de cada imagem, em graus acumulados (múltiplos de 90): acumular evita a imagem "desgirar" ao passar de 270° para 0°.
  const [turns, setTurns] = useState<Record<string, number>>({});
  const stage = useRef<HTMLDivElement>(null);
  const [box, setBox] = useState({ w: 0, h: 0 }); // área disponível para a imagem
  const [natural, setNatural] = useState<{ id: string; w: number; h: number } | null>(null);

  const index = images.findIndex((m) => m.id === id);
  const current = images[index];

  // Abre na hora com a versão média (já está no cache, veio do balão) e troca pelo original assim que ele terminar de baixar.
  const [fullFor, setFullFor] = useState<string | null>(null);
  const currentId = current?.id;
  const currentUrl = current?.url;
  useEffect(() => {
    if (!currentId || !currentUrl) return;
    let alive = true;
    const img = new Image();
    img.onload = () => {
      if (alive) setFullFor(currentId);
    };
    img.src = currentUrl;
    return () => {
      alive = false;
    };
  }, [currentId, currentUrl]);

  // A imagem aberta saiu da conversa (ex.: mensagem removida): fecha em vez de mostrar vazio.
  useEffect(() => {
    if (!current) onClose();
  }, [current, onClose]);

  // Mede a área da imagem (muda com a janela) para encaixar a imagem girada de lado.
  useEffect(() => {
    const el = stage.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const measure = () => {
      const cs = getComputedStyle(el);
      setBox({
        w: el.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight),
        h: el.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom),
      });
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [current]);

  const rotate = useCallback(
    (delta: number) => {
      if (!id) return;
      setTurns((t) => ({ ...t, [id]: (t[id] ?? 0) + delta }));
      setPan({ x: 0, y: 0 });
    },
    [id],
  );

  const resetView = () => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
  };
  const go = useCallback(
    (delta: number) => {
      const next = images[index + delta];
      if (!next) return;
      setId(next.id);
      setZoom(1);
      setPan({ x: 0, y: 0 });
    },
    [images, index],
  );
  const zoomBy = useCallback((delta: number) => {
    setZoom((z) => {
      const next = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.round((z + delta) * 100) / 100));
      if (next === MIN_ZOOM) setPan({ x: 0, y: 0 });
      return next;
    });
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft") go(-1);
      else if (e.key === "ArrowRight") go(1);
      else if (e.key === "+" || e.key === "=") zoomBy(0.5);
      else if (e.key === "-") zoomBy(-0.5);
      else if (e.key === "r") rotate(90);
      else if (e.key === "R") rotate(-90);
      else return;
      e.preventDefault();
      e.stopPropagation();
    };
    window.addEventListener("keydown", onKey, true);
    // Trava a rolagem da página por trás do visualizador.
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey, true);
      document.body.style.overflow = overflow;
    };
  }, [go, onClose, zoomBy, rotate]);

  // Mantém a miniatura da imagem atual visível.
  useEffect(() => {
    thumbs.current?.querySelector<HTMLElement>("[aria-current=true]")?.scrollIntoView({ inline: "center", block: "nearest" });
  }, [id]);

  if (!current) return null;

  const onPointerDown = (e: ReactPointerEvent) => {
    if (zoom === 1) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y };
    setDragging(true);
  };
  const onPointerMove = (e: ReactPointerEvent) => {
    const d = drag.current;
    if (d) setPan({ x: d.px + e.clientX - d.x, y: d.py + e.clientY - d.y });
  };
  const endDrag = () => {
    drag.current = null;
    setDragging(false);
  };
  const sender = current.direction === "out" ? "Você" : contactName;
  const degrees = turns[current.id] ?? 0;
  const sideways = Math.abs(Math.round(degrees / 90)) % 2 === 1;
  // De lado, a largura vira altura: encolhe (ou amplia, até o tamanho original) para a imagem girada caber na área.
  let fit = 1;
  if (sideways && natural?.id === current.id && box.w > 0 && box.h > 0) {
    const s0 = Math.min(1, box.w / natural.w, box.h / natural.h); // tamanho em que a imagem aparece sem girar
    fit = Math.min(box.w / (natural.h * s0), box.h / (natural.w * s0), 1 / s0);
  }
  const iconBtn =
    "inline-flex size-10 items-center justify-center rounded-full text-white/85 transition-colors hover:bg-white/10 hover:text-white disabled:pointer-events-none disabled:opacity-30";

  return createPortal(
    <motion.div
      role="dialog"
      aria-modal="true"
      aria-label={`Imagem de ${sender}`}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.15 }}
      className="fixed inset-0 z-[100] flex flex-col bg-[#0b0d10]/[0.97] text-white"
    >
      <header className="flex shrink-0 items-center gap-3 px-4 py-3">
        <Avatar name={sender} size="md" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{sender}</p>
          <p className="text-xs text-white/60">{stamp(current.sentAt)}</p>
        </div>
        <button type="button" className={iconBtn} onClick={() => zoomBy(-0.5)} disabled={zoom <= MIN_ZOOM} aria-label="Reduzir" title="Reduzir (−)">
          <ZoomOut className="size-5" />
        </button>
        <button type="button" className={iconBtn} onClick={() => zoomBy(0.5)} disabled={zoom >= MAX_ZOOM} aria-label="Ampliar" title="Ampliar (+)">
          <ZoomIn className="size-5" />
        </button>
        <button type="button" className={iconBtn} onClick={() => rotate(-90)} aria-label="Girar para a esquerda" title="Girar para a esquerda (Shift+R)">
          <RotateCcw className="size-5" />
        </button>
        <button type="button" className={iconBtn} onClick={() => rotate(90)} aria-label="Girar para a direita" title="Girar para a direita (R)">
          <RotateCw className="size-5" />
        </button>
        <a href={current.url} download="" className={iconBtn} aria-label="Baixar imagem" title="Baixar">
          <Download className="size-5" />
        </a>
        <button type="button" className={iconBtn} onClick={onClose} aria-label="Fechar" title="Fechar (Esc)">
          <X className="size-5" />
        </button>
      </header>

      {/* Clicar no fundo fecha; clicar na imagem não. */}
      <div
        ref={stage}
        className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden px-16"
        onClick={(e) => e.target === e.currentTarget && onClose()}
        onWheel={(e) => zoomBy(e.deltaY < 0 ? 0.25 : -0.25)}
      >
        <button
          type="button"
          className={cn(iconBtn, "absolute left-3 top-1/2 z-10 -translate-y-1/2 bg-white/10")}
          onClick={() => go(-1)}
          disabled={index <= 0}
          aria-label="Imagem anterior"
        >
          <ChevronLeft className="size-6" />
        </button>
        <img
          key={current.id}
          src={fullFor === current.id ? current.url : previewUrl(current.url)}
          alt={current.caption ?? "Imagem"}
          draggable={false}
          onLoad={(e) => setNatural({ id: current.id, w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
          onDoubleClick={() => (zoom === 1 ? setZoom(2) : resetView())}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
          style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom * fit}) rotate(${degrees}deg)` }}
          className={cn(
            "max-h-full max-w-full select-none object-contain transition-transform duration-150",
            dragging && "!transition-none",
            zoom > 1 ? "cursor-grab active:cursor-grabbing" : "cursor-zoom-in",
          )}
        />
        <button
          type="button"
          className={cn(iconBtn, "absolute right-3 top-1/2 z-10 -translate-y-1/2 bg-white/10")}
          onClick={() => go(1)}
          disabled={index >= images.length - 1}
          aria-label="Próxima imagem"
        >
          <ChevronRight className="size-6" />
        </button>
      </div>

      {current.caption && <p className="shrink-0 px-6 py-2 text-center text-sm text-white/85">{current.caption}</p>}

      {images.length > 1 && (
        <div ref={thumbs} className="flex shrink-0 gap-2 overflow-x-auto border-t border-white/10 px-4 py-3 [&>:first-child]:ml-auto [&>:last-child]:mr-auto">
          {images.map((m) => (
            <button
              key={m.id}
              type="button"
              aria-current={m.id === id}
              aria-label={`Imagem de ${stamp(m.sentAt)}`}
              onClick={() => {
                setId(m.id);
                resetView();
              }}
              className={cn(
                "size-14 shrink-0 overflow-hidden rounded-md border-2 transition-opacity",
                m.id === id ? "border-emerald-400" : "border-transparent opacity-60 hover:opacity-100",
              )}
            >
              <img src={previewUrl(m.url)} alt="" loading="lazy" decoding="async" className="size-full object-cover" style={turns[m.id] ? { transform: `rotate(${turns[m.id]}deg)` } : undefined} draggable={false} />
            </button>
          ))}
        </div>
      )}
    </motion.div>,
    document.body,
  );
}
