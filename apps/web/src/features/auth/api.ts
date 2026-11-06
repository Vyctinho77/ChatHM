import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, get, post } from "@/lib/api";
import type { Theme } from "@/lib/theme";

export type Role = "admin" | "agent";

export type Me = {
  user: { id: string; email: string; name: string; role: Role; theme: Theme; mfaEnabled: boolean };
  csrfToken: string;
  mfaRequired: boolean;
  mfaSetupRequired: boolean;
};

export const meKey = ["auth", "me"] as const;

/** Sessão atual; `null` quando não autenticado. */
export function useMe() {
  return useQuery({
    queryKey: meKey,
    queryFn: async () => {
      try {
        return await get<Me>("/auth/me");
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    staleTime: 5 * 60_000,
    retry: false,
  });
}

export function useSetMe() {
  const qc = useQueryClient();
  return (me: Me | null) => qc.setQueryData(meKey, me);
}

export const login = (email: string, password: string) => post<Me>("/auth/login", { email, password });
/** Cadastro por conta própria: a conta nasce inativa e o administrador precisa aprová-la. */
export const register = (name: string, email: string, password: string) =>
  post<{ pending: true }>("/auth/register", { name, email, password });

/** Opções públicas da tela de entrada (o cadastro próprio vem desligado por padrão). */
export function useAuthOptions() {
  return useQuery({
    queryKey: ["auth", "options"] as const,
    queryFn: () => get<{ selfRegister: boolean }>("/auth/options"),
    staleTime: 5 * 60_000,
    retry: false,
  });
}
export const verifyMfa = (code: string) => post<Me>("/auth/mfa/verify", { code });
export const setupMfa = () => post<{ secret: string; otpauthUrl: string }>("/auth/mfa/setup");
export const enableMfa = (code: string) => post<Me>("/auth/mfa/enable", { code });
export const logout = () => post<void>("/auth/logout");
