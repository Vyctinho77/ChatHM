import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { login, loginWithCode, useAuthOptions, useSetMe } from "./api";
import { AuthLayout } from "./AuthLayout";
import { CodeInput } from "./CodeInput";
import { InvitePage, JoinPage, SetupPage } from "./OnboardingPages";

type View = "login" | "join" | "personal";

/**
 * Entrada. Sem administrador (instalação nova): QR do WhatsApp do escritório. Com administrador:
 * e-mail + código do autenticador (a senha é só uma alternativa para contas antigas) e "Sou da equipe".
 */
export function LoginPage() {
  const setMe = useSetMe();
  const options = useAuthOptions().data;
  const [view, setView] = useState<View>("login");
  const [email, setEmail] = useState(DEMO ? "admin@hm.com.br" : "");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState(DEMO ? "demo" : "");
  const [byPassword, setByPassword] = useState(false);
  const [recovery, setRecovery] = useState(false);
  const [error, setError] = useState("");
  const [fields, setFields] = useState<FieldErrors>({});
  const [loading, setLoading] = useState(false);

  if (options?.needsSetup) return <SetupPage needsInstallCode={options.setupCode} />;
  if (view === "join") return <JoinPage onBack={() => setView("login")} onPersonalCode={() => setView("personal")} />;
  if (view === "personal") return <InvitePage onBack={() => setView("join")} />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setFields({});
    setLoading(true);
    try {
      setMe(await (byPassword ? login(email, password) : loginWithCode(email, code)));
    } catch (err) {
      setError(errorMessage(err));
      setFields(fieldErrors(err));
      setCode("");
      setPassword("");
    } finally {
      setLoading(false);
    }
  }

  const ready = byPassword ? password.length > 0 : recovery ? code.trim().length >= 10 : code.length === 6;

  return (
    <AuthLayout title="Entrar no ChatHM" subtitle={byPassword ? "E-mail e senha." : "E-mail e código do app autenticador."}>
      <form onSubmit={onSubmit} className="flex flex-col gap-4" aria-busy={loading}>
        {DEMO && (
          <Alert tone="info">
            Demonstração: qualquer código de 6 dígitos serve (ou “Entrar com senha”: <code>demo</code>).
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        <Field
          label="E-mail"
          type="email"
          autoComplete="username"
          autoFocus
          required
          maxLength={254}
          disabled={loading}
          error={fields.email}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        {byPassword ? (
          <Field label="Senha" type="password" autoComplete="current-password" required disabled={loading} value={password} onChange={(e) => setPassword(e.target.value)} />
        ) : recovery ? (
          <Field
            label="Código de recuperação"
            placeholder="XXXXX-XXXXX"
            autoCapitalize="characters"
            className="[&_input]:font-mono [&_input]:uppercase"
            required
            disabled={loading}
            value={code}
            onChange={(e) => setCode(e.target.value)}
          />
        ) : (
          <CodeInput value={code} onChange={setCode} error={fields.code} />
        )}
        <Button type="submit" loading={loading} className="h-10" disabled={!ready}>
          Entrar
        </Button>
        <div className="flex flex-col items-center gap-1.5 text-sm text-muted">
          {!byPassword && (
            <button type="button" className="hover:text-fg" onClick={() => (setRecovery(!recovery), setCode(""))}>
              {recovery ? "Usar o código do app" : "Perdi o celular"}
            </button>
          )}
          <button type="button" className="hover:text-fg" onClick={() => (setByPassword(!byPassword), setRecovery(false), setError(""))}>
            {byPassword ? "Entrar com o código do app" : "Entrar com senha"}
          </button>
        </div>
      </form>
      <div className="mt-5 border-t border-line pt-4">
        <Button type="button" variant="secondary" className="h-10 w-full" disabled={loading} onClick={() => setView("join")}>
          Sou da equipe
        </Button>
      </div>
    </AuthLayout>
  );
}
