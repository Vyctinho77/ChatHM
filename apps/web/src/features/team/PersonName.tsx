import { cn } from "@/lib/cn";
import { colorInkVar, colorVar, usePersonColorKey } from "./personColor";

/**
 * O nome de uma pessoa da equipe na cor dela. Só o texto colorido: sem balão, sem fundo.
 * `ink`: use sobre o balão recebido (que é claro nos dois temas).
 */
export function PersonName({ id, children, ink = false, className }: { id: string | undefined; children: React.ReactNode; ink?: boolean; className?: string }) {
  const key = usePersonColorKey(id);
  return (
    <span className={cn("font-semibold", className)} style={key ? { color: (ink ? colorInkVar : colorVar)(key) } : undefined}>
      {children}
    </span>
  );
}
