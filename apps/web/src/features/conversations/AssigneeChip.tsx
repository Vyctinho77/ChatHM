import { UserRound } from "lucide-react";
import { cn } from "@/lib/cn";
import { colorVar, usePersonColorKey } from "@/features/team/personColor";

/**
 * Quem está atendendo, ao lado do nome do cliente: "Você" para o próprio usuário, o primeiro nome para os demais.
 * Só o texto, na cor da pessoa (definida em Equipe): sem balão.
 */
export function AssigneeChip({ id, name, mine, className }: { id: string; name: string; mine: boolean; className?: string }) {
  const label = mine ? "Você" : name.trim().split(/\s+/)[0] || name;
  const key = usePersonColorKey(id);
  return (
    <span
      title={mine ? "Você está atendendo" : `Atendendo: ${name}`}
      className={cn("inline-flex min-w-0 max-w-[40%] shrink items-center gap-0.5 text-[11px] font-semibold leading-4", className)}
      style={key ? { color: colorVar(key) } : undefined}
    >
      <UserRound aria-hidden className="size-3 shrink-0" />
      <span className="sr-only">Atendendo: </span>
      <span className="truncate">{label}</span>
    </span>
  );
}
