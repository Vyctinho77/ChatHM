import { useEffect, useState, type FormEvent } from "react";
import QRCode from "react-qr-code";
import { Alert, Button, Spinner } from "@/components/ui";
import { ApiError, errorMessage } from "@/lib/api";
import { enableMfa, logout, setupMfa, useSetMe, verifyMfa } from "./api";
import { AuthLayout } from "./AuthLayout";
import { CodeInput } from "./CodeInput";

function useCodeSubmit(action: (code: string) => Promise<unknown>) {
  const setMe = useSetMe();
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (code.length !== 6) return setError("Informe os 6 dígitos.");
    setError("");
    setLoading(true);
    try {
      await action(code);
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setMe(null); // sessão encerrada pelo servidor
        return;
      }
      setError(errorMessage(err));
      setCode("");
    } finally {
      setLoading(false);
    }
  }
  return { code, setCode, error, loading, submit };
}

function CancelLink() {
  const setMe = useSetMe();
  return (
    <button
      type="button"
      className="text-sm text-muted hover:text-fg"
      onClick={async () => {
        await logout().catch(() => undefined);
        setMe(null);
      }}
    >
      Entrar com outra conta
    </button>
  );
}

export function MfaVerifyPage() {
  const setMe = useSetMe();
  const f = useCodeSubmit(async (code) => setMe(await verifyMfa(code)));
  return (
    <AuthLayout title="Verificação em duas etapas" subtitle="Abra o app autenticador e digite o código atual.">
      <form onSubmit={f.submit} className="flex flex-col gap-4">
        <CodeInput value={f.code} onChange={f.setCode} error={f.error} />
        <Button type="submit" loading={f.loading} className="h-10">
          Verificar
        </Button>
        <CancelLink />
      </form>
    </AuthLayout>
  );
}

export function MfaSetupPage() {
  const setMe = useSetMe();
  const [setup, setSetup] = useState<{ secret: string; otpauthUrl: string } | null>(null);
  const [loadError, setLoadError] = useState("");
  const f = useCodeSubmit(async (code) => setMe(await enableMfa(code)));

  useEffect(() => {
    setupMfa().then(setSetup, (e) => setLoadError(errorMessage(e)));
  }, []);

  return (
    <AuthLayout
      title="Ative a verificação em duas etapas"
      subtitle="Obrigatória para administradores. Escaneie o QR Code com Google Authenticator, Microsoft Authenticator ou similar."
    >
      {loadError && <Alert>{loadError}</Alert>}
      {!setup && !loadError && <Spinner className="mx-auto" />}
      {setup && (
        <form onSubmit={f.submit} className="flex flex-col gap-4">
          <div className="mx-auto rounded-xl bg-white p-3">
            <QRCode value={setup.otpauthUrl} size={176} />
          </div>
          <details className="text-sm text-muted">
            <summary className="cursor-pointer">Não consegue escanear?</summary>
            <p className="mt-2">Digite esta chave no aplicativo:</p>
            <code className="mt-1 block break-all font-mono text-xs text-fg">
              {setup.secret}
            </code>
          </details>
          <CodeInput value={f.code} onChange={f.setCode} error={f.error} />
          <Button type="submit" loading={f.loading} className="h-10">
            Ativar
          </Button>
          <CancelLink />
        </form>
      )}
    </AuthLayout>
  );
}
