import { cn } from "@/lib/cn";
import type { Role } from "@/features/auth/api";
import { roleLabel } from "./roles";
import { VerifiedBadge } from "./VerifiedBadge";

// Todo perfil tem seu selo de verificado; a cor diz quem é: atendente azul, gestor verde da marca, administrador dourado.
// O balão recebido é claro nos dois temas; a lista de membros acompanha o tema.
const onBubble: Record<Role, string> = {
  admin: "text-[#c8960a]",
  manager: "text-[#5f6e24]",
  agent: "text-[#1d8fe0]",
};
const themed: Record<Role, string> = {
  admin: "text-[#e0aa1c]",
  manager: "text-brand",
  agent: "text-[#3b9cf0]",
};

/** O nome do perfil fica na dica ao passar o mouse. */
export function RoleBadge({ role, onBubbleSurface = false, className }: { role: Role; onBubbleSurface?: boolean; className?: string }) {
  return <VerifiedBadge label={roleLabel[role]} className={cn((onBubbleSurface ? onBubble : themed)[role], className)} />;
}
