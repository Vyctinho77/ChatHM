import { useEffect, useRef, useState } from "react";
import { openVoiceCapture, type VoiceCapture } from "./voiceCapture";
import { playRecordStartSound } from "@/lib/sounds";

export function useVoiceRecorder(onDone: (blob: Blob, durationSec: number) => void, onError: (message: string) => void) {
  const active = useRef<{ recorder: MediaRecorder; capture: VoiceCapture; cancelled: boolean } | null>(null);
  const startingRef = useRef(false);
  const version = useRef(0);
  const [starting, setStarting] = useState(false);
  const [recording, setRecording] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const started = useRef(0);
  const supported = typeof window !== "undefined" && "MediaRecorder" in window && !!navigator.mediaDevices?.getUserMedia;

  useEffect(() => () => {
    version.current++;
    const state = active.current;
    active.current = null;
    if (state) {
      state.cancelled = true;
      state.recorder.onstop = null;
      state.recorder.onerror = null;
      state.recorder.ondataavailable = null;
      if (state.recorder.state !== "inactive") state.recorder.stop();
      state.capture.dispose();
    }
  }, []);

  useEffect(() => {
    if (!recording) return;
    const timer = setInterval(() => setElapsed((performance.now() - started.current) / 1000), 250);
    return () => clearInterval(timer);
  }, [recording]);

  async function start() {
    if (startingRef.current || active.current) return;
    startingRef.current = true;
    setStarting(true);
    const current = version.current;
    let capture: VoiceCapture | undefined;
    try {
      await playRecordStartSound();
      if (current !== version.current) return;
      capture = await openVoiceCapture();
      if (current !== version.current) { capture.dispose(); return; }
      const mimeType = ["audio/webm;codecs=opus", "audio/ogg;codecs=opus"].find(type => MediaRecorder.isTypeSupported(type));
      const recorder = new MediaRecorder(capture.stream, { audioBitsPerSecond: 64_000, ...(mimeType ? { mimeType } : {}) });
      const state = { recorder, capture, cancelled: false };
      const chunks: Blob[] = [];
      active.current = state;
      recorder.ondataavailable = event => { if (event.data.size) chunks.push(event.data); };
      recorder.onstop = () => {
        const duration = (performance.now() - started.current) / 1000;
        capture!.dispose();
        if (active.current !== state) return;
        active.current = null;
        setRecording(false); setElapsed(0);
        if (!state.cancelled && duration >= 0.5 && chunks.length) onDone(new Blob(chunks, { type: recorder.mimeType }), duration);
      };
      recorder.onerror = () => {
        state.cancelled = true;
        capture!.dispose();
        if (active.current !== state) return;
        active.current = null;
        setRecording(false); setElapsed(0);
        onError("A gravação foi interrompida. Confira o microfone e tente novamente.");
      };
      capture.input.getAudioTracks().forEach(track => track.addEventListener("ended", () => {
        if (active.current !== state || state.cancelled) return;
        state.cancelled = true;
        if (recorder.state !== "inactive") recorder.stop();
        onError("O microfone foi desconectado. Grave o áudio novamente.");
      }, { once: true }));
      started.current = performance.now();
      recorder.start();
      setRecording(true);
    } catch (error) {
      capture?.dispose();
      active.current = null;
      throw error;
    } finally {
      startingRef.current = false;
      if (current === version.current) setStarting(false);
    }
  }

  function stop(cancelled = false) {
    const state = active.current;
    if (!state || state.recorder.state === "inactive") return;
    state.cancelled = cancelled;
    state.recorder.stop();
  }

  return { supported, starting, recording, elapsed, start, stop };
}
