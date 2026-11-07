import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, FileText, LoaderCircle, Plus, SendHorizontal, Trash2, X } from "lucide-react";
import { Button, IconButton, Modal } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatBytes } from "@/lib/format";
import type { OutgoingDraft } from "./store.types";

type Attachment = { id: string; file: File; kind: "image" | "document"; hd: boolean; caption: string };
export type AttachmentSelection = { files: File[]; kind: "image" | "document"; hd: boolean };
const MAX_FILE = 25 * 1024 * 1024;
const MAX_FILES = 10;

function LocalImage({ file, alt, className }: { file: File; alt: string; className: string }) {
  const img = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const value = URL.createObjectURL(file);
    if (img.current) img.current.src = value;
    return () => URL.revokeObjectURL(value);
  }, [file]);
  return <img ref={img} alt={alt} className={className} />;
}

function Thumbnail({ item }: { item: Attachment }) {
  return item.kind === "image" ? <LocalImage file={item.file} alt="" className="size-full object-cover" /> : <FileText className="size-6 text-brand-ink" />;
}

/** PDF local renderizado em canvas; não incorpora scripts ou links do documento. */
function PdfPreview({ file }: { file: File }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [page, setPage] = useState(1);
  const [pages, setPages] = useState(0);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let disposed = false;
    let destroy: (() => void) | undefined;
    void (async () => {
      try {
        const [pdf, worker] = await Promise.all([import("pdfjs-dist"), import("pdfjs-dist/build/pdf.worker.min.mjs?url")]);
        if (disposed) return;
        pdf.GlobalWorkerOptions.workerSrc = worker.default;
        const data = new Uint8Array(await file.arrayBuffer());
        if (disposed) return;
        const task = pdf.getDocument({ data });
        destroy = () => { void task.destroy(); };
        const doc = await task.promise;
        if (disposed) return;
        setPages(doc.numPages);
        const p = await doc.getPage(page);
        if (disposed || !canvas.current) return;
        const original = p.getViewport({ scale: 1 });
        const viewport = p.getViewport({ scale: Math.min(2, 1200 / original.width) });
        const c = canvas.current;
        c.width = viewport.width;
        c.height = viewport.height;
        await p.render({ canvas: c, viewport }).promise;
        if (!disposed) setLoading(false);
      } catch {
        if (!disposed) { setError(true); setLoading(false); }
      }
    })();
    return () => { disposed = true; destroy?.(); };
  }, [file, page]);
  return (
    <div className="flex size-full min-h-0 flex-col items-center gap-3">
      <div className="relative flex min-h-0 w-full flex-1 items-center justify-center overflow-hidden">
        {loading && <LoaderCircle aria-label="Carregando PDF" className="absolute size-7 animate-spin text-brand-ink" />}
        {error ? <p className="max-w-sm text-center text-muted">Não foi possível exibir este PDF. Você ainda pode enviar o arquivo original.</p> : <canvas ref={canvas} aria-label={`Prévia do PDF, página ${page}`} className={cn("max-h-full max-w-full object-contain shadow-md", loading && "invisible")} />}
      </div>
      {pages > 0 && !error && <div className="flex shrink-0 items-center gap-3 rounded-full bg-panel px-2 py-1 text-xs text-muted">
        <IconButton label="Página anterior" disabled={page <= 1 || loading} onClick={() => { setLoading(true); setPage(p => p - 1); }}><ChevronLeft className="size-4" /></IconButton>
        <span>Página {page} de {pages}</span>
        <IconButton label="Próxima página" disabled={page >= pages || loading} onClick={() => { setLoading(true); setPage(p => p + 1); }}><ChevronRight className="size-4" /></IconButton>
      </div>}
    </div>
  );
}

function FilePreview({ item }: { item: Attachment }) {
  if (item.kind === "image") return <LocalImage file={item.file} alt={`Prévia de ${item.file.name}`} className="max-h-full max-w-full object-contain shadow-md" />;
  if (item.file.type === "application/pdf" || /\.pdf$/i.test(item.file.name)) return <PdfPreview key={item.id} file={item.file} />;
  return <div className="flex max-w-sm flex-col items-center gap-4 rounded-2xl bg-panel p-8 text-center"><FileText className="size-16 text-brand-ink" /><p className="break-all font-medium">{item.file.name}</p><span className="text-sm text-muted">{formatBytes(item.file.size)}</span></div>;
}

function attachment(file: File, kind: Attachment["kind"], hd: boolean): Attachment {
  return { id: crypto.randomUUID(), file, kind: kind === "image" && file.type.startsWith("image/") ? "image" : "document", hd, caption: "" };
}

async function outgoing(item: Attachment): Promise<OutgoingDraft> {
  const { file, caption } = item;
  if (item.kind === "document") return { kind: "document", file, url: URL.createObjectURL(file), fileName: file.name, mimeType: file.type || "application/octet-stream", sizeBytes: file.size, caption: caption.trim() };
  const url = URL.createObjectURL(file);
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    return { kind: "image", file, url, width: img.naturalWidth, height: img.naturalHeight, caption: caption.trim(), hd: item.hd };
  } catch {
    URL.revokeObjectURL(url);
    throw new Error(`Não foi possível abrir ${file.name}. Escolha outra imagem.`);
  }
}

