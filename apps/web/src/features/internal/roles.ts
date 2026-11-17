import type { Role } from "@/features/auth/api";

export const roleLabel: Record<Role, string> = { admin: "Administrador", manager: "Gestor", agent: "Atendente" };

/** Ordem de apresentação: quem coordena primeiro. */
export const roleOrder: Record<Role, number> = { admin: 0, manager: 1, agent: 2 };
