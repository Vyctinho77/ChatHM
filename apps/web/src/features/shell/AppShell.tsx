import { safeStorage } from "@/lib/storage";
import { useState } from "react";
import { Outlet } from "react-router";
import { Group, Panel, Separator, useDefaultLayout } from "react-resizable-panels";
import { useMediaQuery } from "@/lib/theme";
import type { Me } from "@/features/auth/api";
import { DEMO } from "@/lib/env";
import { useRealtime } from "@/features/realtime/useRealtime";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";

export const APP_VERSION = "v0.1.0";

export function ResizeHandle() {
  return (
    <Separator className="group relative w-px bg-line outline-none">
      <span className="absolute inset-y-0 -left-1 -right-1 transition-colors group-hover:bg-accent/40 group-data-[separator=active]:bg-accent/60" />
    </Separator>
  );
}

export function AppShell({ me }: { me: Me }) {
  const desktop = useMediaQuery("(min-width: 1024px)");
  const [drawer, setDrawer] = useState(false);
  useRealtime(!DEMO);
  const layout = useDefaultLayout({ id: "chathm-shell", storage: safeStorage });

  return (
    <div className="flex h-full flex-col bg-app">
      <TopBar me={me} onOpenMenu={desktop ? undefined : () => setDrawer(true)} />

      <div className="min-h-0 flex-1">
        {desktop ? (
          <Group
            orientation="horizontal"
            defaultLayout={layout.defaultLayout}
            onLayoutChanged={layout.onLayoutChanged}
          >
            <Panel id="sidebar" defaultSize={220} minSize={180} maxSize={300} groupResizeBehavior="preserve-pixel-size">
              <Sidebar role={me.user.role} />
            </Panel>
            <ResizeHandle />
            <Panel id="main" minSize={480}>
              <Outlet />
            </Panel>
          </Group>
        ) : (
          <>
            <Outlet />
            {drawer && (
              <div className="fixed inset-0 z-40" role="dialog" aria-modal aria-label="Menu">
                <button
                  type="button"
                  aria-label="Fechar menu"
                  className="absolute inset-0 bg-black/50"
                  onClick={() => setDrawer(false)}
                />
                <div className="absolute inset-y-0 left-0 w-72 max-w-[85%] shadow-2xl">
                  <Sidebar role={me.user.role} onNavigate={() => setDrawer(false)} />
                </div>
              </div>
            )}
          </>
        )}
      </div>

      <footer className="hidden h-7 shrink-0 items-center border-t border-line bg-app px-4 text-[11px] text-muted lg:flex">
        ChatHM {APP_VERSION}
        {DEMO && <span className="ml-3">· Demonstração com dados fictícios; nada é salvo ao recarregar.</span>}
      </footer>
    </div>
  );
}
