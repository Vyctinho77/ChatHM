import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from "react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Check, FileText, Image, LoaderCircle, MapPin, Mic, Paperclip, Pencil, Reply, SendHorizontal, Smile, Sparkles, Trash2, X } from "lucide-react";
import { Menu, MenuItem } from "@/components/Menu";
import { IconButton } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { useVoiceRecorder } from "./useVoiceRecorder";
import { recordingError } from "./voiceCapture";
import { getDraft, setDraft } from "./drafts";
import type { OutgoingDraft } from "./store";
import { AttachmentPreview, type AttachmentSelection } from "./AttachmentPreview";

const quickEmojis = ["😀", "😉", "🙏", "👍", "👏", "✅", "📄", "📎", "⏰", "💰", "🎉", "❤️"];

const MAX_FILE = 25 * 1024 * 1024; // padrão do servidor (MAX_ATTACHMENT_MB); a validação real é do backend

/** Forma de onda real (40 barras de 0 a 1) a partir do áudio gravado; aleatória se o navegador não decodificar. */
async function waveformOf(blob: Blob, bars = 40): Promise<number[]> {
  let ctx: AudioContext | undefined;
  try {
    ctx = new AudioContext();
    const buf = await ctx.decodeAudioData(await blob.arrayBuffer());
    const data = buf.getChannelData(0);
    const step = Math.max(1, Math.floor(data.length / bars));
    const out = Array.from({ length: bars }, (_, i) => {
      let peak = 0;
      for (let j = i * step; j < Math.min(data.length, (i + 1) * step); j++) peak = Math.max(peak, Math.abs(data[j]!));
      return peak;
    });
    const max = Math.max(...out, 0.01);
    return out.map((v) => 0.15 + (v / max) * 0.85);
  } catch {
    return Array.from({ length: bars }, () => 0.25 + Math.random() * 0.75);
  } finally {
    if (ctx && ctx.state !== "closed") void ctx.close().catch(() => undefined);
  }
}

