import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return <span aria-hidden="true" className={cn("block animate-pulse rounded-md bg-fg/10 motion-reduce:animate-none", className)} />;
}

function Loading({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return <div role="status" aria-label={label} aria-busy="true" className={className}>
    <div aria-hidden="true" className="h-full">{children}</div>
  </div>;
}

export function ListSkeleton({ rows = 5, avatar = true, label = "Carregando lista", className }: {
  rows?: number; avatar?: boolean; label?: string; className?: string;
}) {
  return <Loading label={label} className={className}>
    {Array.from({ length: rows }, (_, i) => <div key={i} className="flex min-h-16 items-center gap-3 px-3 py-3">
      {avatar && <Skeleton className="size-9 shrink-0 rounded-full" />}
      <div className="min-w-0 flex-1 space-y-2.5">
        <Skeleton className={i % 2 ? "h-3 w-2/5" : "h-3 w-1/3"} />
        <Skeleton className={i % 2 ? "h-2.5 w-3/5" : "h-2.5 w-3/4"} />
      </div>
      <Skeleton className="h-2.5 w-8 shrink-0" />
    </div>)}
  </Loading>;
}

export function ChatSkeleton() {
  return <Loading label="Carregando mensagens" className="py-4">
    <div className="flex flex-col gap-6">
      <Skeleton className="mx-auto mb-2 h-5 w-14" />
      {[false, true, false, false, true].map((out, i) => <div key={i} className={cn("w-[min(20rem,70%)] space-y-2.5 rounded-xl bg-fg/5 p-3", out && "self-end")}>
        <Skeleton className="h-3 w-4/5" />
        {i % 2 === 0 && <Skeleton className="h-3 w-3/5" />}
        <Skeleton className="ml-auto h-2 w-8" />
      </div>)}
    </div>
  </Loading>;
}

export function DetailSkeleton({ className }: { className?: string }) {
  return <Loading label="Carregando detalhes" className={cn("space-y-6 p-5", className)}>
    <div className="space-y-7">
      <Skeleton className="h-5 w-40" />
      {[0, 1, 2].map((i) => <div key={i} className="space-y-3">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-10 w-full rounded-lg" />
        <Skeleton className="h-2.5 w-3/5" />
      </div>)}
    </div>
  </Loading>;
}

export function CalendarSkeleton({ rows = 5 }: { rows?: number }) {
  return <Loading label="Carregando agenda" className="min-h-72 flex-1">
    <div className="grid h-full grid-cols-7" style={{ gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))` }}>
      {Array.from({ length: rows * 7 }, (_, i) => <div key={i} className="min-h-14 space-y-3 border-b border-r border-line p-2 [&:nth-child(7n)]:border-r-0">
        <Skeleton className="h-3 w-4" />
        {i % 5 === 0 && <Skeleton className="h-4 w-4/5" />}
      </div>)}
    </div>
  </Loading>;
}

export function AppSkeleton() {
  return <Loading label="Carregando aplicativo" className="h-full bg-app">
    <div className="flex h-full">
      <div className="hidden w-56 shrink-0 border-r border-line bg-panel px-3 py-5 lg:block">
        <Skeleton className="mb-8 h-6 w-28" />
        {[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="mb-5 h-5 w-4/5" />)}
      </div>
      <div className="w-full bg-panel md:w-80 md:shrink-0 md:border-r md:border-line">
        <div className="space-y-5 p-4">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-8 w-full rounded-lg" />
        </div>
        <ListSkeleton rows={6} />
      </div>
      <div className="hidden min-w-0 flex-1 bg-chat md:block">
        <div className="flex h-14 items-center gap-3 border-b border-line bg-panel px-4">
          <Skeleton className="size-9 rounded-full" /><Skeleton className="h-3 w-28" />
        </div>
        <div className="p-4"><ChatSkeleton /></div>
      </div>
    </div>
  </Loading>;
}
