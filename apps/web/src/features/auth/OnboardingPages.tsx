import { useEffect, useState, type FormEvent } from "react";
import QRCode from "react-qr-code";
import { Check, Copy } from "lucide-react";
import { Alert, Button, Field, Spinner } from "@/components/ui";
import { ApiError, errorMessage, fieldErrors, type FieldErrors } from "@/lib/api";
import {
  enrollFinish,
  enrollStart,
  joinFinish,
  joinStart,
  setupFinish,
  setupStart,
  setupWhatsAppConnect,
  setupWhatsAppStatus,
  useSetMe,
  type Enrollment,
  type MeWithCodes,
  type SetupConnection,
} from "./api";
import { AuthLayout } from "./AuthLayout";
import { CodeInput } from "./CodeInput";

// ---------------------------------------------------------------------------
// Peças comuns

function Steps({ current, total }: { current: number; total: number }) {
  return (
    <p className="text-xs font-medium text-muted" aria-label={`Passo ${current} de ${total}`}>
      Passo {current} de {total}
    </p>
  );
}

/** Autenticador: QR do app + código de 6 dígitos. */
function AuthenticatorStep({
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
      <p className="text-sm text-muted">Escaneie com um app autenticador (Google Authenticator, Microsoft Authenticator…) e digite o código.</p>
      <div className="mx-auto rounded-xl bg-white p-3">
        <QRCode value={enrollment.otpauthUrl} size={168} />
      </div>
      <details className="text-sm text-muted">
        <summary className="cursor-pointer">Não consegue escanear?</summary>
        <code className="mt-2 block break-all rounded-lg bg-elevated p-2 font-mono text-xs text-fg">{enrollment.secret}</code>
      </details>
      <CodeInput value={code} onChange={setCode} error={error} />
      <Button type="submit" loading={busy} className="h-10" disabled={code.length !== 6}>
        {submitLabel}
      </Button>
    </form>
  );
}

