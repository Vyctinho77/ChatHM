import { cn } from "@/lib/cn";
import { initials } from "@/lib/format";

const palette = [
  "bg-selected text-fg",
  "bg-elevated text-fg",
  "bg-hover text-fg",
  "bg-fg/10 text-fg",
];

function colorFor(seed: string) {
  let h = 0;
  for (const ch of seed) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return palette[h % palette.length];
}

const sizes = {
  sm: "size-7 text-[10.5px]",
  list: "size-9 text-xs", // lista de conversas
  md: "size-10 text-[13px]",
  lg: "size-20 text-2xl",
} as const;

export function Avatar({
  name,
  src,
  size = "md",
  className,
}: {
  name: string;
  src?: string;
  size?: keyof typeof sizes;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 select-none items-center justify-center overflow-hidden rounded-full font-semibold",
        sizes[size],
        !src && colorFor(name),
        className,
      )}
      aria-hidden
    >
      {src ? <img src={src} alt="" className="size-full object-cover" /> : initials(name)}
    </span>
  );
}
