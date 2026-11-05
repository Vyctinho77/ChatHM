import { Bell, ChevronDown, LogOut, Menu as MenuIcon, Settings } from "lucide-react";
import { useNavigate } from "react-router";
import { Avatar } from "@/components/Avatar";
import { Logo } from "@/components/Logo";
import { Menu, MenuItem } from "@/components/Menu";
import { IconButton } from "@/components/ui";
import { logout, useSetMe, type Me } from "@/features/auth/api";
import { ThemeSwitcher } from "./ThemeSwitcher";

const roleLabel = { admin: "Administrador", agent: "Atendente" } as const;

export function TopBar({ me, onOpenMenu }: { me: Me; onOpenMenu?: () => void }) {
  const navigate = useNavigate();
  const setMe = useSetMe();

  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-app px-3 sm:px-4">
      {onOpenMenu && (
        <IconButton label="Abrir menu" onClick={onOpenMenu} className="-ml-1">
          <MenuIcon className="size-5" />
        </IconButton>
      )}
      <Logo />
      <div className="ml-auto flex items-center gap-2 sm:gap-3">
        <ThemeSwitcher className="hidden sm:flex" />
        <IconButton label="Notificações" className="relative">
          <Bell className="size-[18px]" />
        </IconButton>
        <Menu
          trigger={({ toggle, open }) => (
            <button
              type="button"
              onClick={toggle}
              aria-expanded={open}
              aria-haspopup="menu"
              className="flex items-center gap-2 rounded-lg py-1 pl-1 pr-2 hover:bg-hover"
            >
              <Avatar name={me.user.name} size="sm" className="bg-blue-600 text-white" />
              <span className="hidden max-w-40 truncate text-[13px] font-medium sm:block">{me.user.name}</span>
              <ChevronDown className="size-4 text-muted" />
            </button>
          )}
        >
          {(close) => (
            <>
              <div className="border-b border-line px-3 py-2">
                <p className="truncate text-[13px] font-medium">{me.user.name}</p>
                <p className="truncate text-xs text-muted">
                  {me.user.email} · {roleLabel[me.user.role]}
                </p>
              </div>
              <div className="px-3 py-2 sm:hidden">
                <ThemeSwitcher />
              </div>
              <MenuItem
                icon={<Settings />}
                onClick={() => {
                  close();
                  navigate("/configuracoes");
                }}
              >
                Configurações
              </MenuItem>
              <MenuItem
                icon={<LogOut />}
                danger
                onClick={async () => {
                  close();
                  await logout().catch(() => undefined);
                  setMe(null);
                }}
              >
                Sair
              </MenuItem>
            </>
          )}
        </Menu>
      </div>
    </header>
  );
}
