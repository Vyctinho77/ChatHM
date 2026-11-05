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
export const verifyMfa = (code: string) => post<Me>("/auth/mfa/verify", { code });
export const setupMfa = () => post<{ secret: string; otpauthUrl: string }>("/auth/mfa/setup");
export const enableMfa = (code: string) => post<Me>("/auth/mfa/enable", { code });
export const logout = () => post<void>("/auth/logout");
