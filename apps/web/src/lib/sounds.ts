import { useEffect, useSyncExternalStore } from "react";
import { useLocation } from "react-router";
import { safeStorage } from "./storage";
import sendURL from "@/assets/sounds/message-send.mp3";
import receiveURL from "@/assets/sounds/message-receive.mp3";
import recordURL from "@/assets/sounds/record-start.mp3";
import defaultURL from "@/assets/sounds/notification-default.mp3";
import waterURL from "@/assets/sounds/notification-water.mp3";
import nightURL from "@/assets/sounds/notification-night.mp3";
import hypnoticURL from "@/assets/sounds/notification-hypnotic.mp3";

export const notificationTones = {
  default: { label: "Padrão", url: defaultURL },
  water: { label: "Águas", url: waterURL },
  night: { label: "Noites", url: nightURL },
  hypnotic: { label: "Hipnótico", url: hypnoticURL },
} as const;
export type NotificationTone = keyof typeof notificationTones;
/** Quando o chat interno toca som: só quando a pessoa é mencionada (padrão), a cada mensagem ou nunca. */
export type InternalChatSound = "mentions" | "all" | "off";
export const internalChatSounds: Record<InternalChatSound, string> = { mentions: "Só quando me mencionarem", all: "Todas as mensagens", off: "Nunca" };
export type SoundPreferences = { messages: boolean; notifications: boolean; tone: NotificationTone; volume: number; internalChat: InternalChatSound };
const defaults: SoundPreferences = { messages: true, notifications: true, tone: "default", volume: 0.8, internalChat: "mentions" };
/** Identificador do chat interno para os sons: aberto e em foco toca o som de dentro da conversa; fora dele, a notificação. */
export const INTERNAL_SOUND_CONVERSATION = "internal-chat";
let preferences = defaults;
let userId: string | null = null;
let activeConversation: string | null = null;
let generation = 0;
let playbackVersion = 0;
let context: AudioContext | null = null;
let master: GainNode | null = null;
const buffers = new Map<string, Promise<AudioBuffer>>();
const sources = new Set<AudioBufferSourceNode>();
const recordingMutes = new Set<symbol>();
const heard = new Set<string>();
const listeners = new Set<() => void>();
const lastPlayed = { messages: -Infinity, notifications: -Infinity };
const preferenceKey = () => `chathm:sounds:${userId}`;
const notify = () => listeners.forEach((listener) => listener());

function normalizePreferences(saved: Partial<SoundPreferences> | null): SoundPreferences {
  return {
    messages: typeof saved?.messages === "boolean" ? saved.messages : defaults.messages,
    notifications: typeof saved?.notifications === "boolean" ? saved.notifications : defaults.notifications,
    tone: saved?.tone && Object.hasOwn(notificationTones, saved.tone) ? saved.tone : defaults.tone,
    volume: typeof saved?.volume === "number" && Number.isFinite(saved.volume) ? Math.max(0, Math.min(1, saved.volume)) : defaults.volume,
    internalChat: saved?.internalChat && Object.hasOwn(internalChatSounds, saved.internalChat) ? saved.internalChat : defaults.internalChat,
  };
}

function readPreferences(): SoundPreferences {
  try {
    return normalizePreferences(JSON.parse(safeStorage.getItem(preferenceKey()) ?? "null") as Partial<SoundPreferences> | null);
  } catch { return { ...defaults }; }
}

export function useSoundPreferences() {
  return useSyncExternalStore((listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; }, () => preferences, () => defaults);
}

export const getInternalChatSound = (): InternalChatSound => preferences.internalChat;

export function updateSoundPreferences(change: Partial<SoundPreferences>) {
  if (!userId) return;
  preferences = normalizePreferences({ ...preferences, ...change });
  safeStorage.setItem(preferenceKey(), JSON.stringify(preferences));
  if (master) master.gain.value = preferences.volume;
  if (!preferences.volume || (!preferences.messages && !preferences.notifications)) stopSounds();
  notify();
}

function stopSounds() {
  playbackVersion++;
  for (const source of sources) { try { source.stop(); } catch { /* Already ended. */ } }
  sources.clear();
}

async function unlockAudio(): Promise<boolean> {
  if (!userId || typeof window === "undefined") return false;
  try {
    if (!context || context.state === "closed") {
      const AudioContextClass = window.AudioContext;
      if (!AudioContextClass) return false;
      context = new AudioContextClass();
      master = context.createGain();
      master.gain.value = preferences.volume;
      master.connect(context.destination);
      buffers.clear();
    }
    if (context.state !== "running") await context.resume();
    return context.state === "running";
  } catch { return false; }
}

type SoundKind = "messages" | "notifications" | "preview" | "recording";
const enabled = (kind: SoundKind) => !!userId && recordingMutes.size === 0 && preferences.volume > 0 && (kind === "preview" || preferences[kind === "recording" ? "messages" : kind]);

/** Não deixa efeitos do app vazarem para o microfone; a liberação é idempotente. */
export function silenceSoundsForRecording() {
  const token = Symbol();
  recordingMutes.add(token);
  stopSounds();
  return () => { recordingMutes.delete(token); };
}

