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

/** Entra só com o código do app autenticador (ou um código de recuperação). */
export const loginWithCode = (email: string, code: string) => post<Me>("/auth/login", { email, code });

export type Enrollment = { secret: string; otpauthUrl: string; token: string; name?: string };
export type MeWithCodes = Me & { recoveryCodes: string[] };

export const setupStart = (name: string, email: string, installCode?: string) =>
  post<Enrollment>("/setup/start", { name, email, installCode: installCode ?? "" });
export const setupFinish = (token: string, code: string) => post<MeWithCodes>("/setup/finish", { token, code });
export const enrollStart = (email: string, inviteCode: string) => post<Enrollment>("/auth/enroll/start", { email, inviteCode });
export const enrollFinish = (email: string, inviteCode: string, token: string, code: string) =>
  post<MeWithCodes>("/auth/enroll/finish", { email, inviteCode, token, code });
/** Instalação inicial: pareamento do WhatsApp do escritório pela tela de entrada (só sem administrador). */
export type SetupConnection = {
  state: "disconnected" | "pending_qr" | "connected" | "error";
  phone: string;
  displayName: string;
  lastError?: string;
  qr?: { code: string; expiresAt: string } | null;
};
export const setupWhatsAppConnect = (installCode?: string) =>
  post<{ key: string }>("/setup/whatsapp/connect", { installCode: installCode ?? "" });
export const setupWhatsAppStatus = (key: string, installCode?: string) =>
  post<SetupConnection>("/setup/whatsapp/status", { key, installCode: installCode ?? "" });

/** Equipe: cria a conta com o token do escritório (e configura o autenticador). */
export const joinStart = (name: string, email: string, officeToken: string) =>
  post<Enrollment>("/auth/join/start", { name, email, officeToken });
export const joinFinish = (officeToken: string, token: string, code: string) =>
  post<MeWithCodes>("/auth/join/finish", { officeToken, token, code });

export const regenerateRecoveryCodes = () => post<{ recoveryCodes: string[] }>("/auth/recovery-codes");

/** Opções públicas da tela de entrada. */
export function useAuthOptions() {
  return useQuery({
    queryKey: ["auth", "options"] as const,
    queryFn: () => get<{ selfRegister: boolean; needsSetup: boolean; setupCode: boolean }>("/auth/options"),
    refetchOnWindowFocus: true,
    staleTime: 5 * 60_000,
    retry: false,
  });
}
export const verifyMfa = (code: string) => post<Me>("/auth/mfa/verify", { code });
export const setupMfa = () => post<{ secret: string; otpauthUrl: string }>("/auth/mfa/setup");
export const enableMfa = (code: string) => post<Me>("/auth/mfa/enable", { code });
export const logout = () => post<void>("/auth/logout");
