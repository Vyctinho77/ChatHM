import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { DEMO } from "@/lib/env";
import { login, useSetMe } from "./api";
import { AuthLayout } from "./AuthLayout";

export function LoginPage() {
  const setMe = useSetMe();
  const [email, setEmail] = useState(DEMO ? "admin@hm.com.br" : "");
  const [password, setPassword] = useState(DEMO ? "demo" : "");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      setMe(await login(email, password));
    } catch (err) {
      setError(errorMessage(err));
      setPassword("");
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthLayout title="Entrar" subtitle="Central de atendimento do escritório">
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        {DEMO && (
          <Alert tone="info">
            <strong>Demonstração.</strong> Senha <code>demo</code> para <code>admin@hm.com.br</code> (admin),{" "}
            <code>heliton@hm.com.br</code> ou <code>paula@hm.com.br</code> (atendentes). Nada é salvo.
          </Alert>
        )}
        {error && <Alert>{error}</Alert>}
        <Field
          label="E-mail"
          type="email"
          autoComplete="username"
          autoFocus
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Field
          label="Senha"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <Button type="submit" loading={loading} className="mt-2 h-10">
          Entrar
        </Button>
      </form>
    </AuthLayout>
  );
}
