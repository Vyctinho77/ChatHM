import type { ReactNode } from "react";
import { Logo } from "@/components/Logo";
import { ThemeSwitcher } from "@/features/shell/ThemeSwitcher";

export function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <div className="flex min-h-full flex-col bg-app">
      <header className="flex items-center justify-between px-4 py-3 sm:px-6">
        <Logo />
        <ThemeSwitcher />
      </header>
      <main className="flex flex-1 items-center justify-center px-4 pb-16">
        <div className="w-full max-w-sm rounded-2xl border border-line bg-panel p-6 shadow-sm sm:p-8">
          <h1 className="text-xl font-semibold">{title}</h1>
          {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
          <div className="mt-6">{children}</div>
        </div>
      </main>
    </div>
  );
}
