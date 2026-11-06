import type { ReactNode } from "react";
import { Logo } from "@/components/Logo";
import { ThemeSwitcher } from "@/features/shell/ThemeSwitcher";
import brandMark from "@/assets/brand/mark.svg";

export function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <div className="flex min-h-full flex-col bg-auth-bg">
      <header className="flex items-center justify-between px-5 py-5 sm:px-8">
        <Logo />
        <ThemeSwitcher />
      </header>
      <main className="flex flex-1 flex-col items-center justify-center px-5 pb-10 pt-6 sm:pb-16">
        <section className="auth-card w-full max-w-[520px] rounded-[24px] border border-line/70 bg-panel p-7 shadow-[0_12px_48px_-24px_rgba(0,0,0,0.18)] sm:p-9" aria-labelledby="auth-title">
          <img src={brandMark} alt="" width={40} height={41} className="mb-5 h-10 w-10" aria-hidden="true" />
          <h1 id="auth-title" className="text-[23px] font-semibold leading-tight tracking-tight">{title}</h1>
          {subtitle && <p className="mt-2 text-[13px] leading-relaxed text-muted">{subtitle}</p>}
          <div className="mt-7">{children}</div>
        </section>
        <p className="mt-6 text-center text-[11px] tracking-wide text-muted">ChatHM · Central de atendimento</p>
      </main>
    </div>
  );
}
