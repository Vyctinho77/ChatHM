import type { Role } from "@/features/auth/api";

export const roleLabel: Record<Role, string> = { admin: "Administrador", manager: "Gestor", agent: "Atendente" };

/** Ordem de apresentação: quem coordena primeiro. */
export const roleOrder: Record<Role, number> = { admin: 0, manager: 1, agent: 2 };

/** Cor do nome de cada pessoa (estável pelo id), legível sobre o balão claro. */
export function authorColor(id: string) {
  let h = 0;
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  const hue = h % 360;
  return `hsl(${hue} 55% 32%)`;
}
