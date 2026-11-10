import { CalendarDays, Contact, Layers, MessageSquareText, Plug, Settings, Tag, UsersRound } from "lucide-react";
import type { ComponentType } from "react";
import { WhatsAppIcon } from "@/components/WhatsAppIcon";
import { AiIcon } from "@/components/AiIcon";
import type { Role } from "@/features/auth/api";

export type NavItem = { to: string; label: string; icon: ComponentType<{ className?: string }>; roles?: Role[] };

export const mainNav: NavItem[] = [
  { to: "/conversas", label: "Conversas", icon: MessageSquareText },
  { to: "/filas", label: "Filas / Segmentos", icon: Layers },
  { to: "/clientes", label: "Clientes", icon: Contact },
  { to: "/agenda", label: "Agenda", icon: CalendarDays },
  { to: "/etiquetas", label: "Etiquetas", icon: Tag },
];

export const secondaryNav: NavItem[] = [
  { to: "/equipe", label: "Equipe", icon: UsersRound },
  { to: "/integracoes/mcp", label: "MCP", icon: Plug },
  { to: "/automacao", label: "Envios automáticos", icon: AiIcon, roles: ["admin"] },
  { to: "/whatsapp", label: "Conectar", icon: WhatsAppIcon, roles: ["admin"] },
];

export const settingsNav: NavItem = { to: "/configuracoes", label: "Configurações", icon: Settings };
