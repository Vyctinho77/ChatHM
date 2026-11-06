import { useEffect } from "react";
import { Outlet } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { AppSkeleton } from "@/components/Skeleton";
import { Alert, Button } from "@/components/ui";
import { setUnauthorizedHandler } from "@/lib/api";
import { useSystemThemeSync } from "@/lib/theme";
import { meKey, useMe } from "@/features/auth/api";
import { LoginPage } from "@/features/auth/LoginPage";
import { MfaSetupPage, MfaVerifyPage } from "@/features/auth/MfaPages";
import { AppShell } from "@/features/shell/AppShell";
import { setTheme, useTheme } from "@/features/shell/useTheme";

/** Decide entre login, MFA e aplicação conforme o estado da sessão. */
export function Root() {
  const qc = useQueryClient();
  const me = useMe();
  const theme = useTheme();
  useSystemThemeSync(theme);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      qc.setQueryData(meKey, null);
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== meKey[0] });
    });
  }, [qc]);

  // Preferência salva no usuário prevalece ao entrar.
  const serverTheme = me.data?.user.theme;
  useEffect(() => {
    if (serverTheme) setTheme(serverTheme);
  }, [serverTheme]);

  if (me.isPending) {
    return <AppSkeleton />;
  }
  if (me.isError) {
    return (
      <div className="mx-auto flex h-full max-w-sm flex-col items-center justify-center gap-4 p-6">
        <Alert>Não foi possível conectar ao servidor.</Alert>
        <Button onClick={() => me.refetch()}>Tentar novamente</Button>
      </div>
    );
  }
  if (!me.data) return <LoginPage />;
  if (me.data.mfaRequired) return <MfaVerifyPage />;
  if (me.data.mfaSetupRequired) return <MfaSetupPage />;
  return <AppShell me={me.data} />;
}

export function AdminOnly() {
  const { data } = useMe();
  if (data?.user.role !== "admin") {
    return <p className="p-8 text-center text-sm text-muted">Acesso restrito a administradores.</p>;
  }
  return <Outlet />;
}
