import { NavLink } from "react-router";
import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";
import type { Me } from "@/features/auth/api";
import { useConversations } from "@/features/conversations/store";
import { stateDot, stateLabel, useConnection } from "@/features/whatsapp/api";
import { mainNav, secondaryNav, settingsNav, type NavItem } from "./nav";
import { UserMenu } from "./UserMenu";

function Item({ item, extra, onNavigate }: { item: NavItem; extra?: React.ReactNode; onNavigate?: () => void }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      onClick={onNavigate}
      className={({ isActive }) =>
        cn(
          "flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13px] transition-colors",
          isActive ? "bg-accent/15 font-medium text-fg ring-1 ring-accent/30" : "text-fg/85 hover:bg-hover",
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon className={cn("size-[18px] shrink-0", isActive ? "text-accent" : "text-muted")} />
          <span className="truncate">{item.label}</span>
          {extra && <span className="ml-auto">{extra}</span>}
        </>
      )}
    </NavLink>
  );
}

export function Sidebar({ me, onNavigate }: { me: Me; onNavigate?: () => void }) {
  const role = me.user.role;
  const { data } = useConversations();
  const { data: wa } = useConnection();
  const unread = data?.filter((c) => c.unreadCount > 0).length ?? 0;
  const allowed = (i: NavItem) => !i.roles || i.roles.includes(role);

  return (
    <nav aria-label="Menu principal" className="flex h-full flex-col bg-panel px-2.5 py-3">
      <div className="flex flex-col gap-0.5">
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
      <div className="flex flex-col gap-0.5">
        {secondaryNav.filter(allowed).map((item) => (
          <Item
            key={item.to}
            item={item}
            onNavigate={onNavigate}
            extra={
              item.to === "/whatsapp" ? (
                <span
                  className={cn("block size-2 rounded-full", stateDot[wa?.state ?? "disconnected"])}
                  title={stateLabel[wa?.state ?? "disconnected"]}
                />
              ) : undefined
            }
          />
        ))}
      </div>
      <div className="mt-auto flex flex-col gap-1 border-t border-line pt-3">
        <Item item={settingsNav} onNavigate={onNavigate} />
        <UserMenu me={me} />
      </div>
    </nav>
  );
}
