import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Sounds = typeof import("./sounds");
type Decoded = AudioBuffer & { url: string };
let sounds: Sounds;
let cleanup: (() => void) | undefined;
let documentEvents: EventTarget & { visibilityState: string; hasFocus: () => boolean };
let storage: Map<string, string>;
let started: string[];
let contexts: FakeContext[];
let request: ReturnType<typeof vi.fn>;
let finishClips: boolean;
let endClips: (() => void)[];

class FakeContext {
  state = "suspended";
  destination = {};
  gain = { gain: { value: 1 }, connect: vi.fn() };
  resume = vi.fn(async () => { this.state = "running"; });
  suspend = vi.fn(async () => { this.state = "suspended"; });
  constructor() { contexts.push(this); }
  createGain() { return this.gain; }
  async decodeAudioData(data: ArrayBuffer) { return { url: new TextDecoder().decode(data) } as Decoded; }
  createBufferSource() {
    const source = { buffer: null as Decoded | null, connect: vi.fn(), disconnect: vi.fn(), stop: vi.fn(), onended: null as (() => void) | null, start: () => {
      started.push(source.buffer!.url);
      const end = () => source.onended?.();
      endClips.push(end);
      if (finishClips) queueMicrotask(end);
    } };
    return source;
  }
}

const incoming = (id = "message") => ({ id, conversationId: "chat", direction: "in", sentAt: new Date(Date.now()).toISOString() });
function unlock() { documentEvents.dispatchEvent(new Event("pointerdown")); }

beforeEach(async () => {
  vi.resetModules();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-07T16:00:00Z"));
  storage = new Map(); started = []; contexts = []; finishClips = true; endClips = [];
  documentEvents = Object.assign(new EventTarget(), { visibilityState: "visible", hasFocus: () => true });
  const windowEvents = Object.assign(new EventTarget(), { AudioContext: FakeContext, localStorage: { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) } });
  vi.stubGlobal("window", windowEvents);
  vi.stubGlobal("document", documentEvents);
  request = vi.fn(async (_name: string, claim: () => boolean) => claim());
  vi.stubGlobal("navigator", { locks: { request } });
  vi.stubGlobal("fetch", vi.fn(async (url: string) => ({ ok: true, arrayBuffer: async () => new TextEncoder().encode(url).buffer })));
  sounds = await import("./sounds");
  cleanup = sounds.startSoundSession("user-a");
});

