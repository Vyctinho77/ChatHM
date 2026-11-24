import { beforeEach, describe, expect, it } from "vitest";
import { __resetAudioEngine, pause, seekTrack, stopAudio, toggleTrack, type Track } from "./audioEngine";

// <audio> falso: play/pause/eventos à mão.
class FakeAudio {
  src = "";
  currentTime = 0;
  paused = true;
  preload = "";
  failNextPlay = false;
  plays: string[] = [];
  private on = new Map<string, (() => void)[]>();
  addEventListener(n: string, f: () => void) {
    this.on.set(n, [...(this.on.get(n) ?? []), f]);
  }
  emit(n: string) {
    (this.on.get(n) ?? []).forEach((f) => f());
  }
  setAttribute() {}
  removeAttribute(n: string) {
    if (n === "src") this.src = "";
  }
  load() {}
  play() {
    this.plays.push(this.src);
    if (this.failNextPlay) {
      this.failNextPlay = false;
      this.emit("error");
      return Promise.reject(new Error("falhou"));
    }
    this.paused = false;
    this.emit("play");
    return Promise.resolve();
  }
  pause() {
    if (this.paused) return;
    this.paused = true;
    this.emit("pause");
  }
}

let a: FakeAudio;
beforeEach(() => {
  a = new FakeAudio();
  __resetAudioEngine(() => a as unknown as HTMLAudioElement);
});

const track = (id: string, o: Partial<Track> = {}): Track => ({ id, conversationId: "c1", url: `/api/v1/attachments/${id}`, label: "Cliente", durationSec: 30, outgoing: false, ...o });

describe("áudio em segundo plano", () => {
  it("toca, pausa e retoma o mesmo áudio", () => {
    toggleTrack(track("a"));
    expect(a.paused).toBe(false);
    toggleTrack(track("a"));
    expect(a.paused).toBe(true);
    a.currentTime = 7;
    toggleTrack(track("a"));
    expect(a.paused).toBe(false);
  });

  it("só um áudio toca por vez e cada um lembra onde parou", () => {
    toggleTrack(track("a"));
    a.currentTime = 12;
    toggleTrack(track("b")); // pausa o a (parou em 12) e começa o b do início
    expect(a.src).toContain("/b");
    expect(a.currentTime).toBe(0);
    a.currentTime = 5;
    toggleTrack(track("a")); // volta ao a, de onde parou
    expect(a.src).toContain("/a");
    expect(a.currentTime).toBe(12);
  });

  it("segue tocando sem nenhuma tela: o <audio> não depende de balão montado", () => {
    toggleTrack(track("a"));
    // (nenhum componente existe aqui; é o caso de sair da conversa)
    expect(a.paused).toBe(false);
    expect(a.src).toContain("/a");
  });

  it("fechar o controle para o áudio", () => {
    toggleTrack(track("a"));
    stopAudio();
    expect(a.paused).toBe(true);
    expect(a.src).toBe("");
  });

  it("pause() para o que estiver tocando (ex.: ao começar a gravar)", () => {
    toggleTrack(track("a"));
    pause();
    expect(a.paused).toBe(true);
    pause(); // sem nada tocando: não faz nada
  });

  it("ir para um ponto de um áudio que não está tocando só guarda a posição", () => {
    toggleTrack(track("a"));
    seekTrack({ id: "b" }, 9);
    expect(a.paused).toBe(false);
    toggleTrack(track("b"));
    expect(a.currentTime).toBe(9);
  });

  it("se o aparelho não toca Ogg/Opus, tenta o AAC uma vez", () => {
    a.failNextPlay = true;
    toggleTrack(track("a"));
    expect(a.plays[0]).toBe("/api/v1/attachments/a");
    expect(a.plays[1]).toBe("/api/v1/attachments/a?format=m4a");
  });
});
