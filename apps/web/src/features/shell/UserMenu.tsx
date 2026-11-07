import { ChevronsUpDown, LogOut } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { logout, useSetMe, type Me } from "@/features/auth/api";
import { ThemeSwitcher } from "./ThemeSwitcher";

const roleLabel = { admin: "Administrador", agent: "Atendente" } as const;

/** Perfil do usuário no rodapé do menu lateral; o menu abre para cima. */
export function UserMenu({ me }: { me: Me }) {
  const setMe = useSetMe();
  return (
    <Menu
      align="left"
      placement="top"
      rootClassName="w-full"
      className="w-full min-w-0"
      trigger={({ toggle, open }) => (
        <button
          type="button"
          onClick={toggle}
          aria-expanded={open}
          aria-haspopup="menu"
          className="flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-hover"
        >
          <Avatar name={me.user.name} size="sm" className="size-8 bg-selected text-brand-ink" />
          <span className="flex min-w-0 flex-1 flex-col gap-1">
            <span className="block truncate text-[13px] font-semibold leading-4">{me.user.name}</span>
            <span className="block truncate text-[11px] leading-4 text-muted">{roleLabel[me.user.role]}</span>
          </span>
          <ChevronsUpDown className="ml-1 size-3.5 shrink-0 text-muted/70" />
        </button>
      )}
    >
      {(close) => (
        <>
          <div className="border-b border-line px-3 py-2">
            <p className="truncate text-[13px] font-medium">{me.user.name}</p>
            <p className="truncate text-[11.5px] text-muted">{me.user.email}</p>
          </div>
          <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-3">
            <span className="text-xs text-muted">Tema</span>
            <ThemeSwitcher />
          </div>
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
  );
}