/** Códigos de recuperação: mostrados uma única vez; só depois de guardá-los a pessoa entra. */
function RecoveryStep({ result, onDone }: { result: MeWithCodes; onDone: () => void }) {
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  return (
    <AuthLayout title="Guarde seus códigos" subtitle="Se perder o celular, cada código entra uma única vez. Não aparecem de novo.">
      <div className="flex flex-col gap-4">
        <ul className="grid grid-cols-2 gap-2 rounded-xl border border-line bg-elevated p-3 font-mono text-[13px]">
          {result.recoveryCodes.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
        <Button
          type="button"
          variant="secondary"
          onClick={() => void navigator.clipboard?.writeText(result.recoveryCodes.join("\n")).then(() => setCopied(true))}
        >
          <Copy className="size-4" /> {copied ? "Copiado" : "Copiar códigos"}
        </Button>
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

// ---------------------------------------------------------------------------
// Instalação inicial: WhatsApp (QR) → seus dados → autenticador

/** Tela no estilo do WhatsApp Web: texto curto à esquerda, QR grande à direita. */
function WhatsAppQrCard({ installCode, onConnected }: { installCode: string; onConnected: (c: SetupConnection) => void }) {
  const [key, setKey] = useState<string | null>(null);
  const [conn, setConn] = useState<SetupConnection | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function run() {
      try {
        const { key: k } = await setupWhatsAppConnect(installCode);
        if (!live) return;
        setKey(k);
        const poll = async () => {
          try {
            const c = await setupWhatsAppStatus(k, installCode);
            if (!live) return;
            setConn(c);
            if (c.state === "connected") return onConnected(c);
          } catch (e) {
            if (!live) return;
            setError(errorMessage(e));
            return;
          }
          timer = setTimeout(poll, 1500);
        };
        void poll();
      } catch (e) {
        if (live) setError(errorMessage(e));
      }
    }
    void run();
    return () => {
      live = false;
      clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reinicia só ao pedir um novo QR
  }, [attempt, installCode]);

  const expired = conn && (conn.state === "disconnected" || conn.state === "error");
  void key;

  return (
    <div className="mx-auto w-full max-w-3xl rounded-3xl border border-line bg-panel p-8 shadow-sm sm:p-12">
      <div className="flex flex-col items-center gap-8 md:flex-row md:justify-between">
        <div className="max-w-sm">
          <h1 className="text-2xl font-light sm:text-3xl">Escaneie para começar</h1>
          <ol className="mt-6 space-y-3 text-[15px]">
            {["Abra o WhatsApp no celular do escritório", "Toque em Aparelhos conectados", "Aponte a câmera para o QR Code"].map((t, i) => (
              <li key={t} className="flex items-center gap-3">
                <span className="flex size-6 shrink-0 items-center justify-center rounded-full border border-fg/60 text-xs">{i + 1}</span>
                {t}
              </li>
            ))}
          </ol>
          <p className="mt-6 text-xs text-muted">O número escaneado passa a ser o do escritório e você será o administrador.</p>
        </div>
        <div className="flex size-60 shrink-0 items-center justify-center rounded-2xl bg-white p-3">
          {error ? (
            <p className="text-center text-sm text-danger">{error}</p>
          ) : conn?.qr ? (
            <QRCode value={conn.qr.code} size={216} />
          ) : expired ? (
            <div className="flex flex-col items-center gap-3 text-center text-sm text-neutral-700">
              {conn?.lastError ?? "O QR Code expirou."}
              <Button
                type="button"
                onClick={() => {
                  setConn(null);
                  setError("");
                  setAttempt((n) => n + 1);
                }}
              >
                Gerar novo QR
              </Button>
            </div>
          ) : (
            <Spinner className="size-8" />
          )}
        </div>
      </div>
    </div>
  );
}

export function SetupPage({ needsInstallCode }: { needsInstallCode: boolean }) {
  const setMe = useSetMe();
  const [installCode, setInstallCode] = useState("");
  const [codeOk, setCodeOk] = useState(!needsInstallCode);
  const [whatsapp, setWhatsapp] = useState<SetupConnection | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [result, setResult] = useState<MeWithCodes | null>(null);
  const [fields, setFields] = useState<FieldErrors>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (result) return <RecoveryStep result={result} onDone={() => setMe(result)} />;

  // Passo 0 (opcional): código de instalação definido no servidor.
  if (!codeOk) {
    return (
      <AuthLayout title="Instalação" subtitle="Digite o código de instalação que você recebeu.">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setCodeOk(true);
          }}
          className="flex flex-col gap-4"
        >
          <Field label="Código de instalação" autoFocus required value={installCode} onChange={(e) => setInstallCode(e.target.value)} />
          <Button type="submit" className="h-10">
            Continuar
          </Button>
        </form>
      </AuthLayout>
    );
  }

  // Passo 1: QR do WhatsApp.
  if (!whatsapp) {
    return (
      <div className="flex min-h-full flex-col bg-app">
        <header className="px-6 py-4">
          <span className="text-lg font-semibold text-brand">ChatHM</span>
        </header>
        <main className="flex flex-1 items-center px-4 pb-16">
          <WhatsAppQrCard installCode={installCode} onConnected={setWhatsapp} />
        </main>
      </div>
    );
  }

  async function start(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setFields({});
    try {
      setEnrollment(await setupStart(name || whatsapp?.displayName || "", email, installCode));
    } catch (err) {
      if (err instanceof ApiError && err.code === "whatsapp_required") setWhatsapp(null);
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

  if (enrollment) {
    return (
      <AuthLayout title="Proteja seu acesso" subtitle="Você entrará sempre com o código do app, sem senha.">
        <div className="mb-4">
          <Steps current={3} total={3} />
        </div>
        <AuthenticatorStep enrollment={enrollment} onSubmit={finish} busy={busy} error={error} submitLabel="Ativar e continuar" />
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="WhatsApp conectado" subtitle={whatsapp.phone ? `Número ${whatsapp.phone}` : "Agora, seus dados."}>
      <div className="mb-4 flex items-center gap-2 text-sm text-brand">
        <Check className="size-4" /> Conectado
        <span className="ml-auto text-muted">
          <Steps current={2} total={3} />
        </span>
      </div>
      <form onSubmit={start} className="flex flex-col gap-4">
        {error && Object.keys(fields).length === 0 && <Alert>{error}</Alert>}
        <Field
          label="Seu nome"
          autoComplete="name"
          required
          maxLength={120}
          defaultValue={whatsapp.displayName}
          error={fields.name}
          onChange={(e) => setName(e.target.value)}
        />
        <Field label="E-mail" type="email" autoComplete="username" required maxLength={254} value={email} error={fields.email} onChange={(e) => setEmail(e.target.value)} />
        <Button type="submit" loading={busy} className="h-10">
          Continuar
        </Button>
      </form>
    </AuthLayout>
  );
}

// ---------------------------------------------------------------------------
// Equipe: token do escritório + e-mail → cria a conta

export function JoinPage({ onBack, onPersonalCode }: { onBack: () => void; onPersonalCode: () => void }) {
  const setMe = useSetMe();
  const [officeToken, setOfficeToken] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
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
      setEnrollment(await joinStart(name, email, officeToken));
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
      setResult(await joinFinish(officeToken, enrollment.token, code));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (result) return <RecoveryStep result={result} onDone={() => setMe(result)} />;

  if (enrollment) {
    return (
      <AuthLayout title="Proteja seu acesso" subtitle="Você entrará sempre com o código do app, sem senha.">
        <AuthenticatorStep enrollment={enrollment} onSubmit={finish} busy={busy} error={error} submitLabel="Criar conta e entrar" />
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title="Entrar na equipe" subtitle="Use o token que o administrador enviou.">
      <form onSubmit={start} className="flex flex-col gap-4">
        {error && Object.keys(fields).length === 0 && <Alert>{error}</Alert>}
        <Field
          label="Token do escritório"
          placeholder="XXXX-XXXX-XXXX"
          autoFocus
          required
          autoCapitalize="characters"
          className="[&_input]:font-mono [&_input]:uppercase"
          value={officeToken}
          onChange={(e) => setOfficeToken(e.target.value)}
        />
        <Field label="Seu nome" autoComplete="name" required maxLength={120} value={name} error={fields.name} onChange={(e) => setName(e.target.value)} />
        <Field label="E-mail" type="email" autoComplete="username" required maxLength={254} value={email} error={fields.email} onChange={(e) => setEmail(e.target.value)} />
        <Button type="submit" loading={busy} className="h-10">
          Continuar
        </Button>
        <div className="flex flex-col items-center gap-1.5 text-sm text-muted">
          <button type="button" className="hover:text-fg" onClick={onPersonalCode}>
            Tenho um código pessoal
          </button>
          <button type="button" className="hover:text-fg" onClick={onBack}>
            Voltar
          </button>
        </div>
      </form>
    </AuthLayout>
  );
}

// ---------------------------------------------------------------------------
// Código pessoal (convite individual / autenticador redefinido pelo administrador)

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
      title={enrollment ? `Olá, ${enrollment.name ?? ""}`.trim() : "Código pessoal"}
      subtitle={enrollment ? "Configure o autenticador para entrar sem senha." : "O código que o administrador gerou para você."}
    >
      {enrollment ? (
        <AuthenticatorStep enrollment={enrollment} onSubmit={finish} busy={busy} error={error} submitLabel="Ativar e entrar" />
      ) : (
        <form onSubmit={start} className="flex flex-col gap-4">
          {error && <Alert>{error}</Alert>}
          <Field label="E-mail" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <Field
            label="Código"
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
            Voltar
          </button>
        </form>
      )}
    </AuthLayout>
  );
}
