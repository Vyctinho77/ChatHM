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

/** Cor automática: estável pelo id (a mesma pessoa tem sempre a mesma cor). */
export function autoColorKey(id: string): ColorKey {
  let h = 0;
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return COLOR_KEYS[h % COLOR_KEYS.length]!;
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

/** A cor (chave) de uma pessoa, a partir da lista da equipe (a mesma consulta de "pessoas", já guardada pelo app). */
export function usePersonColorKey(id: string | undefined): ColorKey | undefined {
  const { data } = useQuery({
    queryKey: ["people"] as const,
    staleTime: 60_000,
    queryFn: async () => (await get<{ people: PersonColor[] }>("/people")).people,
    select: (people) => (id ? resolveColorKey(id, people.find((p) => p.id === id)?.color) : undefined),
  });
  return id ? (data ?? autoColorKey(id)) : undefined;
}
