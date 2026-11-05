import { NavLink } from "react-router";
import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";
import type { Role } from "@/features/auth/api";
import { useConversations } from "@/features/conversations/store";
import { mainNav, secondaryNav, settingsNav, type NavItem } from "./nav";

function Item({ item, extra, onNavigate }: { item: NavItem; extra?: React.ReactNode; onNavigate?: () => void }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      onClick={onNavigate}
      className={({ isActive }) =>
        cn(
          "flex h-11 items-center gap-3 rounded-lg px-3 text-[15px] transition-colors",
          isActive ? "bg-accent/15 font-medium text-fg ring-1 ring-accent/30" : "text-fg/85 hover:bg-hover",
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon className={cn("size-5 shrink-0", isActive ? "text-accent" : "text-muted")} />
          <span className="truncate">{item.label}</span>
          {extra && <span className="ml-auto">{extra}</span>}
        </>
      )}
    </NavLink>
  );
}

export function Sidebar({ role, onNavigate }: { role: Role; onNavigate?: () => void }) {
  const { data } = useConversations();
  const unread = data?.filter((c) => c.unreadCount > 0).length ?? 0;
  const allowed = (i: NavItem) => !i.roles || i.roles.includes(role);

  return (
    <nav aria-label="Menu principal" className="flex h-full flex-col bg-panel px-3 py-4">
      <div className="flex flex-col gap-1">
        {mainNav.filter(allowed).map((item) => (
          <Item
            key={item.to}
            item={item}
            onNavigate={onNavigate}
            extra={
              item.to === "/conversas" && unread > 0 ? (
                <Badge className="bg-accent text-accent-fg">{unread}</Badge>
              ) : undefined
            }
          />
        ))}
      </div>
      <hr className="my-3 border-line" />
      <div className="flex flex-col gap-1">
        {secondaryNav.filter(allowed).map((item) => (
          <Item
            key={item.to}
            item={item}
            onNavigate={onNavigate}
            extra={
              item.to === "/whatsapp" ? (
                <span className="block size-2.5 rounded-full bg-muted/60" title="Não conectado" />
              ) : undefined
            }
          />
        ))}
      </div>
      <div className="mt-auto border-t border-line pt-3">
        <Item item={settingsNav} onNavigate={onNavigate} />
      </div>
    </nav>
  );
}