export function AttachmentPreview({ selection, onClose, onSend }: { selection: AttachmentSelection; onClose: () => void; onSend: (draft: OutgoingDraft) => void }) {
  const [items, setItems] = useState(() => selection.files.map(f => attachment(f, selection.kind, selection.hd)));
  const [active, setActive] = useState(0);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const add = useRef<HTMLInputElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const item = items[active];
  const busyRef = useRef(false);
  useEffect(() => { panel.current?.focus(); }, []);

  function patch(change: Partial<Attachment>) { setItems(old => old.map((it, i) => i === active ? { ...it, ...change } : it)); }
  function addFiles(files: File[]) {
    if (files.some(f => f.size > MAX_FILE)) return setError("Cada arquivo pode ter até 25 MB.");
    if (items.length + files.length > MAX_FILES) return setError("Selecione até 10 arquivos por envio.");
    setError("");
    setItems(old => [...old, ...files.map(f => attachment(f, f.type.startsWith("image/") ? "image" : "document", false))]);
    if (files.length) setActive(items.length);
  }
  async function send() {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true); setError("");
    const drafts: OutgoingDraft[] = [];
    try {
      for (const it of items) drafts.push(await outgoing(it));
      drafts.forEach(onSend);
      onClose();
    } catch (e) {
      drafts.forEach(d => { if ("url" in d) URL.revokeObjectURL(d.url); });
      setError(e instanceof Error ? e.message : "Não foi possível preparar os anexos.");
    } finally { busyRef.current = false; setBusy(false); }
  }

  if (!item) return null;

  return <div ref={panel} role="region" aria-label="Preparar anexos" tabIndex={-1}
    onKeyDown={e => { if (e.key === "Escape" && !busy) { e.stopPropagation(); setConfirm(true); } }}
    onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); if (!busy) addFiles(Array.from(e.dataTransfer.files)); }}
    className="absolute inset-x-0 bottom-0 top-14 z-30 flex min-h-0 flex-col bg-app outline-none">
    <div className="flex h-14 shrink-0 items-center gap-3 border-b border-line px-3 sm:px-5">
      <IconButton label="Fechar prévia" disabled={busy} onClick={() => setConfirm(true)}><X className="size-5" /></IconButton>
      <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{item.file.name}</p><p className="text-xs text-muted">{formatBytes(item.file.size)} · {active + 1} de {items.length}</p></div>
      {item.kind === "image" && <button type="button" aria-pressed={item.hd} disabled={busy} onClick={() => patch({ hd: !item.hd })} className={cn("rounded-lg px-3 py-1.5 text-xs font-semibold transition-colors", item.hd ? "bg-brand/15 text-brand-ink" : "bg-elevated text-muted hover:bg-hover")}>HD{item.hd ? " ativado" : ""}</button>}
      <IconButton label="Remover arquivo" disabled={busy} onClick={() => { if (items.length === 1) return setConfirm(true); setItems(old => old.filter((_, i) => i !== active)); setActive(Math.max(0, active - 1)); }}><Trash2 className="size-4" /></IconButton>
    </div>
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-hidden p-4 sm:p-7"><FilePreview key={item.id} item={item} /></div>
    <div className="mx-auto w-full max-w-3xl shrink-0 px-4 pb-4">
      {error && <p role="alert" className="mb-2 text-sm text-danger">{error}</p>}
      <textarea aria-label="Legenda do anexo" placeholder="Adicione uma legenda…" value={item.caption} disabled={busy} maxLength={1024} rows={2} onChange={e => patch({ caption: e.target.value })} className="max-h-28 min-h-12 w-full resize-none rounded-2xl bg-panel px-4 py-3 text-sm outline-none placeholder:text-muted focus-visible:ring-2 focus-visible:ring-brand" />
    </div>
    <div className="flex shrink-0 items-center gap-3 border-t border-line px-4 py-3 sm:px-6">
      <div className="flex min-w-0 flex-1 items-center justify-center gap-2 overflow-x-auto p-1">
        {items.map((it, i) => <button type="button" key={it.id} disabled={busy} aria-label={`Selecionar ${it.file.name}`} aria-pressed={i === active} onClick={() => setActive(i)} className={cn("flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border-2 bg-panel", i === active ? "border-brand ring-2 ring-brand/20" : "border-line hover:border-brand/50")}><Thumbnail item={it} /></button>)}
        <IconButton label="Adicionar arquivos" disabled={busy || items.length >= MAX_FILES} onClick={() => add.current?.click()} className="size-14 shrink-0 rounded-lg border border-line"><Plus className="size-5" /></IconButton>
      </div>
      <input ref={add} type="file" multiple hidden onChange={e => { addFiles(Array.from(e.target.files ?? [])); e.target.value = ""; }} />
      <button type="button" disabled={busy} aria-label={`Enviar ${items.length} ${items.length === 1 ? "arquivo" : "arquivos"}`} onClick={() => void send()} className="flex size-14 shrink-0 items-center justify-center rounded-full bg-brand text-accent-fg shadow-sm transition-colors hover:bg-brand/85 disabled:opacity-50">{busy ? <LoaderCircle className="size-6 animate-spin" /> : <SendHorizontal className="size-6" />}</button>
    </div>
    <Modal open={confirm} onClose={() => setConfirm(false)} title="Descartar seleção?">
      <p className="mb-6 text-sm text-muted">Os arquivos e as legendas desta seleção serão descartados.</p>
      <div className="flex justify-end gap-2"><Button variant="secondary" onClick={() => setConfirm(false)}>Continuar editando</Button><Button variant="danger" onClick={onClose}>Descartar</Button></div>
    </Modal>
  </div>;
}
