import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { login, register, useSetMe } from "./api";
import { AuthLayout } from "./AuthLayout";

export function LoginPage() {
  const setMe = useSetMe();
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
      setMe(await (creating ? register(name, email, password) : login(email, password)));
    } catch (err) {
      setError(errorMessage(err));
      setFields(fieldErrors(err));
      if (!creating) setPassword("");
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthLayout title={creating ? "Crie sua conta" : "Boas-vindas ao ChatHM"} subtitle={creating ? "Comece a usar a central de atendimento." : "Entre para continuar seu atendimento."}>
      <form onSubmit={onSubmit} className="flex flex-col gap-4" aria-busy={loading}>
        {DEMO && (
          <Alert tone="info">
            <strong>Demonstração.</strong> Senha <code>demo</code> para <code>admin@hm.com.br</code> (admin),{" "}
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
        {creating && <Field label="Confirmar senha" type={showPassword ? "text" : "password"} autoComplete="new-password" required value={confirmation} error={fields.confirmation} disabled={loading} onChange={(e) => setConfirmation(e.target.value)} />}
        <label className="flex items-center gap-2 text-xs text-muted">
          <input type="checkbox" checked={showPassword} onChange={(e) => setShowPassword(e.target.checked)} />
          Mostrar senha
        </label>
        {creating && <p className="text-xs text-muted">Sua conta será criada como atendente. O administrador gerencia as equipes e permissões.</p>}
        <Button type="submit" loading={loading} className="mt-2 h-10">
          {creating ? "Criar conta e entrar" : "Entrar"}
        </Button>
      </form>
      <div className="mt-6 border-t border-line pt-5 text-center">
        <p className="mb-2 text-sm text-muted">{creating ? "Já tem uma conta?" : "Ainda não tem uma conta?"}</p>
        <Button type="button" variant="secondary" className="h-10 w-full" disabled={loading} onClick={switchMode}>
          {creating ? "Entrar na minha conta" : "Criar minha conta"}
        </Button>
      </div>
    </AuthLayout>
  );
}
