import { Menu as MenuIcon } from "lucide-react";
import { Logo } from "@/components/Logo";
import { IconButton } from "@/components/ui";
import { ThemeSwitcher } from "./ThemeSwitcher";

export function TopBar({ onOpenMenu }: { onOpenMenu?: () => void }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-app px-3 sm:px-4">
      {onOpenMenu && (
        <IconButton label="Abrir menu" onClick={onOpenMenu} className="-ml-1">
          <MenuIcon className="size-5" />
        </IconButton>
      )}
      <Logo />
      <ThemeSwitcher className="ml-auto" />
    </header>
  );
}
