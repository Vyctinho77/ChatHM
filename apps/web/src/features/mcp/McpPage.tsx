import { DEMO } from "@/lib/env";
import { AiAccessCard } from "./AiAccessCard";
import { McpHelp } from "./McpHelp";
import { useMe } from "@/features/auth/api";

export function McpPage() {
  const { data: me } = useMe();
  return (
    <div className="h-full overflow-y-auto bg-app">
      <header className="flex items-start justify-between gap-4 border-b border-line px-4 py-4 sm:px-6">
        <div>
          <h1 className="text-xl font-semibold">MCP</h1>
          <p className="mt-1 text-sm text-muted">Conecte seus assistentes de IA e gerencie os acessos ao ChatHM.</p>
        </div>
        {me && <McpHelp key={me.user.id} userId={me.user.id} />}
      </header>
      <div className="mx-auto max-w-4xl px-4 py-6 sm:px-8 [&>section]:border-t-0 [&>section]:pt-0">
        {DEMO ? <p className="text-sm text-muted">As integrações de IA não estão disponíveis nesta demonstração.</p> : <AiAccessCard />}
      </div>
    </div>
  );
}
