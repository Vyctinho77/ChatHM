import { Contact, Layers, MessageSquareText, Settings, Smartphone, Tag, UsersRound, type LucideIcon } from "lucide-react";
import type { Role } from "@/features/auth/api";

export type NavItem = { to: string; label: string; icon: LucideIcon; roles?: Role[] };

export const mainNav: NavItem[] = [
  { to: "/conversas", label: "Conversas", icon: MessageSquareText },
  { to: "/filas", label: "Filas / Segmentos", icon: Layers },
  { to: "/clientes", label: "Clientes", icon: Contact },
  { to: "/etiquetas", label: "Etiquetas", icon: Tag },
];

export const secondaryNav: NavItem[] = [
  { to: "/equipe", label: "Equipe", icon: UsersRound },
  { to: "/whatsapp", label: "Conexão WhatsApp", icon: Smartphone, roles: ["admin"] },
];

export const settingsNav: NavItem = { to: "/configuracoes", label: "Configurações", icon: Settings };