afterEach(() => { cleanup?.(); cleanup = undefined; vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("app sound effects", () => {
  it("finishes the microphone cue before resolving and does not throttle it after another message sound", async () => {
    unlock();
    await sounds.playSentSound("just-sent", "sent");
    finishClips = false;
    let ready = false;
    const cue = sounds.playRecordStartSound().then(() => { ready = true; });
    await vi.advanceTimersByTimeAsync(0);
    expect(started.at(-1)).toContain("record-start");
    expect(ready).toBe(false);
    endClips.at(-1)!();
    await cue;
    expect(ready).toBe(true);
  });

  it("silences effects during recording and restores preferences when capture ends", async () => {
    unlock();
    await sounds.playRecordStartSound();
    const release = sounds.silenceSoundsForRecording();
    await sounds.playSentSound("while-recording", "sent");
    expect(await sounds.previewNotification("water")).toBe(false);
    expect(started).toHaveLength(1);
    release(); release();
    await vi.advanceTimersByTimeAsync(200);
    await sounds.playSentSound("after-recording", "sent");
    expect(started).toHaveLength(2);
  });

  it("plays the microphone click effect and respects message mute and volume", async () => {
    await sounds.playRecordStartSound();
    expect(started).toHaveLength(1);
    expect(started[0]).toContain("record-start");
    sounds.updateSoundPreferences({ messages: false });
    await sounds.playRecordStartSound();
    expect(started).toHaveLength(1);
    sounds.updateSoundPreferences({ messages: true, volume: 0 });
    await sounds.playRecordStartSound();
    expect(started).toHaveLength(1);
  });

  it("waits for interaction and plays only confirmed outgoing messages once", async () => {
    await sounds.playSentSound("before-interaction", "sent");
    expect(contexts).toHaveLength(0);
    unlock();
    await sounds.playSentSound("failed", "failed");
    await sounds.playSentSound("pending", "pending");
    expect(started).toHaveLength(0);
    await sounds.playSentSound("sent", "sent");
    await sounds.playSentSound("sent", "read");
    expect(started).toHaveLength(1);
    expect(started[0]).toContain("message-send");
    expect(request).toHaveBeenCalled();
  });

  it("uses receive for the open chat and ignores history, old messages and own-device messages", async () => {
    unlock(); sounds.setSoundConversation("chat");
    await sounds.playIncomingSound(incoming());
    await sounds.playIncomingSound(incoming());
    await sounds.playIncomingSound({ ...incoming("history"), historical: true });
    await sounds.playIncomingSound({ ...incoming("old"), sentAt: new Date(Date.now() - 120_000).toISOString() });
    await sounds.playIncomingSound({ ...incoming("outgoing"), direction: "out" });
    expect(started).toHaveLength(1);
    expect(started[0]).toContain("message-receive");
  });

  it("plays the internal chat in-app sound when it is open, and the notification when it is not", async () => {
    unlock();
    expect(sounds.getInternalChatSound()).toBe("mentions"); // padrão: só quando me mencionam
    sounds.setSoundConversation(sounds.INTERNAL_SOUND_CONVERSATION);
    await sounds.playIncomingSound({ ...incoming("open"), conversationId: sounds.INTERNAL_SOUND_CONVERSATION });
    expect(started[0]).toContain("message-receive");
    sounds.setSoundConversation("another-chat");
    await sounds.playIncomingSound({ ...incoming("closed"), conversationId: sounds.INTERNAL_SOUND_CONVERSATION });
    expect(started[1]).toContain("notification-default");
  });

  it("keeps the internal chat sound preference, falling back to mentions for anything unknown", async () => {
    sounds.updateSoundPreferences({ internalChat: "all" });
    expect(sounds.getInternalChatSound()).toBe("all");
    sounds.updateSoundPreferences({ internalChat: "off" });
    expect(sounds.getInternalChatSound()).toBe("off");
    sounds.updateSoundPreferences({ internalChat: "whatever" as never });
    expect(sounds.getInternalChatSound()).toBe("mentions");
    expect(Object.keys(sounds.internalChatSounds)).toEqual(["mentions", "all", "off"]);
  });

  it("uses the selected notification for another chat and respects separate mute options", async () => {
    unlock(); sounds.setSoundConversation("another-chat");
    sounds.updateSoundPreferences({ tone: "water" });
    await sounds.playIncomingSound(incoming());
    expect(started[0]).toContain("notification-water");
    sounds.updateSoundPreferences({ notifications: false });
    await sounds.playIncomingSound(incoming("muted-notification"));
    expect(started).toHaveLength(1);
    sounds.setSoundConversation("chat");
    await sounds.playIncomingSound(incoming("receive"));
    expect(started[1]).toContain("message-receive");
    sounds.updateSoundPreferences({ messages: false });
    await sounds.playSentSound("muted-send", "sent");
    expect(started).toHaveLength(2);
  });

  it("uses a notification when the tab is hidden and limits bursts", async () => {
    unlock(); sounds.setSoundConversation("chat"); documentEvents.visibilityState = "hidden";
    const first = sounds.playIncomingSound(incoming());
    await vi.advanceTimersByTimeAsync(150); await first;
    expect(started[0]).toContain("notification-default");
    const next = sounds.playIncomingSound(incoming("burst"));
    await vi.advanceTimersByTimeAsync(150); await next;
    expect(started).toHaveLength(1);
  });

  it("supports all four previews, caches decoded clips and applies volume", async () => {
    sounds.updateSoundPreferences({ volume: 0.4 });
    for (const tone of ["default", "water", "night", "hypnotic"] as const) expect(await sounds.previewNotification(tone)).toBe(true);
    expect(started).toHaveLength(4);
    expect(contexts[0]?.gain.gain.value).toBe(0.4);
    await sounds.previewNotification("default");
    expect(fetch).toHaveBeenCalledTimes(4);
    sounds.updateSoundPreferences({ volume: 0 });
    expect(await sounds.previewNotification("default")).toBe(false);
  });

  it("keeps account preferences separate and prevents replay after reopening the same account", async () => {
    unlock(); await sounds.playSentSound("original", "sent");
    sounds.updateSoundPreferences({ tone: "night", volume: 0.25 });
    cleanup!(); cleanup = sounds.startSoundSession("user-b"); unlock();
    expect(contexts[0]?.gain.gain.value).toBe(0.8);
    cleanup(); cleanup = sounds.startSoundSession("user-a"); unlock();
    expect(contexts[0]?.gain.gain.value).toBe(0.25);
    await sounds.playSentSound("original", "sent");
    expect(started).toHaveLength(1);
    expect(JSON.parse(storage.get("chathm:sounds:user-a")!).tone).toBe("night");
  });

  it("handles blocked audio and failed downloads without affecting messaging", async () => {
    unlock(); contexts[0]!.state = "suspended";
    contexts[0]!.resume.mockRejectedValue(new Error("blocked"));
    expect(await sounds.previewNotification("default")).toBe(false);
    contexts[0]!.state = "running";
    vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
    expect(await sounds.previewNotification("default")).toBe(false);
    expect(await sounds.previewNotification("default")).toBe(true);
  });

  it("cancels pending playback when the session ends", async () => {
    unlock();
    let complete!: (response: { ok: boolean; arrayBuffer: () => Promise<ArrayBuffer> }) => void;
    vi.mocked(fetch).mockImplementationOnce(() => new Promise((resolve) => { complete = resolve as typeof complete; }));
    const pending = sounds.playSentSound("late", "sent");
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledOnce();
    cleanup!(); cleanup = undefined;
    complete({ ok: true, arrayBuffer: async () => new ArrayBuffer(0) });
    await pending;
    expect(started).toHaveLength(0);
  });
});
