import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { motion } from "motion/react";
import { getDocument, GlobalWorkerOptions, TextLayer, type PDFDocumentLoadingTask, type PDFDocumentProxy, type RenderTask } from "pdfjs-dist";
import workerSrc from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { Download, LoaderCircle, X, ZoomIn, ZoomOut } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatBytes } from "@/lib/format";
import type { Message } from "./types";
import "./pdf-text-layer.css";

GlobalWorkerOptions.workerSrc = workerSrc;

export type ViewablePdf = Extract<Message, { kind: "document" }> & { url: string };

const ZOOMS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3];

type Load = { state: "loading" } | { state: "error"; message: string } | { state: "ready"; doc: PDFDocumentProxy; ratio: number };

/** Uma página: só é desenhada quando chega perto da área visível. */
function PdfPage({
  doc,
  num,
  width,
  ratio,
  root,
}: {
  doc: PDFDocumentProxy;
  num: number;
  width: number;
  ratio: number; // altura/largura estimada (da 1ª página) até a página real carregar
  root: HTMLElement | null;
}) {
  const wrap = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const text = useRef<HTMLDivElement>(null);
  const [near, setNear] = useState(false);
  const [own, setOwn] = useState<number | null>(null);

  useEffect(() => {
    const el = wrap.current;
    if (!el || !root) return;
    const io = new IntersectionObserver(([e]) => e && setNear(e.isIntersecting), { root, rootMargin: "1200px 0px" });
    io.observe(el);
    return () => io.disconnect();
  }, [root]);

  useEffect(() => {
    if (!near || width <= 0) return;
    let cancelled = false;
    let task: RenderTask | undefined;
    let layer: TextLayer | undefined;
    void (async () => {
      try {
        const page = await doc.getPage(num);
        if (cancelled) return;
        const scale = width / page.getViewport({ scale: 1 }).width;
        const viewport = page.getViewport({ scale });
        setOwn(viewport.height / viewport.width);
        const c = canvas.current;
        const t = text.current;
        if (!c || !t) return;
        const dpr = window.devicePixelRatio || 1;
        c.width = Math.floor(viewport.width * dpr);
        c.height = Math.floor(viewport.height * dpr);
        task = page.render({ canvas: c, viewport, transform: dpr !== 1 ? [dpr, 0, 0, dpr, 0, 0] : undefined });
        await task.promise;
        if (cancelled) return;
        t.replaceChildren();
        t.style.setProperty("--total-scale-factor", String(scale));
        layer = new TextLayer({ textContentSource: page.streamTextContent(), container: t, viewport });
        await layer.render();
      } catch {
        /* renderização cancelada (zoom/fechar) ou página ilegível: fica em branco */
      }
    })();
    return () => {
      cancelled = true;
      task?.cancel();
      layer?.cancel();
    };
  }, [near, width, doc, num]);

  return (
    <div
      ref={wrap}
      data-page={num}
      className="relative shrink-0 bg-white shadow-lg"
      style={{ width, height: Math.round(width * (own ?? ratio)) }}
    >
      <canvas ref={canvas} className="absolute inset-0 size-full" />
      <div
        ref={text}
        className="pdf-text-layer"
        onPointerDown={() => {
          const el = text.current;
          el?.classList.add("selecting");
          window.addEventListener("pointerup", () => el?.classList.remove("selecting"), { once: true });
        }}
      />
    </div>
  );
}

/**
 * Visualizador de PDF em tela cheia, dentro do app: páginas rolando, zoom e texto
 * selecionável/copiável (camada de texto do pdf.js). Só o núcleo do pdf.js é usado: scripts do PDF não rodam.
 */
