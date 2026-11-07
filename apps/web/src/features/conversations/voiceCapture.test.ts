import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { openVoiceCapture, recordingError } from "./voiceCapture";

const { release, mute } = vi.hoisted(() => {
  const release = vi.fn();
  return { release, mute: vi.fn(() => release) };
});
vi.mock("@/lib/sounds", () => ({ silenceSoundsForRecording: mute }));

let inputTrack: { stop: ReturnType<typeof vi.fn> };
let outputTrack: { stop: ReturnType<typeof vi.fn> };
let input: MediaStream;
let output: MediaStream;
let media: ReturnType<typeof vi.fn>;
let close: ReturnType<typeof vi.fn>;
let resume: ReturnType<typeof vi.fn>;
let filters: { type: string; frequency: { value: number }; Q: { value: number }; connect: ReturnType<typeof vi.fn> }[];
let source: { connect: ReturnType<typeof vi.fn> };
let destination: { stream: MediaStream; channelCount: number; channelCountMode: string };

beforeEach(() => {
  vi.clearAllMocks();
  inputTrack = { stop: vi.fn() }; outputTrack = { stop: vi.fn() };
  input = { getTracks: () => [inputTrack] } as unknown as MediaStream;
  output = { getTracks: () => [outputTrack] } as unknown as MediaStream;
  media = vi.fn(async () => input);
  close = vi.fn(async () => undefined); resume = vi.fn(async () => undefined);
  filters = []; source = { connect: vi.fn() };
  destination = { stream: output, channelCount: 2, channelCountMode: "max" };
  vi.stubGlobal("navigator", { mediaDevices: { getUserMedia: media } });
  vi.stubGlobal("document", { querySelectorAll: () => [] });
  vi.stubGlobal("AudioContext", class {
    state = "running";
    close = close;
    resume = resume;
    createMediaStreamSource() { return source; }
    createMediaStreamDestination() { return destination; }
    createBiquadFilter() {
      const filter = { type: "", frequency: { value: 0 }, Q: { value: 0 }, connect: vi.fn() };
      filters.push(filter); return filter;
    }
  });
});
afterEach(() => vi.unstubAllGlobals());

describe("voice capture", () => {
  it("requests native denoising without forced gain and records a filtered mono stream", async () => {
    const capture = await openVoiceCapture();
    expect(media).toHaveBeenCalledWith({ audio: expect.objectContaining({ noiseSuppression: { ideal: true }, echoCancellation: { ideal: true }, autoGainControl: { ideal: false }, sampleRate: { ideal: 48_000 }, channelCount: { ideal: 1 } }) });
    expect(capture.stream).toBe(output);
    expect(destination.channelCount).toBe(1);
    expect(filters.map(f => [f.type, f.frequency.value])).toEqual([["highpass", 80], ["lowpass", 12_000]]);
    expect(source.connect).toHaveBeenCalledWith(filters[0]);
    expect(filters[0]!.connect).toHaveBeenCalledWith(filters[1]);
    expect(filters[1]!.connect).toHaveBeenCalledWith(destination);
    capture.dispose(); capture.dispose();
    expect(inputTrack.stop).toHaveBeenCalledOnce();
    expect(outputTrack.stop).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();
    expect(release).toHaveBeenCalledOnce();
  });

  it("keeps native capture available if the processing context is blocked", async () => {
    resume.mockRejectedValueOnce(new Error("blocked"));
    const capture = await openVoiceCapture();
    expect(capture.stream).toBe(input);
    expect(inputTrack.stop).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalledOnce();
    capture.dispose();
    expect(inputTrack.stop).toHaveBeenCalledOnce();
    expect(release).toHaveBeenCalledOnce();
  });

  it("does not mute sounds or leave resources open after a microphone permission failure", async () => {
    const denied = new DOMException("denied", "NotAllowedError");
    media.mockRejectedValueOnce(denied);
    await expect(openVoiceCapture()).rejects.toBe(denied);
    expect(mute).not.toHaveBeenCalled();
    expect(recordingError(denied)).toContain("Permissão");
    expect(recordingError(new DOMException("missing", "NotFoundError"))).toContain("Nenhum microfone");
  });
});
