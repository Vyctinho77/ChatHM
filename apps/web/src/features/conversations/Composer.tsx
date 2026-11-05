import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { FileText, Image, MapPin, Mic, Paperclip, SendHorizontal, Smile, Sparkles, Square, Trash2 } from "lucide-react";
import { Menu, MenuItem } from "@/components/Menu";
import { IconButton } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatDuration } from "@/lib/format";
import { getDraft, setDraft } from "./drafts";
import type { OutgoingDraft } from "./store";

const quickEmojis = ["😀", "😉", "🙏", "👍", "👏", "✅", "📄", "📎", "⏰", "💰", "🎉", "❤️"];

const MAX_FILE = 25 * 1024 * 1024; // padrão do servidor (MAX_ATTACHMENT_MB); a validação real é do backend

/** Forma de onda real (40 barras de 0 a 1) a partir do áudio gravado; aleatória se o navegador não decodificar. */
async function waveformOf(blob: Blob, bars = 40): Promise<number[]> {
  try {
    const ctx = new AudioContext();
    const buf = await ctx.decodeAudioData(await blob.arrayBuffer());
    void ctx.close();
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
  }
}

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
  mediaDisabledReason,
  onSend,
}: {
  conversationId: string;
  disabled?: boolean;
  disabledReason?: string;
  /** Quando definido, anexos e gravação de áudio ficam desativados com esta explicação. */
  mediaDisabledReason?: string;
  onSend: (d: OutgoingDraft) => void;
}) {
  const [text, setText] = useState(() => getDraft(conversationId));
  const [error, setError] = useState("");
  const ta = useRef<HTMLTextAreaElement>(null);
  const docInput = useRef<HTMLInputElement>(null);
  const imgInput = useRef<HTMLInputElement>(null);
  const hdInput = useRef<HTMLInputElement>(null);

  const recorder = useRecorder((blob, durationSec) => {
    void waveformOf(blob).then((waveform) =>
      onSend({ kind: "audio", url: URL.createObjectURL(blob), durationSec, waveform, file: blob }),
    );
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

  function sendLocation() {
    if (!("geolocation" in navigator)) return setError("Localização indisponível neste navegador.");
    setError("");
    navigator.geolocation.getCurrentPosition(
      (pos) => onSend({ kind: "location", lat: pos.coords.latitude, lng: pos.coords.longitude }),
      () => setError("Não foi possível obter a localização (permissão negada?)."),
      { enableHighAccuracy: true, timeout: 10_000 },
    );
  }

  function onFile(kind: "document" | "image", hd = false) {
    return (e: ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      e.target.value = "";
      if (!file) return;
      if (file.size > MAX_FILE) return setError("Arquivo acima de 25 MB.");
      setError("");
      const url = URL.createObjectURL(file);
      if (kind === "image" && file.type.startsWith("image/")) {
        const img = new window.Image();
        img.onload = () =>
          onSend({ kind: "image", url, width: img.naturalWidth, height: img.naturalHeight, hd, file });
        img.src = url;
      } else {
        onSend({ kind: "document", url, fileName: file.name, mimeType: file.type || "application/octet-stream", sizeBytes: file.size, file });
      }
    };
  }

  if (disabled) {
    return (
      <div className="border-t border-line bg-panel px-4 py-3 text-center text-[12.5px] text-muted">{disabledReason}</div>
    );
  }

  return (
    <div className="border-t border-line bg-panel px-2.5 py-2">
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
                disabled={!!mediaDisabledReason}
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
          <input ref={docInput} type="file" hidden onChange={onFile("document")} />
          <input ref={imgInput} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={onFile("image")} />
          <input ref={hdInput} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={onFile("image", true)} />

          <textarea
            ref={ta}
            rows={1}
            value={text}
            onChange={(e) => update(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Digite uma mensagem"
            aria-label="Mensagem"
            className="mx-1 max-h-40 min-h-9 flex-1 resize-none rounded-lg border border-line bg-elevated px-3.5 py-2 text-[13.5px] leading-5 outline-none placeholder:text-muted focus-visible:outline-none"
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
                  label={
                    mediaDisabledReason ??
                    (recorder.supported ? "Gravar áudio" : "Gravação de áudio indisponível neste navegador")
                  }
                  disabled={!recorder.supported || !!mediaDisabledReason}
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
