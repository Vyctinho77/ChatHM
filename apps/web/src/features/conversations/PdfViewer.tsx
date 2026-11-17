import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { motion } from "motion/react";
import { getDocument, GlobalWorkerOptions, TextLayer, type PDFDocumentLoadingTask, type PDFDocumentProxy, type RenderTask } from "pdfjs-dist";
import workerSrc from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { Check, ChevronDown, ChevronUp, Download, KeyRound, LoaderCircle, Maximize2, MoveHorizontal, Search, X, ZoomIn, ZoomOut } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatBytes } from "@/lib/format";
import type { Message } from "./types";
import { applyReadingOrder } from "./pdfReadingOrder";
import { findMatches, type Match } from "./pdfSearch";
import { findAccessKeys, formatAccessKey } from "./pdfAccessKey";
import "./pdf-text-layer.css";

GlobalWorkerOptions.workerSrc = workerSrc;

export type ViewablePdf = Extract<Message, { kind: "document" }> & { url: string };

const ZOOMS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3];
const MIN_ZOOM = 0.25;
const MAX_ZOOM = ZOOMS[ZOOMS.length - 1]!;

type Load = { state: "loading" } | { state: "error"; message: string } | { state: "ready"; doc: PDFDocumentProxy; ratio: number };

/** Destaques da busca (CSS Custom Highlight API). Sem suporte no navegador, a busca só leva até a página. */
type HighlightApi = { highlights: Map<string, unknown> };
const highlightApi = (): HighlightApi | null => {
  const css = (globalThis as { CSS?: Partial<HighlightApi> }).CSS;
  const H = (globalThis as { Highlight?: new (...r: Range[]) => unknown }).Highlight;
  return css?.highlights && H ? (css as HighlightApi) : null;
};

/** Onde cada página registra seus destaques; o visualizador junta tudo e pinta. */
type Registry = {
  set: (page: number, all: Range[], current: Range[]) => void;
  clear: (page: number) => void;
};

/** Uma página: só é desenhada quando chega perto da área visível. */
function PdfPage({
  doc,
  num,
  width,
  ratio,
  root,
  matches,
  current,
  registry,
}: {
  doc: PDFDocumentProxy;
  num: number;
  width: number;
  ratio: number; // altura/largura estimada (da 1ª página) até a página real carregar
  root: HTMLElement | null;
  matches: Match[];
  current: number | null; // ocorrência atual desta página (a "laranja"), se houver
  registry: Registry;
}) {
  const wrap = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const text = useRef<HTMLDivElement>(null);
  const divs = useRef<HTMLElement[]>([]);
  const [near, setNear] = useState(false);
  const [own, setOwn] = useState<number | null>(null);
  const [tick, setTick] = useState(0); // sobe a cada vez que a camada de texto é refeita

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
        if (cancelled) return;
        divs.current = layer.textDivs; // na ordem do PDF (a mesma da busca), antes de reordenar o DOM
        // A seleção segue a ordem do DOM: reordena os textos como a pessoa lê (linhas, esquerda→direita) em vez da ordem de desenho do PDF.
        applyReadingOrder(t, layer.textDivs);
        setTick((n) => n + 1);
      } catch {
        /* renderização cancelada (zoom/fechar) ou página ilegível: fica em branco */
      }
    })();
    return () => {
      cancelled = true;
      divs.current = [];
      registry.clear(num);
      task?.cancel();
      layer?.cancel();
    };
  }, [near, width, doc, num, registry]);

  // Destaques da busca: transforma as ocorrências em trechos de texto da camada (só com a página já desenhada).
  useEffect(() => {
    if (!tick || !matches.length) return;
    const range = (m: Match) =>
      m.pieces.flatMap((p) => {
        const node = divs.current[p.item]?.firstChild;
        if (!node || node.nodeType !== Node.TEXT_NODE) return [];
        const r = new Range();
        r.setStart(node, Math.min(p.start, node.textContent?.length ?? 0));
        r.setEnd(node, Math.min(p.end, node.textContent?.length ?? 0));
        return [r];
      });
    registry.set(num, matches.flatMap(range), current !== null && matches[current] ? range(matches[current]) : []);
    return () => registry.clear(num);
  }, [tick, matches, current, num, registry]);

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

