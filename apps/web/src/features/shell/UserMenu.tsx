import { ChevronsUpDown, LogOut } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Menu, MenuItem } from "@/components/Menu";
import { logout, useSetMe, type Me } from "@/features/auth/api";

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
          className="flex w-full items-center gap-2.5 rounded-lg p-1.5 text-left transition-colors hover:bg-hover"
        >
          <Avatar name={me.user.name} size="sm" className="bg-blue-600 text-white" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[13px] font-medium leading-tight">{me.user.name}</span>
            <span className="block truncate text-[11px] leading-tight text-muted">{roleLabel[me.user.role]}</span>
          </span>
          <ChevronsUpDown className="size-3.5 shrink-0 text-muted" />
        </button>
      )}
    >
      {(close) => (
        <>
          <div className="border-b border-line px-3 py-2">
            <p className="truncate text-[13px] font-medium">{me.user.name}</p>
            <p className="truncate text-[11.5px] text-muted">{me.user.email}</p>
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
