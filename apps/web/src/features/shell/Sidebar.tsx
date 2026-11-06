import { NavLink } from "react-router";
import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";
import type { Me } from "@/features/auth/api";
import { useConversations } from "@/features/conversations/store";
import { stateLabel, useConnection } from "@/features/whatsapp/api";
import { mainNav, secondaryNav, settingsNav, type NavItem } from "./nav";
import { UserMenu } from "./UserMenu";

function Item({ item, extra, onNavigate, iconClassName, description, className }: { item: NavItem; extra?: React.ReactNode; onNavigate?: () => void; iconClassName?: string; description?: string; className?: string }) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      onClick={onNavigate}
      aria-label={description}
      title={description}
      className={({ isActive }) =>
        cn(
          "flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13px] transition-colors",
          isActive ? iconClassName ? "bg-selected font-medium text-fg" : "bg-accent font-medium text-accent-fg" : "text-fg/85 hover:bg-hover",
          className,
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon className={cn("size-[18px] shrink-0", iconClassName ?? (isActive ? "text-accent-fg" : "text-muted"))} />
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
  const unread = data?.filter((c) => c.unreadCount > 0 && !c.snoozedUntil).length ?? 0;
  const allowed = (i: NavItem) => !i.roles || i.roles.includes(role);
  const whatsappConnected = wa?.state === "connected";

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
                <Badge className="bg-unread text-accent-fg">{unread}</Badge>
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
            item={item.to === "/whatsapp" ? { ...item, label: whatsappConnected ? "Conectado" : "Conectar" } : item}
            onNavigate={onNavigate}
            iconClassName={item.to === "/whatsapp" ? whatsappConnected ? "text-brand-ink" : "text-danger" : undefined}
            description={item.to === "/whatsapp" ? `WhatsApp: ${stateLabel[wa?.state ?? "disconnected"]}` : undefined}
          />
        ))}
      </div>
      <div className="mt-auto flex flex-col gap-3 border-t border-line pb-1 pt-4">
        <Item item={settingsNav} onNavigate={onNavigate} className="h-10 gap-3.5 px-3" />
        <UserMenu me={me} />
      </div>
    </nav>
  );
}