export function Composer({
  conversationId,
  disabled,
  disabledReason,
  mediaDisabledReason,
  incoming,
  onSend,
  reply,
  onCancelReply,
  edit,
  onCancelEdit,
  onSaveEdit,
}: {
  /** Mensagem sendo respondida: aparece como citação acima do campo e vai junto no envio. */
  reply?: { author: string; text: string } | null;
  onCancelReply?: () => void;
  /** Edição de uma mensagem enviada: o campo começa com o texto atual; Enter salva, Esc cancela. */
  edit?: { id: string; text: string } | null;
  onCancelEdit?: () => void;
  onSaveEdit?: (text: string) => Promise<void>;
  conversationId: string;
  disabled?: boolean;
  disabledReason?: string;
  /** Quando definido, anexos e gravação de áudio ficam desativados com esta explicação. */
  mediaDisabledReason?: string;
  /** Arquivos arrastados do computador para o chat: abrem a prévia de anexos (id muda a cada soltura). */
  incoming?: { id: number; files: File[] } | null;
  onSend: (d: OutgoingDraft) => void;
}) {
  const [text, setText] = useState(() => (edit ? edit.text : getDraft(conversationId)));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [attachments, setAttachments] = useState<AttachmentSelection | null>(null);
  const reducedMotion = useReducedMotion();
  const ta = useRef<HTMLTextAreaElement>(null);
  const docInput = useRef<HTMLInputElement>(null);
  const imgInput = useRef<HTMLInputElement>(null);
  const hdInput = useRef<HTMLInputElement>(null);

  const recorder = useVoiceRecorder((blob, durationSec) => {
    void waveformOf(blob).then((waveform) =>
      onSend({ kind: "audio", url: URL.createObjectURL(blob), durationSec, waveform, file: blob }),
    );
  }, setError);

  // Auto-altura do campo.
  useEffect(() => {
    const el = ta.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [text]);

  function update(v: string) {
    setText(v);
    if (!edit) setDraft(conversationId, v); // editar não mexe no rascunho da conversa
  }

  // Ao responder ou editar, o cursor já fica no campo.
  const focusKey = edit?.id ?? (reply ? "reply" : "");
  useEffect(() => {
    if (focusKey) ta.current?.focus();
  }, [focusKey]);

  async function saveEdit() {
    const t = text.trim();
    if (!edit || !t || saving) return;
    if (t === edit.text.trim()) return onCancelEdit?.();
    setSaving(true);
    setError("");
    try {
      await onSaveEdit?.(t);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Não foi possível editar.");
    } finally {
      setSaving(false);
    }
  }

  function sendText() {
    if (edit) return void saveEdit();
    const t = text.trim();
    if (!t || disabled) return;
    onSend({ kind: "text", text: t });
    update("");
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      sendText();
    }
    if (e.key === "Escape") {
      if (edit) onCancelEdit?.();
      else if (reply) onCancelReply?.();
    }
  }

  function sendLocation() {
    if (!("geolocation" in navigator)) return setError("Localização indisponível neste navegador.");
    setError("");
    navigator.geolocation.getCurrentPosition(
      (pos) => onSend({ kind: "location", lat: pos.coords.latitude, lng: pos.coords.longitude }),
      () => setError("Não foi possível obter a localização (permissão negada?)."),
      { enableHighAccuracy: true, timeout: 10_000 },
    );
  }

  // Arquivos soltos no chat: mesma checagem e mesma prévia de "Anexar".
  const [seenIncoming, setSeenIncoming] = useState(0);
  if (incoming && incoming.id !== seenIncoming) {
    setSeenIncoming(incoming.id);
    if (incoming.files.length > 10) setError("Selecione até 10 arquivos por envio.");
    else if (incoming.files.some((file) => file.size > MAX_FILE)) setError("Cada arquivo pode ter até 25 MB.");
    else {
      setError("");
      setAttachments({ files: incoming.files, kind: "image", hd: false }); // jpeg/png/webp viram foto; o resto vai como documento
    }
  }

  function onFile(kind: "document" | "image", hd = false) {
    return (e: ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(e.target.files ?? []);
      e.target.value = "";
      if (!files.length) return;
      if (files.length > 10) return setError("Selecione até 10 arquivos por envio.");
      if (files.some(file => file.size > MAX_FILE)) return setError("Cada arquivo pode ter até 25 MB.");
      setError("");
      setAttachments({ files, kind, hd });
    };
  }

  if (disabled) {
    return (
      <div className="shrink-0 px-3 pb-3 pt-2 sm:px-4">
        <div className="rounded-2xl bg-panel px-4 py-3 text-center text-[12.5px] text-muted shadow-sm">{disabledReason}</div>
      </div>
    );
  }

  return (
    <div className="shrink-0 px-3 pb-3 pt-2 sm:px-4">
      {attachments && <AttachmentPreview selection={attachments} onClose={() => setAttachments(null)} onSend={onSend} />}
      {error && <p className="px-2 pb-1 text-xs text-danger">{error}</p>}
      <div className="relative">
      {(reply || edit) && !recorder.recording && (
        <div className="mb-1 flex items-start gap-2 rounded-2xl bg-panel px-3 py-2 shadow-sm">
          <span className="mt-0.5 text-brand-ink [&>svg]:size-4">{edit ? <Pencil /> : <Reply />}</span>
          <div className="min-w-0 flex-1 border-l-4 border-brand pl-2.5">
            <p className="truncate text-[12px] font-semibold text-brand-ink">{edit ? "Editando mensagem" : `Respondendo a ${reply?.author}`}</p>
            <p className="line-clamp-2 break-words text-[12.5px] text-muted">{edit ? edit.text : reply?.text}</p>
          </div>
          <IconButton label={edit ? "Cancelar edição" : "Cancelar resposta"} onClick={edit ? onCancelEdit : onCancelReply} className="size-7 shrink-0">
            <X className="size-4" />
          </IconButton>
        </div>
      )}
      {recorder.recording ? (
        <div className="flex min-h-14 items-center gap-3 rounded-2xl bg-panel px-3 py-2 shadow-lg shadow-black/5">
          <IconButton label="Descartar gravação" onClick={() => recorder.stop(true)}>
            <Trash2 className="size-5 text-danger" />
          </IconButton>
          <span className="flex items-center gap-2 text-sm">
            <span className="size-2.5 animate-pulse rounded-full bg-danger" />
            Gravando {formatDuration(recorder.elapsed)}
          </span>
          <span aria-hidden="true" className="ml-auto size-8 shrink-0" />
        </div>
      ) : (
        <div className="flex items-end gap-1 rounded-2xl bg-panel px-2.5 py-2 shadow-lg shadow-black/5">
          <Menu
            align="left"
            placement="top"
            className="grid w-64 grid-cols-6 gap-1 p-2"
            trigger={({ toggle }) => (
              <IconButton label="Emojis" onClick={toggle}>
                <Smile className="size-5" />
              </IconButton>
            )}
          >
            {(close) =>
              quickEmojis.map((e) => (
                <button
                  key={e}
                  type="button"
                  className="rounded-md p-1 text-xl hover:bg-hover"
                  onClick={() => {
                    update(text + e);
                    close();
                    ta.current?.focus();
                  }}
                >
                  {e}
                </button>
              ))
            }
          </Menu>
          <Menu
            align="left"
            placement="top"
            trigger={({ toggle }) => (
              <IconButton
                label={mediaDisabledReason ?? "Anexar"}
                disabled={!!mediaDisabledReason || !!edit}
                onClick={toggle}
              >
                <Paperclip className="size-5" />
              </IconButton>
            )}
          >
            {(close) => (
              <>
                <MenuItem icon={<FileText />} onClick={() => (docInput.current?.click(), close())}>
                  Documento
                </MenuItem>
                <MenuItem icon={<Image />} onClick={() => (imgInput.current?.click(), close())}>
                  Fotos
                </MenuItem>
                <MenuItem icon={<Sparkles />} onClick={() => (hdInput.current?.click(), close())}>
                  Foto em HD (original)
                </MenuItem>
                <MenuItem icon={<MapPin />} onClick={() => (sendLocation(), close())}>
                  Localização atual
                </MenuItem>
              </>
            )}
          </Menu>
          <input ref={docInput} type="file" multiple hidden onChange={onFile("document")} />
          <input ref={imgInput} type="file" multiple accept="image/jpeg,image/png,image/webp" hidden onChange={onFile("image")} />
          <input ref={hdInput} type="file" multiple accept="image/jpeg,image/png,image/webp" hidden onChange={onFile("image", true)} />

          <textarea
            ref={ta}
            rows={1}
            value={text}
            onChange={(e) => update(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Digite uma mensagem"
            aria-label="Mensagem"
            className="mx-1 max-h-40 min-h-9 min-w-0 flex-1 resize-none bg-transparent px-2 py-2 text-[13.5px] leading-5 outline-none placeholder:text-muted focus-visible:outline-none"
          />

          <AnimatePresence mode="popLayout" initial={false}>
            <motion.span
              key={text.trim() || edit ? "send" : "mic"}
              initial={{ scale: 0.4, opacity: 0, rotate: -30 }}
              animate={{ scale: 1, opacity: 1, rotate: 0 }}
              exit={{ scale: 0.4, opacity: 0, rotate: 30 }}
              transition={{ type: "spring", stiffness: 600, damping: 30 }}
              className="inline-flex"
            >
              {text.trim() || edit ? (
                <IconButton label={edit ? "Salvar edição" : "Enviar"} onClick={sendText} disabled={saving || (!!edit && !text.trim())} className="text-brand-ink hover:text-brand-ink">
                  {edit ? <Check className="size-5" /> : <SendHorizontal className="size-5" />}
                </IconButton>
              ) : (
                <span aria-hidden="true" className="size-8" />
              )}
            </motion.span>
          </AnimatePresence>
        </div>
      )}
      {!attachments && ((!text.trim() && !edit) || recorder.recording) && (
        <button
          type="button"
          aria-label={recorder.recording ? "Enviar áudio" : mediaDisabledReason ?? (recorder.starting ? "Preparando microfone" : recorder.supported ? "Gravar áudio" : "Gravação de áudio indisponível neste navegador")}
          disabled={!recorder.supported || recorder.starting || !!mediaDisabledReason}
          onClick={() => {
            if (recorder.recording) return recorder.stop();
            setError("");
            void recorder.start().catch((error: unknown) => setError(recordingError(error)));
          }}
          className={cn("group/voice absolute bottom-0 right-0.5 isolate flex size-12 items-center justify-center rounded-full outline-none focus-visible:ring-2 focus-visible:ring-brand", !recorder.recording && "text-muted hover:text-fg disabled:opacity-50")}
        >
          <AnimatePresence>
            {recorder.recording && <motion.span
              key="recording-circle"
              aria-hidden="true"
              initial={{ scale: reducedMotion ? 1 : 0, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: reducedMotion ? 1 : 0, opacity: 0 }}
              transition={{ duration: reducedMotion ? 0 : 0.2, ease: "easeOut" }}
              className="pointer-events-none absolute inset-0 -z-10 rounded-full bg-brand shadow-md"
            />}
          </AnimatePresence>
          <motion.span initial={false} animate={{ scale: recorder.recording ? 1.15 : 1 }} transition={{ duration: reducedMotion ? 0 : 0.18 }} className={cn("pointer-events-none inline-flex", recorder.recording && "text-accent-fg transition-opacity duration-150 group-hover/voice:opacity-0 group-focus-visible/voice:opacity-0 motion-reduce:transition-none")}>
            {recorder.starting ? <LoaderCircle className="size-5 animate-spin" /> : <Mic className="size-5" />}
          </motion.span>
          {recorder.recording && <span aria-hidden="true" className="pointer-events-none absolute inset-0 flex scale-90 items-center justify-center text-accent-fg opacity-0 transition-[opacity,scale] duration-150 group-hover/voice:scale-100 group-hover/voice:opacity-100 group-focus-visible/voice:scale-100 group-focus-visible/voice:opacity-100 motion-reduce:transition-none"><SendHorizontal className="size-5" /></span>}
        </button>
      )}
      </div>
    </div>
  );
}
