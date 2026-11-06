import { useState, type FormEvent } from "react";
import QRCode from "react-qr-code";
import { Copy } from "lucide-react";
import { Alert, Button, Field } from "@/components/ui";
import { errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import {
  enrollFinish,
  enrollStart,
  setupFinish,
  setupStart,
  useSetMe,
  type Enrollment,
  type MeWithCodes,
} from "./api";
import { AuthLayout } from "./AuthLayout";
import { CodeInput } from "./CodeInput";

function QrStep({
  enrollment,
  onSubmit,
  busy,
  error,
  submitLabel,
}: {
  enrollment: Enrollment;
  onSubmit: (code: string) => void;
  busy: boolean;
  error: string;
  submitLabel: string;
}) {
  const [code, setCode] = useState("");
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (code.length === 6) onSubmit(code);
      }}
      className="flex flex-col gap-4"
    >
      <ol className="list-decimal space-y-1 pl-5 text-sm text-muted">
        <li>Instale um app autenticador (Google Authenticator, Microsoft Authenticator, Authy…).</li>
        <li>Escaneie o QR Code abaixo.</li>
        <li>Digite o código de 6 dígitos que o app mostrar.</li>
      </ol>
      <div className="mx-auto rounded-xl bg-white p-3">
        <QRCode value={enrollment.otpauthUrl} size={176} />
      </div>
      <details className="text-sm text-muted">
        <summary className="cursor-pointer">Não consegue escanear?</summary>
        <p className="mt-2">Digite esta chave no aplicativo:</p>
        <code className="mt-1 block break-all rounded-lg bg-elevated p-2 font-mono text-xs text-fg">{enrollment.secret}</code>
      </details>
      <CodeInput value={code} onChange={setCode} error={error} />
      <Button type="submit" loading={busy} className="h-10" disabled={code.length !== 6}>
        {submitLabel}
      </Button>
    </form>
  );
}

/** Códigos de recuperação: mostrados uma única vez. Só depois de guardá-los a pessoa entra no app. */
function RecoveryStep({ result, onDone }: { result: MeWithCodes; onDone: () => void }) {
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  const text = result.recoveryCodes.join("\n");
  return (
    <AuthLayout title="Guarde seus códigos de recuperação" subtitle="Se perder o celular, cada código entra uma única vez no lugar do app.">
      <div className="flex flex-col gap-4">
        <ul className="grid grid-cols-2 gap-2 rounded-xl border border-line bg-elevated p-3 font-mono text-[13px]">
          {result.recoveryCodes.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
        <Button
          type="button"
          variant="secondary"
          onClick={() => {
            void navigator.clipboard?.writeText(text).then(() => setCopied(true));
          }}
        >
          <Copy className="size-4" /> {copied ? "Copiado" : "Copiar códigos"}
        </Button>
        <Alert tone="info">Eles não serão mostrados de novo. Guarde em um gerenciador de senhas ou em papel.</Alert>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
          Já guardei meus códigos
        </label>
        <Button type="button" className="h-10" disabled={!saved} onClick={onDone}>
          Entrar no ChatHM
        </Button>
      </div>
    </AuthLayout>
  );
}

/** Primeiro acesso ao sistema: cria o administrador e liga o autenticador. */
export function SetupPage({ needsInstallCode }: { needsInstallCode: boolean }) {
  const setMe = useSetMe();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [installCode, setInstallCode] = useState("");
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [result, setResult] = useState<MeWithCodes | null>(null);
  const [fields, setFields] = useState<FieldErrors>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function start(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setFields({});
    try {
      setEnrollment(await setupStart(name, email, installCode));
    } catch (err) {
      setError(errorMessage(err));
      setFields(fieldErrors(err));
    } finally {
      setBusy(false);
    }
  }

  async function finish(code: string) {
    if (!enrollment) return;
    setBusy(true);
    setError("");
    try {
      setResult(await setupFinish(enrollment.token, code));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (result) return <RecoveryStep result={result} onDone={() => setMe(result)} />;

  return (
    <AuthLayout
      title={enrollment ? "Configure o autenticador" : "Bem-vindo ao ChatHM"}
      subtitle={enrollment ? "Você entrará sempre com o código do app, sem senha." : "Crie o administrador do escritório para começar."}
    >
      {enrollment ? (
        <QrStep enrollment={enrollment} onSubmit={finish} busy={busy} error={error} submitLabel="Ativar e continuar" />
      ) : (
        <form onSubmit={start} className="flex flex-col gap-4">
          {error && Object.keys(fields).length === 0 && <Alert>{error}</Alert>}
          <Field label="Seu nome" autoComplete="name" required maxLength={120} value={name} error={fields.name} onChange={(e) => setName(e.target.value)} />
          <Field label="E-mail" type="email" autoComplete="username" required maxLength={254} value={email} error={fields.email} onChange={(e) => setEmail(e.target.value)} />
          {needsInstallCode && (
            <Field label="Código de instalação" required value={installCode} hint="Definido pelo responsável pelo servidor (SETUP_TOKEN)." onChange={(e) => setInstallCode(e.target.value)} />
          )}
          <Button type="submit" loading={busy} className="h-10">
            Continuar
          </Button>
        </form>
      )}
    </AuthLayout>
  );
}

/** Convite: e-mail + código recebido do administrador → configura o autenticador → entra. */
export function InvitePage({ onBack }: { onBack: () => void }) {
  const setMe = useSetMe();
  const [email, setEmail] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [result, setResult] = useState<MeWithCodes | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function start(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      setEnrollment(await enrollStart(email, inviteCode));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function finish(code: string) {
    if (!enrollment) return;
    setBusy(true);
    setError("");
    try {
      setResult(await enrollFinish(email, inviteCode, enrollment.token, code));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (result) return <RecoveryStep result={result} onDone={() => setMe(result)} />;

  return (
    <AuthLayout
      title={enrollment ? `Olá, ${enrollment.name ?? ""}`.trim() : "Tenho um convite"}
      subtitle={enrollment ? "Configure o autenticador para entrar sem senha." : "Digite o e-mail e o código que o administrador passou."}
    >
      {enrollment ? (
        <QrStep enrollment={enrollment} onSubmit={finish} busy={busy} error={error} submitLabel="Ativar e entrar" />
      ) : (
        <form onSubmit={start} className="flex flex-col gap-4">
          {error && <Alert>{error}</Alert>}
          <Field label="E-mail" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <Field
            label="Código do convite"
            required
            autoCapitalize="characters"
            placeholder="XXXXX-XXXXX"
            className="[&_input]:font-mono [&_input]:uppercase"
            value={inviteCode}
            onChange={(e) => setInviteCode(e.target.value)}
          />
          <Button type="submit" loading={busy} className="h-10">
            Continuar
          </Button>
          <button type="button" className="text-sm text-muted hover:text-fg" onClick={onBack}>
            Voltar para a entrada
          </button>
        </form>
      )}
    </AuthLayout>
  );
}
