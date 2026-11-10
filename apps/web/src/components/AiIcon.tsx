import aiIcon from "@/assets/icons/ai.svg";
import { cn } from "@/lib/cn";

/** Vetor da identidade de IA; acompanha a cor do texto em qualquer tema. */
export function AiIcon({ className }: { className?: string }) {
  return <span aria-hidden="true" className={cn("inline-block shrink-0 bg-current [mask-position:center] [mask-repeat:no-repeat] [mask-size:contain]", className)} style={{ maskImage: `url("${aiIcon}")`, WebkitMaskImage: `url("${aiIcon}")` }} />;
}
