import { silenceSoundsForRecording } from "@/lib/sounds";

export const voiceConstraints: MediaTrackConstraints = {
  channelCount: { ideal: 1 },
  sampleRate: { ideal: 48_000 },
  echoCancellation: { ideal: true },
  noiseSuppression: { ideal: true },
  // Evita que o ganho aumente o chiado durante pausas na fala.
  autoGainControl: { ideal: false },
};

export type VoiceCapture = { stream: MediaStream; input: MediaStream; dispose: () => void };

export async function openVoiceCapture(): Promise<VoiceCapture> {
  const input = await navigator.mediaDevices.getUserMedia({ audio: voiceConstraints });
  const releaseSounds = silenceSoundsForRecording();
  document.querySelectorAll("audio").forEach(audio => audio.pause());
  let context: AudioContext | undefined;
  let output: MediaStream | undefined;
  let disposed = false;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    input.getTracks().forEach(track => track.stop());
    output?.getTracks().forEach(track => track.stop());
    if (context && context.state !== "closed") void context.close().catch(() => undefined);
    releaseSounds();
  };
  try {
    context = new AudioContext({ sampleRate: 48_000 });
    await context.resume();
    const source = context.createMediaStreamSource(input);
    const highpass = context.createBiquadFilter();
    highpass.type = "highpass";
    highpass.frequency.value = 80;
    highpass.Q.value = Math.SQRT1_2;
    const lowpass = context.createBiquadFilter();
    lowpass.type = "lowpass";
    lowpass.frequency.value = 12_000;
    lowpass.Q.value = Math.SQRT1_2;
    const destination = context.createMediaStreamDestination();
    destination.channelCount = 1;
    destination.channelCountMode = "explicit";
    source.connect(highpass);
    highpass.connect(lowpass);
    lowpass.connect(destination);
    // Não conecta aos alto-falantes: a própria voz não retorna ao microfone.
    output = destination.stream;
    return { stream: output, input, dispose };
  } catch {
    // Se o navegador não permitir Web Audio, mantém a captura com supressão nativa.
    if (context && context.state !== "closed") void context.close().catch(() => undefined);
    context = undefined;
    return { stream: input, input, dispose };
  }
}

export function recordingError(error: unknown) {
  const name = error instanceof Error ? error.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") return "Permissão de microfone negada.";
  if (name === "NotFoundError") return "Nenhum microfone foi encontrado.";
  if (name === "NotReadableError") return "Não foi possível acessar o microfone. Confira se outro aplicativo está usando ele.";
  return "Não foi possível iniciar a gravação. Tente novamente.";
}
