import { useSyncExternalStore } from "react";

/**
 * Aparência do chat por pessoa: intensidade do papel de parede (0-100) e cores de fundo dos balões.
 * O texto dos balões é escolhido sozinho (claro ou escuro) para sempre ter contraste legível.
 * Campos ausentes usam o padrão do tema. Fica no servidor (acompanha a pessoa) e em cache local (aplicado antes do primeiro paint).
 */
export type Appearance = { wallpaper?: number; out?: string; in?: string };

const KEY = "chathm:chat-vars";
const VARS = ["--wallpaper-i", "--c-out", "--c-out-text", "--c-in", "--c-in-text"] as const;

const HEX = /^#[0-9a-f]{6}$/i;

function channel(v: number): number {
  const c = v / 255;
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  return 0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255);
}

export function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/** Texto preto ou branco, o que tiver mais contraste com o fundo (para qualquer cor o melhor dos dois passa de 4,5:1). */
export function readableText(background: string): string {
  return contrast(background, "#ffffff") >= contrast(background, "#000000") ? "#ffffff" : "#000000";
}

export function sanitize(a: Appearance | null | undefined): Appearance {
  const out: Appearance = {};
  if (!a) return out;
  if (typeof a.wallpaper === "number" && a.wallpaper >= 0 && a.wallpaper <= 100) out.wallpaper = Math.round(a.wallpaper);
  if (typeof a.out === "string" && HEX.test(a.out)) out.out = a.out.toLowerCase();
  if (typeof a.in === "string" && HEX.test(a.in)) out.in = a.in.toLowerCase();
  return out;
}

/** Variáveis CSS que a aparência define (vazio = tudo no padrão do tema). */
export function appearanceVars(a: Appearance): Record<string, string> {
  const v: Record<string, string> = {};
  if (a.wallpaper !== undefined) v["--wallpaper-i"] = (a.wallpaper / 100).toFixed(2);
  if (a.out) {
    v["--c-out"] = a.out;
    v["--c-out-text"] = readableText(a.out);
  }
  if (a.in) {
    v["--c-in"] = a.in;
    v["--c-in-text"] = readableText(a.in);
  }
  return v;
}

export const presets: { id: string; label: string; value: Appearance }[] = [
  { id: "default", label: "Padrão", value: {} },
  { id: "soft", label: "Claro e suave", value: { wallpaper: 12, out: "#cfe8b0", in: "#ffffff" } },
  { id: "contrast", label: "Alto contraste", value: { wallpaper: 0, out: "#14532d", in: "#ffffff" } },
  { id: "blue", label: "Azul", value: { wallpaper: 18, out: "#2563eb", in: "#e8eefb" } },
  { id: "rose", label: "Rosa", value: { wallpaper: 18, out: "#be185d", in: "#fdf2f8" } },
  { id: "graphite", label: "Grafite", value: { wallpaper: 18, out: "#374151", in: "#e5e7eb" } },
];

export function sameAppearance(a: Appearance, b: Appearance): boolean {
  return a.wallpaper === b.wallpaper && (a.out ?? "") === (b.out ?? "") && (a.in ?? "") === (b.in ?? "");
}

// --- estado e aplicação ---------------------------------------------------------------------------

let current: Appearance = {};
const listeners = new Set<() => void>();

export function applyAppearance(a: Appearance) {
  const root = document.documentElement.style;
  for (const name of VARS) root.removeProperty(name);
  const vars = appearanceVars(a);
  for (const [name, value] of Object.entries(vars)) root.setProperty(name, value);
  try {
    if (Object.keys(vars).length > 0) localStorage.setItem(KEY, JSON.stringify(vars));
    else localStorage.removeItem(KEY);
  } catch {
    /* storage indisponível */
  }
}

export function setAppearance(a: Appearance) {
  const next = sanitize(a);
  // Sempre reaplica: o cache local pode ter sido aplicado antes do React (index.html) e divergir do servidor.
  applyAppearance(next);
  if (sameAppearance(next, current)) return;
  current = next;
  listeners.forEach((l) => l());
}

export function useAppearance(): Appearance {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}
