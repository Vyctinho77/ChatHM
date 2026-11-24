import { useSyncExternalStore } from "react";
import { markHeard } from "./audioHeard";

/**
 * Reprodução de áudios das conversas, fora das telas.
 *
 * O <audio> mora aqui (um só para o app inteiro), e não dentro de cada balão: ao sair da conversa a tela é desmontada, e com
 * o <audio> dentro dela o som cortava. Agora o áudio segue tocando em segundo plano (com o controle em AudioBar), o balão apenas
 * mostra e comanda o que está aqui, e só um áudio toca por vez.
 */

export type Track = {
  /** Id da mensagem. */
  id: string;
  conversationId: string;
  /** Endereço do arquivo (Ogg/Opus guardado pelo servidor). */
  url: string;
  /** Quem aparece no controle em segundo plano (nome do contato). */
  label: string;
  durationSec: number;
  /** Enviado por nós: não conta como "ouvido". */
  outgoing: boolean;
};

export type AudioView = { active: boolean; playing: boolean; pos: number; failed: boolean };

/** Aparelhos antigos (sobretudo iPhones) não tocam Ogg/Opus, o formato guardado. Eles recebem o mesmo áudio em AAC (.m4a), convertido pelo servidor. */
let oggOpusUnsupported: boolean | undefined;
function cannotPlayOggOpus(): boolean {
  if (oggOpusUnsupported === undefined) {
    try {
      oggOpusUnsupported = typeof document !== "undefined" && document.createElement("audio").canPlayType('audio/ogg; codecs="opus"') === "";
    } catch {
      oggOpusUnsupported = false;
    }
  }
  return oggOpusUnsupported;
}

/** Endereço da versão em AAC do áudio (só para anexos do servidor). */
function m4aUrl(url: string): string {
  return url.startsWith("/api/") ? `${url}${url.includes("?") ? "&" : "?"}format=m4a` : url;
}

/** O endereço que este aparelho consegue tocar (também usado para desenhar a onda). */
export function playableUrl(url: string): string {
  return cannotPlayOggOpus() ? m4aUrl(url) : url;
}

type State = { track: Track | null; playing: boolean; pos: number; failed: boolean };
const IDLE: State = { track: null, playing: false, pos: 0, failed: false };

let state: State = IDLE;
let usingM4a = false;
let retried = false; // erro e rejeição do play() chegam juntos: conta uma vez só
const positions = new Map<string, number>(); // onde cada áudio parou
const versions = new Map<string, number>();
let globalVersion = 0;
const listeners = new Set<() => void>();
let el: HTMLAudioElement | undefined;
let make = (): HTMLAudioElement => new Audio();

function emit(...ids: (string | undefined)[]) {
  for (const id of ids) if (id) versions.set(id, (versions.get(id) ?? 0) + 1);
  globalVersion++;
  listeners.forEach((l) => l());
}

function set(next: Partial<State>) {
  state = { ...state, ...next };
  emit(state.track?.id);
}

function audio(): HTMLAudioElement {
  if (el) return el;
  const a = make();
  a.preload = "metadata";
  a.addEventListener("play", () => {
    if (state.track && !state.track.outgoing) markHeard(state.track.id);
    set({ playing: true, failed: false });
  });
  a.addEventListener("pause", () => {
    if (state.track) positions.set(state.track.id, a.currentTime);
    set({ playing: false });
  });
  a.addEventListener("timeupdate", () => set({ pos: a.currentTime }));
  a.addEventListener("ended", () => {
    if (state.track) positions.delete(state.track.id);
    set({ playing: false, pos: 0 });
  });
  a.addEventListener("error", () => fail());
  el = a;
  return a;
}

