import { useQuery } from "@tanstack/react-query";
import { get } from "@/lib/api";
import { DEMO } from "@/lib/env";

/**
 * Quem da equipe está online agora: a pessoa tem o ChatHM aberto em alguma aba (conexão em tempo real). O estado inicial vem de GET /presence e as
 * mudanças chegam pelo evento "presence" (useRealtime). Ao sair, o servidor espera alguns segundos antes de avisar, para um F5 não piscar "offline".
 */
export type Presence = { online: string[] };
export const presenceKey = ["presence"] as const;

/** Entrou ou saiu alguém: a lista nova (sem repetir, em ordem estável). */
export function applyPresence(old: Presence | undefined, ev: { userId: string; online: boolean }): Presence {
  const rest = (old?.online ?? []).filter((id) => id !== ev.userId);
  return { online: ev.online ? [...rest, ev.userId].sort() : rest };
}

/** O conjunto de pessoas online. Vazio enquanto carrega (ninguém aparece como online até saber). */
export function useOnline(): ReadonlySet<string> {
  const q = useQuery({
    queryKey: presenceKey,
    enabled: !DEMO,
    staleTime: 30_000,
    queryFn: () => get<Presence>("/presence"),
  });
  return new Set(q.data?.online ?? []);
}
