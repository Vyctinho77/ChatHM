import { Monitor, Moon, Sun } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { patch } from "@/lib/api";
import { cn } from "@/lib/cn";
import type { Theme } from "@/lib/theme";
import { meKey, type Me } from "@/features/auth/api";
import { setTheme, useTheme } from "./useTheme";

const options: { value: Theme; label: string; icon: typeof Sun }[] = [
  { value: "light", label: "Tema claro", icon: Sun },
  { value: "system", label: "Seguir o sistema", icon: Monitor },
  { value: "dark", label: "Tema escuro", icon: Moon },
];

export function ThemeSwitcher({ className }: { className?: string }) {
  const theme = useTheme();
  const qc = useQueryClient();

  function choose(t: Theme) {
    setTheme(t);
    const me = qc.getQueryData<Me | null>(meKey);
    if (me && !me.mfaRequired) {
      qc.setQueryData<Me>(meKey, { ...me, user: { ...me.user, theme: t } });
      patch("/me/preferences", { theme: t }).catch(() => undefined);
    }
  }

  return (
    <div role="radiogroup" aria-label="Tema" className={cn("flex rounded-lg bg-elevated p-0.5", className)}>
      {options.map(({ value, label, icon: Icon }) => (
        <button
          key={value}
          type="button"
          role="radio"
          aria-checked={theme === value}
          aria-label={label}
          title={label}
          onClick={() => choose(value)}
          className={cn(
            "inline-flex size-8 items-center justify-center rounded-md text-muted transition-colors hover:text-fg",
            theme === value && "bg-hover text-fg shadow-sm",
          )}
        >
          <Icon className="size-4" />
        </button>
      ))}
    </div>
  );
}