/** O aparelho não conseguiu tocar: tenta a versão em AAC (uma vez) e, se também não der, avisa. */
function fail() {
  const t = state.track;
  if (!t) return;
  if (usingM4a || cannotPlayOggOpus() || !t.url.startsWith("/api/")) {
    set({ playing: false, failed: true });
    return;
  }
  if (retried) return;
  retried = true;
  usingM4a = true;
  const a = audio();
  const at = a.currentTime || positions.get(t.id) || 0;
  a.src = m4aUrl(t.url);
  a.currentTime = at;
  a.load();
  a.play().catch(() => set({ playing: false, failed: true }));
}

function load(t: Track) {
  const a = audio();
  usingM4a = false;
  retried = false;
  a.src = playableUrl(t.url);
  a.currentTime = positions.get(t.id) ?? 0;
}

function play() {
  const a = audio();
  set({ failed: false });
  a.play().catch(() => fail());
}

function mediaSession(t: Track) {
  if (typeof navigator === "undefined" || !("mediaSession" in navigator)) return;
  try {
    navigator.mediaSession.metadata = new MediaMetadata({ title: `Áudio de ${t.label}`, artist: "ChatHM" });
    navigator.mediaSession.setActionHandler("play", () => play());
    navigator.mediaSession.setActionHandler("pause", () => pause());
    navigator.mediaSession.setActionHandler("seekto", (d) => {
      if (d.seekTime != null) audio().currentTime = d.seekTime;
    });
  } catch {
    // navegador sem os controles de mídia: o áudio toca do mesmo jeito
  }
}

/** Toca o áudio; se já é o atual, alterna entre tocar e pausar; se é outro, o que tocava pausa (e lembra onde parou). */
export function toggleTrack(t: Track) {
  if (state.track?.id === t.id) {
    if (audio().paused) play();
    else audio().pause();
    return;
  }
  const previous = state.track;
  if (previous) {
    positions.set(previous.id, audio().currentTime);
    audio().pause();
  }
  state = { track: t, playing: false, pos: positions.get(t.id) ?? 0, failed: false };
  load(t);
  mediaSession(t);
  emit(previous?.id, t.id);
  play();
}

export function pause() {
  if (state.track && el && !el.paused) el.pause();
}

/** Fecha o controle: para o áudio e esquece o atual (as posições de cada áudio continuam guardadas). */
export function stopAudio() {
  const previous = state.track;
  if (el) {
    if (previous) positions.set(previous.id, el.currentTime);
    el.pause();
    el.removeAttribute("src");
    el.load();
  }
  state = IDLE;
  emit(previous?.id);
}

/** Vai para um ponto (em segundos) do áudio, tocando ou não. */
export function seekTrack(t: Pick<Track, "id">, seconds: number) {
  if (state.track?.id === t.id && el) {
    el.currentTime = seconds;
    set({ pos: seconds });
    return;
  }
  positions.set(t.id, seconds);
  emit(t.id);
}

function viewOf(id: string): AudioView {
  const active = state.track?.id === id;
  return active ? { active, playing: state.playing, pos: state.pos, failed: state.failed } : { active, playing: false, pos: positions.get(id) ?? 0, failed: false };
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => void listeners.delete(fn);
}

/** Estado de um áudio (o balão se atualiza só quando o dele muda). */
export function useAudioView(id: string): AudioView {
  useSyncExternalStore(subscribe, () => versions.get(id) ?? 0, () => 0);
  return viewOf(id);
}

/** O áudio atual (ou null) e como ele está; o controle em segundo plano se atualiza a cada mudança. */
export function useCurrentAudio(): { track: Track; playing: boolean; pos: number; failed: boolean } | null {
  useSyncExternalStore(subscribe, () => globalVersion, () => 0);
  return state.track ? { track: state.track, playing: state.playing, pos: state.pos, failed: state.failed } : null;
}

/** Só para testes: troca a fábrica do <audio> e zera tudo. */
export function __resetAudioEngine(factory?: () => HTMLAudioElement) {
  state = IDLE;
  usingM4a = false;
  retried = false;
  positions.clear();
  versions.clear();
  globalVersion = 0;
  el = undefined;
  make = factory ?? (() => new Audio());
}
