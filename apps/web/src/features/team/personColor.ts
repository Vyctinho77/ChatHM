import { useQuery } from "@tanstack/react-query";
import { get } from "@/lib/api";

/**
 * Cor do nome das pessoas da equipe. A paleta é a mesma do servidor (backend/internal/users/color.go) e os tons de cada cor ficam no CSS
 * (index.css: --pc-<chave> e --pc-<chave>-ink), um para o tema claro e outro para o escuro, para o nome ser legível em qualquer fundo.
 * A cor é da PESSOA (não do perfil): administrador, gestor e atendente usam a mesma paleta. O administrador escolhe em Equipe; sem escolha,
 * vale uma cor automática, sempre a mesma para a mesma pessoa.
 */
export const COLOR_KEYS = ["verde", "azul", "roxo", "rosa", "laranja", "ambar", "ciano", "vermelho", "lima", "indigo", "marrom", "grafite"] as const;
export type ColorKey = (typeof COLOR_KEYS)[number];

export const COLOR_LABEL: Record<ColorKey, string> = {
  verde: "Verde", azul: "Azul", roxo: "Roxo", rosa: "Rosa", laranja: "Laranja", ambar: "Âmbar",
  ciano: "Ciano", vermelho: "Vermelho", lima: "Lima", indigo: "Índigo", marrom: "Marrom", grafite: "Grafite",
};

export const isColorKey = (k: unknown): k is ColorKey => typeof k === "string" && (COLOR_KEYS as readonly string[]).includes(k);

/** As cores vivas: só estas são dadas automaticamente. Grafite e marrom (tons neutros) ficam só para escolha manual, porque parecem "sem cor". */
export const AUTO_KEYS: readonly ColorKey[] = COLOR_KEYS.filter((k) => k !== "grafite" && k !== "marrom");

function hashOf(id: string): number {
  let h = 0;
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return h;
}

/** Cor automática de uma pessoa fora da lista da equipe: estável pelo id, sempre uma cor viva. */
export function autoColorKey(id: string): ColorKey {
  return AUTO_KEYS[hashOf(id) % AUTO_KEYS.length]!;
}

/** Tom (graus no círculo de cores) de cada cor viva: serve para distribuir as cores o mais longe possível umas das outras. */
const HUE: Record<string, number> = { verde: 142, azul: 216, roxo: 272, rosa: 334, laranja: 24, ambar: 42, ciano: 190, vermelho: 4, lima: 78, indigo: 246 };
const hueGap = (a: string, b: string) => {
  const d = Math.abs(HUE[a]! - HUE[b]!);
  return Math.min(d, 360 - d);
};

/**
 * A cor de cada pessoa da equipe. As escolhidas pelo administrador valem como estão (e ficam reservadas). Quem não tem cor escolhida recebe uma cor viva:
 * a primeira vem do id (estável), e as seguintes são sempre a cor livre MAIS DISTANTE em tom das que já estão em uso, para duas pessoas nunca ficarem
 * parecidas (nada de laranja ao lado de vermelho enquanto houver cor melhor). Só repete cor quando acabarem as 10 vivas. O resultado não depende da
 * ordem da lista. Se a equipe mudar (entrar ou sair gente), as cores automáticas podem se reacomodar: para fixar a cor de alguém, o administrador a escolhe.
 */
export function assignColors(people: { id: string; color?: string }[]): Map<string, ColorKey> {
  const out = new Map<string, ColorKey>();
  const used: ColorKey[] = [];
  const auto: string[] = [];
  for (const p of people) {
    if (isColorKey(p.color)) {
      out.set(p.id, p.color);
      used.push(p.color);
    } else auto.push(p.id);
  }
  for (const id of auto.sort()) {
    const start = hashOf(id) % AUTO_KEYS.length;
    let pick: ColorKey = AUTO_KEYS[start]!;
    let best = -1;
    for (let i = 0; i < AUTO_KEYS.length; i++) {
      const k = AUTO_KEYS[(start + i) % AUTO_KEYS.length]!;
      if (used.includes(k)) continue;
      const apart = used.reduce((m, u) => Math.min(m, u in HUE ? hueGap(k, u) : 180), 180); // cor escolhida à mão e neutra (grafite/marrom) não pesa
      if (apart > best) {
        best = apart;
        pick = k;
      }
    }
    out.set(id, pick);
    used.push(pick);
  }
  return out;
}

/** A cor escolhida, se houver e for válida; senão a automática. */
export function resolveColorKey(id: string, chosen?: string | null): ColorKey {
  return isColorKey(chosen) ? chosen : autoColorKey(id);
}

/** Valor CSS da cor: para texto sobre o painel e a lista (acompanha o tema). */
export const colorVar = (k: ColorKey) => `var(--pc-${k})`;
/** Valor CSS para texto sobre o balão recebido (branco nos dois temas). */
export const colorInkVar = (k: ColorKey) => `var(--pc-${k}-ink)`;

type PersonColor = { id: string; color?: string };
const colorsOf = (people: PersonColor[]) => assignColors(people); // fora do componente: o react-query guarda o resultado até a lista mudar

/** A cor (chave) de uma pessoa, a partir da lista da equipe (a mesma consulta de "pessoas", já guardada pelo app). */
export function usePersonColorKey(id: string | undefined): ColorKey | undefined {
  const { data } = useQuery({
    queryKey: ["people"] as const,
    staleTime: 60_000,
    queryFn: async () => (await get<{ people: PersonColor[] }>("/people")).people,
    select: colorsOf,
  });
  return id ? (data?.get(id) ?? autoColorKey(id)) : undefined;
}
