import { cn } from "@/lib/cn";

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn("text-2xl font-bold tracking-tight", className)}>
      <span className="text-brand">Chat</span>
      <span className="text-brand/80">HM</span>
    </span>
  );
}