/** Copia texto para a área de transferência (com plano B para navegadores sem a API). */
async function copyText(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    try {
      const ta = document.createElement("textarea");
      ta.value = value;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand("copy");
      ta.remove();
      return ok;
    } catch {
      return false;
    }
  }
}

/**
 * Visualizador de PDF em tela cheia, dentro do app: páginas rolando, zoom e texto
 * selecionável/copiável (camada de texto do pdf.js). Só o núcleo do pdf.js é usado: scripts do PDF não rodam.
 * Tem busca no documento inteiro (Ctrl+F), ajuste à largura/página, ir para a página N, teclado e cópia da chave de acesso de nota fiscal.
 */
export function PdfViewer({ pdf, onClose }: { pdf: ViewablePdf; onClose: () => void }) {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  const [zoom, setZoom] = useState(1);
  const [box, setBox] = useState<HTMLDivElement | null>(null);
  const [boxWidth, setBoxWidth] = useState(0);
  const [pageNo, setPageNo] = useState(1);
  const [pageDraft, setPageDraft] = useState<string | null>(null);

  // Busca
  const [texts, setTexts] = useState<string[][] | null>(null); // itens de texto de cada página (ordem do PDF)
  const [indexed, setIndexed] = useState(0);
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [cur, setCur] = useState(0);
  const searchInput = useRef<HTMLInputElement>(null);
  const searchOpenRef = useRef(false);
  const pageNoRef = useRef(1);
  const pending = useRef<number | null>(null); // página cuja ocorrência atual deve ser trazida à vista quando desenhada

  // Chave de acesso de nota fiscal
  const [copied, setCopied] = useState(false);

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

  // Lê o texto de todas as páginas em segundo plano (para a busca e para achar a chave de acesso).
  useEffect(() => {
    if (load.state !== "ready") return;
    let cancelled = false;
    const doc = load.doc;
    const all: string[][] = [];
    void (async () => {
      try {
        for (let n = 1; n <= doc.numPages; n++) {
          const page = await doc.getPage(n);
          const content = await page.getTextContent();
          if (cancelled) return;
          all.push(content.items.flatMap((it) => ("str" in it ? [it.str] : [])));
          setIndexed(n);
        }
        if (!cancelled) setTexts(all);
      } catch {
        /* sem texto (PDF escaneado ou ilegível): a busca fica sem resultados */
        if (!cancelled) setTexts(all);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [load]);

  // Largura disponível: as páginas se ajustam à tela e o zoom multiplica essa largura.
  useEffect(() => {
    if (!box) return;
    const ro = new ResizeObserver(() => setBoxWidth(box.clientWidth));
    ro.observe(box); // dispara já na primeira observação
    return () => ro.disconnect();
  }, [box]);

  const zoomBy = useCallback((d: number) => {
    setZoom((z) => {
      if (d > 0) return ZOOMS.find((v) => v > z + 0.001) ?? MAX_ZOOM;
      return [...ZOOMS].reverse().find((v) => v < z - 0.001) ?? ZOOMS[0]!;
    });
  }, []);

  const fitWidth = useCallback(() => setZoom(1), []);
  const fitPage = useCallback(() => {
    if (!box || load.state !== "ready") return;
    const base = Math.max(200, Math.min(box.clientWidth - 32, 1000));
    const z = (box.clientHeight - 32) / (base * load.ratio);
    setZoom(Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.round(z * 100) / 100)));
  }, [box, load]);

  const goToPage = useCallback(
    (n: number) => {
      if (!box || load.state !== "ready") return;
      const target = Math.min(load.doc.numPages, Math.max(1, Math.round(n)));
      const el = box.querySelector<HTMLElement>(`[data-page="${target}"]`);
      if (!el) return;
      box.scrollTo({ top: box.scrollTop + el.getBoundingClientRect().top - box.getBoundingClientRect().top - 16 });
      setPageNo(target);
    },
    [box, load],
  );

  // Busca: ocorrências por página e a lista corrida (para "3 de 12" e para ir de uma à outra).
  const matches = useMemo<Match[][] | null>(() => (texts && query.trim() ? texts.map((items) => findMatches(items, query)) : null), [texts, query]);
  const flat = useMemo(() => (matches ? matches.flatMap((list, p) => list.map((_, i) => ({ page: p + 1, idx: i }))) : []), [matches]);
  const flatRef = useRef(flat);

  // Espelha o estado em refs (lidos por ouvintes de teclado e por efeitos) sem gravar durante a renderização.
  const boxRef = useRef<HTMLDivElement | null>(null);
  useLayoutEffect(() => {
    searchOpenRef.current = searchOpen;
    pageNoRef.current = pageNo;
    flatRef.current = flat;
    boxRef.current = box;
  });

  const go = useCallback(
    (i: number) => {
      const list = flatRef.current;
      if (!list.length) return;
      const n = ((i % list.length) + list.length) % list.length;
      setCur(n);
      pending.current = list[n]!.page;
      goToPage(list[n]!.page);
    },
    [goToPage],
  );

  // Ao mudar o texto procurado, vai para a primeira ocorrência a partir da página atual.
  useEffect(() => {
    const list = flatRef.current;
    if (!searchOpenRef.current || !list.length) {
      setCur(0);
      return;
    }
    const first = list.findIndex((f) => f.page >= pageNoRef.current);
    go(first < 0 ? 0 : first);
  }, [matches, go]);

  // Registro dos destaques: cada página informa seus trechos e aqui se pinta tudo de uma vez.
  const painted = useRef(new Map<number, { all: Range[]; current: Range[] }>());
  const registry = useMemo<Registry>(() => {
    const paint = () => {
      const api = highlightApi();
      if (!api) return;
      const H = (globalThis as unknown as { Highlight: new (...r: Range[]) => unknown }).Highlight;
      const all: Range[] = [];
      const current: Range[] = [];
      for (const v of painted.current.values()) {
        all.push(...v.all);
        current.push(...v.current);
      }
      if (all.length) api.highlights.set("pdf-find", new H(...all));
      else api.highlights.delete("pdf-find");
      if (current.length) api.highlights.set("pdf-find-current", new H(...current));
      else api.highlights.delete("pdf-find-current");
    };
    return {
      set(page, all, current) {
        painted.current.set(page, { all, current });
        paint();
        // A ocorrência atual só é trazida à vista quando a página dela está pronta.
        if (current.length && pending.current === page && boxRef.current) {
          pending.current = null;
          const b = boxRef.current;
          const r = current[0]!.getBoundingClientRect();
          const bb = b.getBoundingClientRect();
          const dy = r.top - bb.top - bb.height / 2;
          const dx = r.left - bb.left - bb.width / 2;
          b.scrollBy({ top: dy, left: Math.abs(dx) > bb.width / 3 ? dx : 0 });
        }
      },
      clear(page) {
        if (painted.current.delete(page)) paint();
      },
    };
  }, []);
  useEffect(
    () => () => {
      painted.current.clear();
      const api = highlightApi();
      api?.highlights.delete("pdf-find");
      api?.highlights.delete("pdf-find-current");
    },
    [],
  );

  const openSearch = useCallback(() => {
    setSearchOpen(true);
    requestAnimationFrame(() => {
      searchInput.current?.focus();
      searchInput.current?.select();
    });
  }, []);
  const closeSearch = useCallback(() => {
    setSearchOpen(false);
    setQuery("");
    pending.current = null;
  }, []);

  // Teclado: Ctrl+F abre a busca; Esc fecha a busca (e depois o PDF); +/- zoom; Page/Home/End/setas rolam.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target;
      const typing = t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement;
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "f") {
        openSearch();
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey) return; // deixa Ctrl+C etc. para o navegador
      if (e.key === "Escape") {
        if (searchOpenRef.current) closeSearch();
        else onClose();
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      if (typing) return;
      const b = boxRef.current;
      const page = b ? b.clientHeight * 0.9 : 0;
      if (e.key === "+" || e.key === "=") zoomBy(1);
      else if (e.key === "-") zoomBy(-1);
      else if (e.key === "PageDown" || (e.key === " " && !e.shiftKey)) b?.scrollBy({ top: page });
      else if (e.key === "PageUp" || (e.key === " " && e.shiftKey)) b?.scrollBy({ top: -page });
      else if (e.key === "ArrowDown") b?.scrollBy({ top: 60 });
      else if (e.key === "ArrowUp") b?.scrollBy({ top: -60 });
      else if (e.key === "ArrowRight") b?.scrollBy({ left: 60 });
      else if (e.key === "ArrowLeft") b?.scrollBy({ left: -60 });
      else if (e.key === "Home") b?.scrollTo({ top: 0 });
      else if (e.key === "End") b?.scrollTo({ top: b.scrollHeight });
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
  }, [onClose, zoomBy, openSearch, closeSearch]);

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

  // Chave de acesso (NF-e/CT-e…): procurada no texto de todas as páginas.
  const keys = useMemo(() => (texts ? findAccessKeys(texts.map((items) => items.join(" ")).join("\n")) : []), [texts]);
  const copyKey = useCallback(async () => {
    const k = keys[0];
    if (!k) return;
    if (await copyText(k)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    }
  }, [keys]);

  const fit = Math.max(200, Math.min(boxWidth - 32, 1000));
  const width = Math.round(fit * zoom);
  const pages = load.state === "ready" ? load.doc.numPages : 0;
  const iconBtn =
    "inline-flex size-10 items-center justify-center rounded-full text-white/85 transition-colors hover:bg-white/10 hover:text-white disabled:pointer-events-none disabled:opacity-30";
  const indexing = load.state === "ready" && !texts;
  const current = flat[cur];

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
      <header className="flex shrink-0 items-center gap-1.5 border-b border-white/10 px-4 py-3 sm:gap-2">
        <span className="inline-flex h-9 w-8 shrink-0 items-center justify-center rounded-md bg-red-600 text-[10px] font-bold">PDF</span>
        <div className="mr-1 min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{pdf.fileName || "Documento"}</p>
          <p className="flex items-center gap-1 text-xs text-white/60">
            {pages > 0 && (
              <>
                Página
                <input
                  value={pageDraft ?? String(pageNo)}
                  inputMode="numeric"
                  aria-label="Ir para a página"
                  onFocus={(e) => {
                    setPageDraft(String(pageNo));
                    e.currentTarget.select();
                  }}
                  onChange={(e) => setPageDraft(e.target.value.replace(/\D/g, "").slice(0, 4))}
                  onBlur={() => setPageDraft(null)}
                  onKeyDown={(e) => {
                    if (e.key !== "Enter") return;
                    const n = Number(pageDraft);
                    if (n) goToPage(n);
                    e.currentTarget.blur();
                  }}
                  className="w-10 rounded bg-white/10 px-1 py-0.5 text-center text-white outline-none focus:ring-1 focus:ring-white/50"
                />
                de {pages}
              </>
            )}
            {pdf.sizeBytes ? <span>{pages > 0 ? "• " : ""}{formatBytes(pdf.sizeBytes)}</span> : null}
          </p>
        </div>
        {keys.length > 0 && (
          <button
            type="button"
            className={cn(iconBtn, "w-auto gap-1.5 px-3 text-xs")}
            onClick={() => void copyKey()}
            aria-label="Copiar chave de acesso da nota"
            title={`Copiar chave de acesso${keys.length > 1 ? ` (a 1ª de ${keys.length})` : ""}: ${formatAccessKey(keys[0]!)}`}
          >
            {copied ? <Check className="size-4 text-emerald-400" /> : <KeyRound className="size-4" />}
            <span className="hidden md:inline">{copied ? "Chave copiada" : "Copiar chave"}</span>
          </button>
        )}
        <button type="button" className={iconBtn} onClick={openSearch} aria-label="Buscar no PDF" title="Buscar (Ctrl+F)">
          <Search className="size-5" />
        </button>
        <button type="button" className={cn(iconBtn, "hidden sm:inline-flex")} onClick={fitWidth} aria-label="Ajustar à largura" title="Ajustar à largura">
          <MoveHorizontal className="size-5" />
        </button>
        <button type="button" className={cn(iconBtn, "hidden sm:inline-flex")} onClick={fitPage} aria-label="Ajustar à página" title="Ajustar à página inteira">
          <Maximize2 className="size-5" />
        </button>
        <button type="button" className={iconBtn} onClick={() => zoomBy(-1)} disabled={zoom <= MIN_ZOOM + 0.001} aria-label="Reduzir" title="Reduzir (−)">
          <ZoomOut className="size-5" />
        </button>
        <span className="w-10 text-center text-xs tabular-nums text-white/70">{Math.round(zoom * 100)}%</span>
        <button type="button" className={iconBtn} onClick={() => zoomBy(1)} disabled={zoom >= MAX_ZOOM - 0.001} aria-label="Ampliar" title="Ampliar (+)">
          <ZoomIn className="size-5" />
        </button>
        <a href={pdf.url} download={pdf.fileName} className={iconBtn} aria-label="Baixar PDF" title="Baixar">
          <Download className="size-5" />
        </a>
        <button type="button" className={iconBtn} onClick={onClose} aria-label="Fechar" title="Fechar (Esc)">
          <X className="size-5" />
        </button>
      </header>

      {searchOpen && (
        <div className="flex shrink-0 items-center gap-2 border-b border-white/10 bg-white/[0.04] px-4 py-2">
          <Search className="size-4 shrink-0 text-white/60" aria-hidden />
          <input
            ref={searchInput}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                go(cur + (e.shiftKey ? -1 : 1));
              }
            }}
            placeholder="Buscar no PDF"
            aria-label="Buscar no PDF"
            className="min-w-0 flex-1 bg-transparent text-sm text-white outline-none placeholder:text-white/40"
          />
          <span className="shrink-0 text-xs tabular-nums text-white/60" role="status" aria-live="polite">
            {indexing
              ? `Lendo ${indexed}/${pages}…`
              : !query.trim()
                ? ""
                : flat.length
                  ? `${cur + 1} de ${flat.length}${current ? ` • pág. ${current.page}` : ""}`
                  : "Nada encontrado"}
          </span>
          <button type="button" className={cn(iconBtn, "size-8")} onClick={() => go(cur - 1)} disabled={!flat.length} aria-label="Ocorrência anterior" title="Anterior (Shift+Enter)">
            <ChevronUp className="size-4" />
          </button>
          <button type="button" className={cn(iconBtn, "size-8")} onClick={() => go(cur + 1)} disabled={!flat.length} aria-label="Próxima ocorrência" title="Próxima (Enter)">
            <ChevronDown className="size-4" />
          </button>
          <button type="button" className={cn(iconBtn, "size-8")} onClick={closeSearch} aria-label="Fechar busca" title="Fechar busca (Esc)">
            <X className="size-4" />
          </button>
        </div>
      )}

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
              <PdfPage
                key={i}
                doc={load.doc}
                num={i + 1}
                width={width}
                ratio={load.ratio}
                root={box}
                matches={matches?.[i] ?? NO_MATCHES}
                current={current && current.page === i + 1 ? current.idx : null}
                registry={registry}
              />
            ))}
          </div>
        )}
      </div>
    </motion.div>,
    document.body,
  );
}

const NO_MATCHES: Match[] = [];
