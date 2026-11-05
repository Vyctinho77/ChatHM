import { cn } from "@/lib/cn";
import { initials } from "@/lib/format";

const palette = [
  "bg-sky-200 text-sky-900",
  "bg-blue-600 text-white",
  "bg-amber-100 text-amber-800",
  "bg-emerald-200 text-emerald-900",
  "bg-violet-200 text-violet-900",
  "bg-rose-200 text-rose-900",
  "bg-teal-600 text-white",
  "bg-orange-200 text-orange-900",
];

function colorFor(seed: string) {
  let h = 0;
  for (const ch of seed) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return palette[h % palette.length];
}

const sizes = { sm: "size-7 text-[10.5px]", md: "size-10 text-[13px]", lg: "size-20 text-2xl" } as const;

export function Avatar({
  name,
  src,
  size = "md",
  ring,
  className,
}: {
  name: string;
  src?: string;
  size?: keyof typeof sizes;
  ring?: boolean;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 select-none items-center justify-center overflow-hidden rounded-full font-semibold",
        sizes[size],
        !src && colorFor(name),
        ring && "ring-2 ring-unread ring-offset-2 ring-offset-panel",
        className,
      )}
      aria-hidden
    >
      {src ? <img src={src} alt="" className="size-full object-cover" /> : initials(name)}
    </span>
  );
}
