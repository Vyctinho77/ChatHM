import { cn } from "@/lib/cn";
import type { Role } from "@/features/auth/api";
import { roleLabel } from "./roles";
import { VerifiedBadge } from "./VerifiedBadge";

// Etiqueta discreta: ponto colorido + nome do perfil, com contorno fino. O balão recebido é claro nos dois temas; a lista de membros acompanha o tema.
type Tone = { chip: string; dot: string };
const onBubble: Record<Exclude<Role, "admin">, Tone> = {
  manager: { chip: "bg-amber-500/10 text-amber-900 ring-amber-600/25", dot: "bg-amber-500" },
  agent: { chip: "bg-black/[0.04] text-black/60 ring-black/10", dot: "bg-sky-500" },
};
const themed: Record<Exclude<Role, "admin">, Tone> = {
  manager: { chip: "bg-amber-500/10 text-amber-800 ring-amber-600/25 dark:text-amber-200 dark:ring-amber-300/25", dot: "bg-amber-500" },
  agent: { chip: "bg-hover text-fg/75 ring-line", dot: "bg-sky-500" },
};

// O administrador aparece como selo de verificado; o nome do perfil fica na dica ao passar o mouse.
const sealColor = { onBubble: "text-[#5f6e24]", themed: "text-brand" };

export function RoleBadge({ role, onBubbleSurface = false, className }: { role: Role; onBubbleSurface?: boolean; className?: string }) {
  if (role === "admin") return <VerifiedBadge label={roleLabel.admin} className={cn(onBubbleSurface ? sealColor.onBubble : sealColor.themed, className)} />;
  const tone = (onBubbleSurface ? onBubble : themed)[role];
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-md px-2 py-[3px] text-[11px] font-medium leading-4 ring-1 ring-inset", tone.chip, className)}>
      <span className={cn("size-1.5 rounded-full", tone.dot)} aria-hidden="true" />
      {roleLabel[role]}
    </span>
  );
}
