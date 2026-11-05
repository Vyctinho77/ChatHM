import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

// Placeholder até a Fase 4 (layout em painéis). Apenas verifica a API.
export function App() {
  const health = useQuery({
    queryKey: ["healthz"],
    queryFn: () => api<{ status: string }>("/healthz"),
  });

  return (
    <main className="flex h-full items-center justify-center bg-white text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <div className="text-center">
        <h1 className="text-2xl font-semibold">ChatHM</h1>
        <p className="mt-2 text-sm text-zinc-500">
          API:{" "}
          {health.isPending ? "verificando…" : health.isError ? "indisponível" : health.data.status}
        </p>
      </div>
    </main>
  );
}
