import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";

const KEY = "chathm:theme";

export function getStoredTheme(): Theme {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    /* storage indisponível */
  }
  return "system";
}

const media = () => window.matchMedia("(prefers-color-scheme: dark)");

export function applyTheme(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && media().matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.style.colorScheme = dark ? "dark" : "light";
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    /* ignore */
  }
}

/** Mantém o tema "sistema" sincronizado com a preferência do dispositivo. */
export function useSystemThemeSync(theme: Theme) {
  useEffect(() => {
    applyTheme(theme);
    if (theme !== "system") return;
    const m = media();
    const onChange = () => applyTheme("system");
    m.addEventListener("change", onChange);
    return () => m.removeEventListener("change", onChange);
  }, [theme]);
}

export function useMediaQuery(query: string) {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const m = window.matchMedia(query);
    const onChange = () => setMatches(m.matches);
    m.addEventListener("change", onChange);
    return () => m.removeEventListener("change", onChange);
  }, [query]);
  return matches;
}
