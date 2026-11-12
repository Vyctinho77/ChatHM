import { useState, type FormEvent } from "react";
import { ShieldCheck, ShieldOff } from "lucide-react";
import { Alert, Button, Field } from "@/components/ui";
import { InfoTip } from "@/components/InfoTip";
import { ApiError, errorMessage, post } from "@/lib/api";
import { APP_VERSION } from "@/lib/version";
import { DEMO } from "@/lib/env";
import { regenerateRecoveryCodes, useMe } from "@/features/auth/api";
import { ThemeSwitcher } from "@/features/shell/ThemeSwitcher";
import { HandoverCard } from "./HandoverCard";
import { SoundSettings } from "./SoundSettings";

/** Cartão de configuração. A explicação (`info`) fica atrás do botão "i", para a tela mostrar só o que importa. */
function Card({ title, info, children }: { title: string; info?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="border-t border-line py-6 first:border-t-0 first:pt-0">
      <div className="flex items-center gap-1.5">
        <h2 className="font-semibold">{title}</h2>
        {info && <InfoTip title={title}>{info}</InfoTip>}
      </div>
      <div className="mt-4">{children}</div>
    </section>
  );
}

export function SettingsPage() {
  const { data: me } = useMe();
  if (!me) return null;
  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto flex max-w-2xl flex-col px-4 py-6 sm:px-8">
        <h1 className="mb-6 text-xl font-semibold">Configurações</h1>

        <Card title="Aparência" info="Claro, escuro ou acompanhar o sistema. Fica salvo no seu usuário.">
          <ThemeSwitcher className="w-fit" />
        </Card>

        <Card title="Perfil">
          <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
            <dt className="text-muted">Nome</dt>
            <dd>{me.user.name}</dd>
            <dt className="text-muted">E-mail</dt>
            <dd>{me.user.email}</dd>
            <dt className="text-muted">Perfil</dt>
            <dd>{me.user.role === "admin" ? "Administrador" : "Atendente"}</dd>
          </dl>
        </Card>

        <Card title="Verificação em duas etapas" info="Pede um código do aplicativo autenticador além da senha. É exigida de administradores em produção e opcional para atendentes.">
          <p className="flex items-center gap-2 text-sm">
            {me.user.mfaEnabled ? (
              <>
                <ShieldCheck className="size-5 text-brand-ink" /> Ativa neste usuário.
              </>
            ) : (
              <>
                <ShieldOff className="size-5 text-muted" />
                Inativa
              </>
            )}
          </p>
        </Card>

        {me.user.mfaEnabled && <RecoveryCodes />}

        <ChangePassword />

        {me.user.role === "admin" && <HandoverCard />}

        <Card title="Sons e notificações" info="Efeitos ao enviar e receber mensagens e avisos sonoros de novas mensagens. As preferências ficam salvas neste navegador para a sua conta.">
          <SoundSettings />
        </Card>

        <Card title="Sobre o app">
          <p className="text-sm text-muted">ChatHM {APP_VERSION}</p>
          {DEMO && <p className="mt-2 text-xs text-muted">Demonstração: nada é salvo.</p>}
        </Card>
      </div>
    </div>
  );
}

function ChangePassword() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);
  const [loading, setLoading] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setFields({});
    setError("");
    setOk(false);
    setLoading(true);
    try {
      await post("/auth/password", { currentPassword: current, newPassword: next });
      setOk(true);
      setCurrent("");
      setNext("");
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) setFields(err.fields);
      else setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card title="Trocar senha" info="Ao trocar a senha, as outras sessões abertas são encerradas. Use no mínimo 12 caracteres.">
      <form onSubmit={submit} className="flex w-full max-w-sm flex-col gap-4">
        {ok && <Alert tone="info">Senha alterada.</Alert>}
        {error && <Alert>{error}</Alert>}
        <Field label="Senha atual" type="password" autoComplete="current-password" value={current} error={fields.currentPassword} onChange={(e) => setCurrent(e.target.value)} />
        <Field label="Nova senha" type="password" autoComplete="new-password" value={next} error={fields.newPassword} onChange={(e) => setNext(e.target.value)} />
        <Button type="submit" loading={loading} className="w-fit">
          Salvar nova senha
        </Button>
      </form>
    </Card>
  );
}

function RecoveryCodes() {
  const [codes, setCodes] = useState<string[] | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  return (
    <Card title="Códigos de recuperação" info="Cada código entra uma única vez se você perder o celular. Gerar novos códigos invalida os anteriores; guarde-os em lugar seguro.">
      {error && <Alert>{error}</Alert>}
      {codes && (
        <ul className="mb-3 grid grid-cols-2 gap-x-6 gap-y-1.5 font-mono text-[13px]">
          {codes.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
      )}
      <Button
        variant="secondary"
        loading={loading}
        onClick={async () => {
          setLoading(true);
          setError("");
          try {
            setCodes((await regenerateRecoveryCodes()).recoveryCodes);
          } catch (e) {
            setError(errorMessage(e));
          } finally {
            setLoading(false);
          }
        }}
      >
        {codes ? "Gerar outros códigos" : "Gerar novos códigos"}
      </Button>
    </Card>
  );
}
