import { fold } from "@/lib/search";

export type EmojiItem = { char: string; label: string; tags: string[]; search: string };
export type EmojiCategory = { key: string; label: string; items: EmojiItem[] };

/** Abas da cartela, como no WhatsApp: cada uma reúne grupos do Unicode (chave da emojibase). */
const TABS: { key: string; label: string; groups: string[] }[] = [
  { key: "smileys", label: "Carinhas e pessoas", groups: ["smileys-emotion", "people-body"] },
  { key: "animals", label: "Animais e natureza", groups: ["animals-nature"] },
  { key: "food", label: "Comida e bebida", groups: ["food-drink"] },
  { key: "activities", label: "Atividades", groups: ["activities"] },
  { key: "travel", label: "Viagens e lugares", groups: ["travel-places"] },
  { key: "objects", label: "Objetos", groups: ["objects"] },
  { key: "symbols", label: "Símbolos", groups: ["symbols"] },
  { key: "flags", label: "Bandeiras", groups: ["flags"] },
];

type Raw = { group?: number; label: string; order?: number; tags?: string[]; unicode: string };
type Messages = { groups: { key: string; order: number }[] };

let loading: Promise<EmojiCategory[]> | undefined;

/** Constrói as categorias a partir dos dados em português da emojibase (Unicode CLDR). Sem tons de pele: um emoji por conceito. */
export function buildCategories(raw: Raw[], messages: Messages): EmojiCategory[] {
  const keyOfGroup = new Map(messages.groups.map((g) => [g.order, g.key]));
  const byGroup = new Map<string, Raw[]>();
  for (const e of raw) {
    if (e.group === undefined || e.order === undefined) continue; // componentes (tons de pele, cabelos) e indicadores soltos
    const key = keyOfGroup.get(e.group);
    if (!key || key === "component") continue;
    (byGroup.get(key) ?? byGroup.set(key, []).get(key)!).push(e);
  }
  return TABS.map((t) => ({
    key: t.key,
    label: t.label,
    items: t.groups
      .flatMap((g) => (byGroup.get(g) ?? []).slice().sort((a, b) => a.order! - b.order!))
      .map((e) => ({ char: e.unicode, label: e.label, tags: e.tags ?? [], search: fold([e.label, ...(e.tags ?? [])].join(" ")) })),
  })).filter((c) => c.items.length > 0);
}

/** Carrega os emojis (só quando a cartela abre: são ~600 KB de nomes em português, guardados pelo navegador). */
export function loadEmojis(): Promise<EmojiCategory[]> {
  loading ??= Promise.all([import("emojibase-data/pt/compact.json"), import("emojibase-data/pt/messages.json")]).then(([c, m]) =>
    buildCategories(c.default as unknown as Raw[], m.default as unknown as Messages),
  );
  return loading;
}

/** Busca por nome ou palavra-chave ("coracao", "sorrindo"), sem diferenciar acentos nem maiúsculas. Todas as palavras precisam aparecer. */
export function searchEmojis(categories: EmojiCategory[], query: string, limit = 160): EmojiItem[] {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  const out: EmojiItem[] = [];
  for (const c of categories) {
    for (const e of c.items) {
      if (words.every((w) => e.search.includes(w))) {
        out.push(e);
        if (out.length >= limit) return out;
      }
    }
  }
  return out;
}

// Emojis usados há pouco (só neste navegador), do mais recente ao mais antigo.
const RECENT_KEY = "chathm:emoji-recent";
const RECENT_MAX = 24;

export function recentEmojis(): string[] {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) ?? "[]");
    return Array.isArray(raw) ? raw.filter((x): x is string => typeof x === "string").slice(0, RECENT_MAX) : [];
  } catch {
    return [];
  }
}

export function rememberEmoji(char: string) {
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify([char, ...recentEmojis().filter((x) => x !== char)].slice(0, RECENT_MAX)));
  } catch {
    // sem armazenamento (modo privado): vale só até recarregar
  }
}
