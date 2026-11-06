import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { login, loginWithCode, register, useAuthOptions, useSetMe } from "./api";
import { CodeInput } from "./CodeInput";
import { InvitePage, SetupPage } from "./OnboardingPages";
import { AuthLayout } from "./AuthLayout";

export function LoginPage() {
  const setMe = useSetMe();
  const options = useAuthOptions().data;
  const canRegister = options?.selfRegister === true;
  // Entrada padrão: código do app autenticador (sem senha). A senha fica como alternativa.
  const [byPassword, setByPassword] = useState(false);
  const [useRecovery, setUseRecovery] = useState(false);
  const [code, setCode] = useState("");
  const [inviting, setInviting] = useState(false);
  const [requested, setRequested] = useState(false);
  const [email, setEmail] = useState(DEMO ? "admin@hm.com.br" : "");
  const [password, setPassword] = useState(DEMO ? "demo" : "");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [fields, setFields] = useState<FieldErrors>({});
  const [showPassword, setShowPassword] = useState(false);

  function switchMode() {
    setCreating(!creating);
    setError("");
    setFields({});
    setPassword("");
    setConfirmation("");
    setShowPassword(false);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setFields({});
    if (creating && password !== confirmation) {
      setFields({ confirmation: "As senhas não coincidem." });
      return;
    }
    setLoading(true);
    try {
      if (creating) {
        await register(name, email, password);
        setRequested(true);
      } else if (byPassword) {
        setMe(await login(email, password));
      } else {
        setMe(await loginWithCode(email, code));
      }
    } catch (err) {
      setError(errorMessage(err));
      setFields(fieldErrors(err));
      if (!creating) {
        setPassword("");
        setCode("");
      }
    } finally {
      setLoading(false);
    }
  }

  if (options?.needsSetup) return <SetupPage needsInstallCode={options.setupCode} />;
  if (inviting) return <InvitePage onBack={() => setInviting(false)} />;

  if (requested) {
    return (
      <AuthLayout title="Cadastro enviado" subtitle="Falta só a aprovação do administrador.">
        <Alert tone="info">
          Sua conta foi criada, mas só poderá entrar depois que um administrador aprovar o acesso. Avise a pessoa responsável
          pelo escritório e volte aqui em seguida.
        </Alert>
        <Button
          type="button"
          variant="secondary"
          className="mt-4 h-10 w-full"
          onClick={() => {
            setRequested(false);
            switchMode();
          }}
        >
          Voltar para a entrada
        </Button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title={creating ? "Crie sua conta" : "Boas-vindas ao ChatHM"} subtitle={creating ? "Comece a usar a central de atendimento." : byPassword ? "Entre com e-mail e senha." : "Entre com o código do seu app autenticador."}>
      <form onSubmit={onSubmit} className="flex flex-col gap-4" aria-busy={loading}>
        {DEMO && (
          <Alert tone="info">
            <strong>Demonstração.</strong> Digite qualquer código de 6 dígitos (ou use "Entrar com senha": <code>demo</code>) para <code>admin@hm.com.br</code> (admin),{" "}
            <code>heliton@hm.com.br</code> ou <code>paula@hm.com.br</code> (atendentes). Nada é salvo.
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        {creating && <Field label="Nome completo" autoComplete="name" required maxLength={120} value={name} error={fields.name} disabled={loading} onChange={(e) => setName(e.target.value)} />}
        <Field
          label="E-mail"
          placeholder="voce@escritorio.com.br"
          maxLength={254}
          error={fields.email}
          disabled={loading}
          type="email"
          autoComplete="username"
          autoFocus
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        {!creating && !byPassword ? (
          useRecovery ? (
            <Field
              label="Código de recuperação"
              placeholder="XXXXX-XXXXX"
              autoCapitalize="characters"
              className="[&_input]:font-mono [&_input]:uppercase"
              required
              disabled={loading}
              value={code}
              error={fields.code}
              onChange={(e) => setCode(e.target.value)}
            />
          ) : (
            <CodeInput value={code} onChange={setCode} error={fields.code} />
          )
        ) : (
          <Field
            label="Senha"
            type={showPassword ? "text" : "password"}
            autoComplete={creating ? "new-password" : "current-password"}
            minLength={creating ? 12 : undefined}
            maxLength={creating ? 128 : undefined}
            hint={creating ? "Use de 12 a 128 caracteres." : undefined}
            error={fields.password}
            disabled={loading}
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        )}
        {creating && <Field label="Confirmar senha" type={showPassword ? "text" : "password"} autoComplete="new-password" required value={confirmation} error={fields.confirmation} disabled={loading} onChange={(e) => setConfirmation(e.target.value)} />}
        {(creating || byPassword) && (
          <label className="flex items-center gap-2 text-xs text-muted">
            <input type="checkbox" checked={showPassword} onChange={(e) => setShowPassword(e.target.checked)} />
            Mostrar senha
          </label>
        )}
        {creating && <p className="text-xs text-muted">Sua conta será criada como atendente e só ficará ativa depois que o administrador aprovar.</p>}
        <Button type="submit" loading={loading} className="mt-2 h-10" disabled={!creating && !byPassword && !useRecovery && code.length !== 6}>
          {creating ? "Solicitar acesso" : "Entrar"}
        </Button>
        {!creating && (
          <div className="flex flex-col items-center gap-1.5 text-sm">
            {!byPassword && (
              <button
                type="button"
                className="text-muted hover:text-fg"
                onClick={() => {
                  setUseRecovery(!useRecovery);
                  setCode("");
                }}
              >
                {useRecovery ? "Usar o código do app" : "Perdi o celular: usar código de recuperação"}
              </button>
            )}
            <button
              type="button"
              className="text-muted hover:text-fg"
              onClick={() => {
                setByPassword(!byPassword);
                setError("");
                setCode("");
                setPassword("");
              }}
            >
              {byPassword ? "Entrar com o código do app" : "Entrar com senha"}
            </button>
          </div>
        )}
      </form>
      {!creating && (
        <div className="mt-5 border-t border-line pt-4 text-center">
          <Button type="button" variant="secondary" className="h-10 w-full" disabled={loading} onClick={() => setInviting(true)}>
            Tenho um convite
          </Button>
        </div>
      )}
      {canRegister && (
      <div className="mt-6 border-t border-line pt-5 text-center">
        <p className="mb-2 text-sm text-muted">{creating ? "Já tem uma conta?" : "Ainda não tem uma conta?"}</p>
        <Button type="button" variant="secondary" className="h-10 w-full" disabled={loading} onClick={switchMode}>
          {creating ? "Entrar na minha conta" : "Solicitar acesso"}
        </Button>
      </div>
      )}
    </AuthLayout>
  );
}
