import { UserRound } from "lucide-react";
import { cn } from "@/lib/cn";

/** Quem está atendendo, ao lado do nome do cliente: "Você" para o próprio usuário, o primeiro nome para os demais. */
export function AssigneeChip({ name, mine, className }: { name: string; mine: boolean; className?: string }) {
  const label = mine ? "Você" : name.trim().split(/\s+/)[0] || name;
  return (
    <span
      title={mine ? "Você está atendendo" : `Atendendo: ${name}`}
      className={cn(
        "inline-flex min-w-0 max-w-[40%] shrink items-center gap-0.5 rounded-full px-1.5 py-px text-[10px] font-semibold leading-4",
        mine ? "bg-brand/15 text-brand-ink" : "bg-hover text-fg/80",
        className,
      )}
    >
      <UserRound aria-hidden className="size-2.5 shrink-0" />
      <span className="sr-only">Atendendo: </span>
      <span className="truncate">{label}</span>
    </span>
  );
}
