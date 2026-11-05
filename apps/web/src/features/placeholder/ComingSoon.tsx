import type { LucideIcon } from "lucide-react";

export function ComingSoon({
  icon: Icon,
  title,
  description,
  phase,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  phase: string;
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 bg-app p-8 text-center">
      <span className="rounded-2xl bg-elevated p-4">
        <Icon className="size-8 text-muted" />
      </span>
      <h1 className="text-lg font-semibold">{title}</h1>
      <p className="max-w-md text-[13px] text-muted">{description}</p>
      <span className="rounded-full border border-line px-3 py-1 text-xs text-muted">{phase}</span>
    </div>
  );
}