async function playClip(url: string, kind: SoundKind, session: number, waitForEnd = false): Promise<boolean> {
  const audio = context;
  const version = playbackVersion;
  if (!audio || audio.state !== "running" || !master || session !== generation || !enabled(kind)) return false;
  try {
    let buffer = buffers.get(url);
    if (!buffer) {
      buffer = fetch(url).then((r) => { if (!r.ok) throw new Error("sound unavailable"); return r.arrayBuffer(); }).then((data) => audio.decodeAudioData(data));
      buffers.set(url, buffer);
      void buffer.catch(() => buffers.delete(url));
    }
    const decoded = await buffer;
    if (session !== generation || version !== playbackVersion || audio.state !== "running" || !enabled(kind)) return false;
    if (kind === "messages" || kind === "notifications") {
      const now = Date.now();
      if (now - lastPlayed[kind] < (kind === "notifications" ? 1800 : 180)) return false;
      lastPlayed[kind] = now;
    } else stopSounds();
    const source = audio.createBufferSource();
    source.buffer = decoded;
    source.connect(master);
    sources.add(source);
    let finish: (() => void) | undefined;
    const ended = waitForEnd ? new Promise<void>(resolve => {
      let complete = false;
      const timer = setTimeout(() => { try { source.stop(); } catch { /* Already ended. */ } finish?.(); }, Math.max(0, decoded.duration || 0.3) * 1000 + 500);
      finish = () => {
        if (complete) return;
        complete = true;
        clearTimeout(timer);
        sources.delete(source); source.disconnect(); resolve();
      };
    }) : undefined;
    source.onended = () => { if (finish) finish(); else { sources.delete(source); source.disconnect(); } };
    source.start();
    if (ended) await ended;
    return true;
  } catch { return false; }
}

// Deduplicate live events, including across tabs when Web Locks is available.
async function claimEvent(key: string): Promise<boolean> {
  if (!userId || heard.has(key)) return false;
  heard.add(key);
  if (heard.size > 500) heard.delete(heard.values().next().value!);
  const owner = userId;
  const claim = () => {
    if (userId !== owner) return false;
    const storageKey = `chathm:sounds:recent:${owner}`;
    let recent: { key: string; at: number }[] = [];
    try {
      const saved: unknown = JSON.parse(safeStorage.getItem(storageKey) ?? "[]");
      if (Array.isArray(saved)) recent = saved.filter((x) => typeof x?.key === "string" && typeof x?.at === "number" && Date.now() - x.at < 120_000);
    } catch { /* Use the in-memory deduplication when storage is unavailable. */ }
    if (recent.some((x) => x.key === key)) return false;
    safeStorage.setItem(storageKey, JSON.stringify([...recent.slice(-199), { key, at: Date.now() }]));
    return true;
  };
  try {
    if (typeof navigator !== "undefined" && navigator.locks) return await navigator.locks.request(`chathm:sounds:${owner}`, claim);
    return claim();
  } catch { return claim(); }
}

export async function playSentSound(id: string, status?: string) {
  if (!id || !enabled("messages") || !context || context.state !== "running" || !status || status === "pending" || status === "failed") return;
  const session = generation;
  if (await claimEvent(`sent:${id}`)) await playClip(sendURL, "messages", session);
}

export async function playRecordStartSound() {
  if (!enabled("messages")) return;
  const session = generation;
  if (await unlockAudio()) await playClip(recordURL, "recording", session, true);
}

export async function playIncomingSound(message: { id: string; conversationId: string; direction: string; sentAt: string; historical?: boolean }) {
  if (!userId || message.direction !== "in" || message.historical || !message.id || !context || context.state !== "running") return;
  const age = Date.now() - Date.parse(message.sentAt);
  if (!Number.isFinite(age) || age > 60_000 || age < -30_000) return;
  const session = generation;
  const focused = typeof document !== "undefined" && document.visibilityState === "visible" && document.hasFocus();
  // Let the foreground tab claim the event before background tabs.
  if (!focused) await new Promise((resolve) => setTimeout(resolve, 150));
  if (session !== generation) return;
  const inChat = focused && activeConversation === message.conversationId;
  const kind = inChat ? "messages" : "notifications";
  if (!(await claimEvent(`received:${message.id}`)) || !enabled(kind)) return;
  await playClip(inChat ? receiveURL : notificationTones[preferences.tone].url, kind, session);
}

export async function previewNotification(tone: NotificationTone) {
  if (!(await unlockAudio())) return false;
  return playClip(notificationTones[tone].url, "preview", generation);
}

export function startSoundSession(id: string) {
  generation++;
  userId = id;
  activeConversation = null;
  heard.clear();
  lastPlayed.messages = lastPlayed.notifications = -Infinity;
  preferences = readPreferences();
  if (master) master.gain.value = preferences.volume;
  notify();
  const session = generation;
  const unlock = () => { void unlockAudio(); };
  const onStorage = (event: StorageEvent) => {
    if (event.key !== null && event.key !== preferenceKey()) return;
    preferences = readPreferences();
    if (master) master.gain.value = preferences.volume;
    if (!preferences.volume || (!preferences.messages && !preferences.notifications)) stopSounds();
    notify();
  };
  document.addEventListener("pointerdown", unlock, { passive: true });
  document.addEventListener("keydown", unlock);
  window.addEventListener("storage", onStorage);
  return () => {
    document.removeEventListener("pointerdown", unlock);
    document.removeEventListener("keydown", unlock);
    window.removeEventListener("storage", onStorage);
    if (session !== generation) return;
    generation++;
    userId = null;
    activeConversation = null;
    heard.clear();
    stopSounds();
    if (context?.state === "running") void context.suspend().catch(() => undefined);
    preferences = defaults;
    notify();
  };
}

export function setSoundConversation(id: string | null) { activeConversation = id; }

export function useAppSounds(id: string) {
  const { pathname } = useLocation();
  useEffect(() => startSoundSession(id), [id]);
  useEffect(() => { setSoundConversation(pathname === "/interno" ? INTERNAL_SOUND_CONVERSATION : /^\/conversas\/([^/]+)\/?$/.exec(pathname)?.[1] ?? null); }, [id, pathname]);
}
