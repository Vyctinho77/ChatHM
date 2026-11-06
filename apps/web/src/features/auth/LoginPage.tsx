import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { login, loginWithCode, useAuthOptions, useSetMe } from "./api";
import { AuthLayout } from "./AuthLayout";
import { CodeInput } from "./CodeInput";
import { InvitePage, JoinPage, SetupPage } from "./OnboardingPages";
import { setHandover, useHandover } from "./handover";
import { HandoverDone } from "./HandoverDone";

type View = "login" | "join" | "personal";

/**
 * Entrada. Sem administrador (instalação nova): QR do WhatsApp do escritório. Com administrador:
 * e-mail + código do autenticador (a senha é só uma alternativa para contas antigas) e "Sou da equipe".
 */
export function LoginPage() {
  const setMe = useSetMe();
  const options = useAuthOptions().data;
  const handover = useHandover();
  const [view, setView] = useState<View>("login");
  const [email, setEmail] = useState(DEMO ? "admin@hm.com.br" : "");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState(DEMO ? "demo" : "");
  const [byPassword, setByPassword] = useState(false);
  const [recovery, setRecovery] = useState(false);
  const [error, setError] = useState("");
  const [fields, setFields] = useState<FieldErrors>({});
  const [loading, setLoading] = useState(false);

  if (handover) return <HandoverDone result={handover} onClose={() => setHandover(null)} />;
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
    <AuthLayout title="Bem-vindo ao ChatHM" subtitle={byPassword ? "Acesse sua conta com e-mail e senha." : "Use seu e-mail e o código do autenticador para entrar."}>
      <form onSubmit={onSubmit} className="flex flex-col gap-5" aria-busy={loading}>
        {DEMO && (
          <Alert tone="info">
            Demonstração: qualquer código de 6 dígitos serve (ou “Entrar com senha”: <code>demo</code>).
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        <Field
          label="E-mail"
          type="email"
          placeholder="voce@escritorio.com.br"
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
          <CodeInput value={code} onChange={setCode} error={fields.code} autoFocus={false} disabled={loading} />
        )}
        <Button type="submit" loading={loading} className="h-11 w-full rounded-[10px] text-sm font-semibold" disabled={!ready}>
          Entrar
        </Button>
        <div className="flex flex-wrap items-center justify-center gap-x-5 gap-y-2 text-xs text-muted">
          {!byPassword && (
            <button type="button" disabled={loading} className="underline-offset-4 transition-colors hover:text-fg hover:underline" onClick={() => (setRecovery(!recovery), setCode(""), setError(""), setFields({}))}>
              {recovery ? "Usar o código do app" : "Perdi o celular"}
            </button>
          )}
          <button type="button" disabled={loading} className="underline-offset-4 transition-colors hover:text-fg hover:underline" onClick={() => (setByPassword(!byPassword), setRecovery(false), setError(""), setFields({}), setCode(""))}>
            {byPassword ? "Entrar com o código do app" : "Entrar com senha"}
          </button>
        </div>
      </form>
      <div className="mt-6 border-t border-line pt-5">
        <p className="mb-3 text-center text-xs text-muted">Primeiro acesso ao escritório?</p>
        <Button type="button" variant="secondary" className="h-11 w-full rounded-[10px] text-sm" disabled={loading} onClick={() => setView("join")}>
          Sou da equipe
        </Button>
      </div>
    </AuthLayout>
  );
}
