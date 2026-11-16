import { cn } from "@/lib/cn";
import type { Role } from "@/features/auth/api";
import { roleLabel } from "./roles";

// O balão recebido é claro nos dois temas; a lista de membros acompanha o tema.
const onBubble: Record<Role, string> = {
  admin: "bg-[#5f6e24]/15 text-[#46521a]",
  manager: "bg-amber-500/20 text-amber-900",
  agent: "bg-zinc-500/15 text-zinc-700",
};
const themed: Record<Role, string> = {
  admin: "bg-brand/15 text-brand-ink",
  manager: "bg-amber-500/15 text-amber-800 dark:text-amber-200",
  agent: "bg-hover text-fg/80",
};

export function RoleBadge({ role, onBubbleSurface = false, className }: { role: Role; onBubbleSurface?: boolean; className?: string }) {
  return (
    <span className={cn("inline-flex items-center rounded-full px-1.5 py-px text-[10.5px] font-medium leading-4", (onBubbleSurface ? onBubble : themed)[role], className)}>
      {roleLabel[role]}
    </span>
  );
}