export function PdfViewer({ pdf, onClose }: { pdf: ViewablePdf; onClose: () => void }) {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  const [zoomIdx, setZoomIdx] = useState(2);
  const [box, setBox] = useState<HTMLDivElement | null>(null);
  const [boxWidth, setBoxWidth] = useState(0);
  const [pageNo, setPageNo] = useState(1);

  // Baixa os bytes (mesma origem, com sessão) e abre no pdf.js.
  useEffect(() => {
    let cancelled = false;
    let loading: PDFDocumentLoadingTask | undefined;
    void (async () => {
      try {
        const res = await fetch(pdf.url, { credentials: "include" });
        if (!res.ok) throw new Error("download");
        const data = new Uint8Array(await res.arrayBuffer());
        loading = getDocument({ data });
        const doc = await loading.promise;
        if (cancelled) return;
        const first = (await doc.getPage(1)).getViewport({ scale: 1 });
        if (!cancelled) setLoad({ state: "ready", doc, ratio: first.height / first.width });
      } catch (err) {
        if (cancelled) return;
        const protectedPdf = (err as { name?: string })?.name === "PasswordException";
        setLoad({
          state: "error",
          message: protectedPdf
            ? "Este PDF é protegido por senha. Baixe o arquivo para abri-lo."
            : "Não foi possível abrir este PDF. Você ainda pode baixá-lo.",
        });
      }
    })();
    return () => {
      cancelled = true;
      void loading?.destroy(); // libera o worker e a memória do documento
    };
  }, [pdf.url]);

  // Largura disponível: as páginas se ajustam à tela e o zoom multiplica essa largura.
  useEffect(() => {
    if (!box) return;
    const ro = new ResizeObserver(() => setBoxWidth(box.clientWidth));
    ro.observe(box); // dispara já na primeira observação
    return () => ro.disconnect();
  }, [box]);

  const zoomBy = useCallback((d: number) => setZoomIdx((i) => Math.min(ZOOMS.length - 1, Math.max(0, i + d))), []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return; // deixa Ctrl+C, Ctrl+F etc. para o navegador
      if (e.key === "Escape") onClose();
      else if (e.key === "+" || e.key === "=") zoomBy(1);
      else if (e.key === "-") zoomBy(-1);
      else return;
      e.preventDefault();
      e.stopPropagation();
    };
    window.addEventListener("keydown", onKey, true);
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey, true);
      document.body.style.overflow = overflow;
    };
  }, [onClose, zoomBy]);

  const onScroll = useCallback(() => {
    if (!box) return;
    const mid = box.getBoundingClientRect().top + box.clientHeight / 2;
    let current = 1;
    for (const el of box.querySelectorAll<HTMLElement>("[data-page]")) {
      if (el.getBoundingClientRect().top <= mid) current = Number(el.dataset.page);
      else break;
    }
    setPageNo(current);
  }, [box]);

  const fit = Math.max(200, Math.min(boxWidth - 32, 1000));
  const width = Math.round(fit * (ZOOMS[zoomIdx] ?? 1));
  const pages = load.state === "ready" ? load.doc.numPages : 0;
  const iconBtn =
    "inline-flex size-10 items-center justify-center rounded-full text-white/85 transition-colors hover:bg-white/10 hover:text-white disabled:pointer-events-none disabled:opacity-30";

  return createPortal(
    <motion.div
      role="dialog"
      aria-modal="true"
      aria-label={`PDF ${pdf.fileName}`}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.15 }}
      className="fixed inset-0 z-[100] flex flex-col bg-[#1a1d21]/[0.98] text-white"
    >
      <header className="flex shrink-0 items-center gap-3 border-b border-white/10 px-4 py-3">
        <span className="inline-flex h-9 w-8 shrink-0 items-center justify-center rounded-md bg-red-600 text-[10px] font-bold">PDF</span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{pdf.fileName}</p>
          <p className="text-xs text-white/60">
            {pages > 0 && `Página ${pageNo} de ${pages} • `}
            {formatBytes(pdf.sizeBytes)}
          </p>
        </div>
        <button type="button" className={iconBtn} onClick={() => zoomBy(-1)} disabled={zoomIdx <= 0} aria-label="Reduzir" title="Reduzir (−)">
          <ZoomOut className="size-5" />
        </button>
        <span className="w-10 text-center text-xs tabular-nums text-white/70">{Math.round((ZOOMS[zoomIdx] ?? 1) * 100)}%</span>
        <button
          type="button"
          className={iconBtn}
          onClick={() => zoomBy(1)}
          disabled={zoomIdx >= ZOOMS.length - 1}
          aria-label="Ampliar"
          title="Ampliar (+)"
        >
          <ZoomIn className="size-5" />
        </button>
        <a href={pdf.url} download={pdf.fileName} className={iconBtn} aria-label="Baixar PDF" title="Baixar">
          <Download className="size-5" />
        </a>
        <button type="button" className={iconBtn} onClick={onClose} aria-label="Fechar" title="Fechar (Esc)">
          <X className="size-5" />
        </button>
      </header>

      <div ref={setBox} onScroll={onScroll} className="min-h-0 flex-1 overflow-auto">
        {load.state === "loading" && (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-white/70">
            <LoaderCircle className="size-5 animate-spin" /> Abrindo PDF…
          </div>
        )}
        {load.state === "error" && (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center text-sm text-white/80">
            <p>{load.message}</p>
            <a
              href={pdf.url}
              download={pdf.fileName}
              className="inline-flex items-center gap-2 rounded-md bg-white/10 px-3 py-1.5 hover:bg-white/15"
            >
              <Download className="size-4" /> Baixar
            </a>
          </div>
        )}
        {load.state === "ready" && (
          <div className={cn("flex w-fit min-w-full flex-col items-center gap-3 p-4")}>
            {Array.from({ length: load.doc.numPages }, (_, i) => (
              <PdfPage key={i} doc={load.doc} num={i + 1} width={width} ratio={load.ratio} root={box} />
            ))}
          </div>
        )}
      </div>
    </motion.div>,
    document.body,
  );
}
