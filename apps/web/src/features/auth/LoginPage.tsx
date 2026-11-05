import { useState, type FormEvent } from "react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage } from "@/lib/api";
import { login, useSetMe } from "./api";
import { AuthLayout } from "./AuthLayout";

export function LoginPage() {
  const setMe = useSetMe();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
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
