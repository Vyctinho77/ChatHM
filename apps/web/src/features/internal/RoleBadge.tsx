import { cn } from "@/lib/cn";
import type { Role } from "@/features/auth/api";
import { roleLabel } from "./roles";
import { VerifiedBadge } from "./VerifiedBadge";

// O balão recebido é claro nos dois temas; a lista de membros acompanha o tema.
const onBubble: Record<Exclude<Role, "admin">, string> = {
  manager: "bg-amber-500/20 text-amber-900",
  agent: "bg-zinc-500/15 text-zinc-700",
};
const themed: Record<Exclude<Role, "admin">, string> = {
  manager: "bg-amber-500/15 text-amber-800 dark:text-amber-200",
  agent: "bg-hover text-fg/80",
};

// O administrador aparece como selo de verificado; o nome do perfil fica na dica ao passar o mouse.
const sealColor = { onBubble: "text-[#5f6e24]", themed: "text-brand" };

export function RoleBadge({ role, onBubbleSurface = false, className }: { role: Role; onBubbleSurface?: boolean; className?: string }) {
  if (role === "admin") return <VerifiedBadge label={roleLabel.admin} className={cn(onBubbleSurface ? sealColor.onBubble : sealColor.themed, className)} />;
  return (
    <span className={cn("inline-flex items-center rounded-full px-1.5 py-px text-[10.5px] font-medium leading-4", (onBubbleSurface ? onBubble : themed)[role], className)}>
      {roleLabel[role]}
    </span>
  );
}
