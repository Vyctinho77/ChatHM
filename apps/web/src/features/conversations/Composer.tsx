import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { FileText, Image, Mic, Paperclip, SendHorizontal, Smile, Square, Trash2 } from "lucide-react";
import { Menu, MenuItem } from "@/components/Menu";
import { IconButton } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { getDraft, setDraft } from "./drafts";
import type { OutgoingDraft } from "./store";

const quickEmojis = ["😀", "😉", "🙏", "👍", "👏", "✅", "📄", "📎", "⏰", "💰", "🎉", "❤️"];

const MAX_FILE = 100 * 1024 * 1024; // limite provisório; validação real fica no backend

function useRecorder(onDone: (blob: Blob, durationSec: number) => void) {
  const rec = useRef<MediaRecorder | null>(null);
  const chunks = useRef<Blob[]>([]);
  const started = useRef(0);
  const cancelled = useRef(false);
  const [elapsed, setElapsed] = useState(0);
  const [recording, setRecording] = useState(false);
  const supported = typeof window !== "undefined" && "MediaRecorder" in window && !!navigator.mediaDevices;

  useEffect(() => {
    if (!recording) return;
    const t = setInterval(() => setElapsed((performance.now() - started.current) / 1000), 250);
    return () => clearInterval(t);
  }, [recording]);

  async function start() {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const r = new MediaRecorder(stream);
    chunks.current = [];
    cancelled.current = false;
    r.ondataavailable = (e) => chunks.current.push(e.data);
    r.onstop = () => {
      stream.getTracks().forEach((t) => t.stop());
      const dur = (performance.now() - started.current) / 1000;
      if (!cancelled.current && dur >= 0.5) onDone(new Blob(chunks.current, { type: r.mimeType }), dur);
      setRecording(false);
      setElapsed(0);
    };
    rec.current = r;
    started.current = performance.now();
    r.start();
    setRecording(true);
  }

  function stop(cancel = false) {
    cancelled.current = cancel;
    rec.current?.stop();
  }

  return { supported, recording, elapsed, start, stop };
}

export function Composer({
  conversationId,
  disabled,
  disabledReason,
  onSend,
}: {
  conversationId: string;
  disabled?: boolean;
  disabledReason?: string;
  onSend: (d: OutgoingDraft) => void;
}) {
  const [text, setText] = useState(() => getDraft(conversationId));
  const [error, setError] = useState("");
  const ta = useRef<HTMLTextAreaElement>(null);
  const docInput = useRef<HTMLInputElement>(null);
  const imgInput = useRef<HTMLInputElement>(null);

  const recorder = useRecorder((blob, durationSec) => {
    const waveform = Array.from({ length: 40 }, () => 0.25 + Math.random() * 0.75);
    onSend({ kind: "audio", url: URL.createObjectURL(blob), durationSec, waveform });
  });

  // Auto-altura do campo.
  useEffect(() => {
    const el = ta.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [text]);

  function update(v: string) {
    setText(v);
    setDraft(conversationId, v);
  }

  function sendText() {
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
  }

  function onFile(kind: "document" | "image") {
    return (e: ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (!file) return;
      if (file.size > MAX_FILE) return setError("Arquivo acima de 100 MB.");
      setError("");
      const url = URL.createObjectURL(file);
      if (kind === "image" && file.type.startsWith("image/")) {
        const img = new window.Image();
        img.onload = () =>
          onSend({ kind: "image", url, width: img.naturalWidth, height: img.naturalHeight, hd: img.naturalWidth > 1600 });
        img.src = url;
      } else {
        onSend({ kind: "document", url, fileName: file.name, mimeType: file.type || "application/octet-stream", sizeBytes: file.size });
      }
    };
  }

  if (disabled) {
    return (
      <div className="border-t border-line bg-panel px-4 py-3 text-center text-sm text-muted">{disabledReason}</div>
    );
  }

  return (
    <div className="border-t border-line bg-panel px-3 py-2.5">
      {error && <p className="px-2 pb-1 text-xs text-danger">{error}</p>}
      {recorder.recording ? (
        <div className="flex items-center gap-3 px-1">
          <IconButton label="Descartar gravação" onClick={() => recorder.stop(true)}>
            <Trash2 className="size-5 text-danger" />
          </IconButton>
          <span className="flex items-center gap-2 text-sm">
            <span className="size-2.5 animate-pulse rounded-full bg-danger" />
            Gravando {formatDuration(recorder.elapsed)}
          </span>
          <button
            type="button"
            onClick={() => recorder.stop()}
            aria-label="Enviar áudio"
            className="ml-auto inline-flex size-10 items-center justify-center rounded-full bg-brand text-white"
          >
            <SendHorizontal className="size-5" />
          </button>
        </div>
      ) : (
        <div className="flex items-end gap-1">
          <Menu
            align="left"
            className="bottom-full top-auto mb-2 grid w-64 grid-cols-6 gap-1 p-2"
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
            className="bottom-full top-auto mb-2"
            trigger={({ toggle }) => (
              <IconButton label="Anexar" onClick={toggle}>
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
              </>
            )}
          </Menu>
          <input ref={docInput} type="file" hidden onChange={onFile("document")} />
          <input ref={imgInput} type="file" accept="image/*" hidden onChange={onFile("image")} />

          <textarea
            ref={ta}
            rows={1}
            value={text}
            onChange={(e) => update(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Digite uma mensagem"
            aria-label="Mensagem"
            className="mx-1 max-h-40 min-h-10 flex-1 resize-none rounded-xl border border-line bg-elevated px-4 py-2.5 text-sm outline-none placeholder:text-muted focus-visible:outline-none"
          />

          <AnimatePresence mode="popLayout" initial={false}>
            <motion.span
              key={text.trim() ? "send" : "mic"}
              initial={{ scale: 0.4, opacity: 0, rotate: -30 }}
              animate={{ scale: 1, opacity: 1, rotate: 0 }}
              exit={{ scale: 0.4, opacity: 0, rotate: 30 }}
              transition={{ type: "spring", stiffness: 600, damping: 30 }}
              className="inline-flex"
            >
              {text.trim() ? (
                <IconButton label="Enviar" onClick={sendText} className="text-brand hover:text-brand">
                  <SendHorizontal className="size-5" />
                </IconButton>
              ) : (
                <IconButton
                  label={recorder.supported ? "Gravar áudio" : "Gravação de áudio indisponível neste navegador"}
                  disabled={!recorder.supported}
                  onClick={() => recorder.start().catch(() => setError("Permissão de microfone negada."))}
                  className={cn(recorder.recording && "text-danger")}
                >
                  {recorder.recording ? <Square className="size-5" /> : <Mic className="size-5" />}
                </IconButton>
              )}
            </motion.span>
          </AnimatePresence>
        </div>
      )}
    </div>
  );
}
