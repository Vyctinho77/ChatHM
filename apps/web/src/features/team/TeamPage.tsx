import { useState, type FormEvent } from "react";
import { useSearchParams } from "react-router";
import { Copy, KeyRound, Pencil, Plus, ShieldCheck, Smartphone, Trash2, UserPlus } from "lucide-react";
import { Avatar } from "@/components/Avatar";
import { Alert, Button, Field, IconButton, Modal, Spinner } from "@/components/ui";
import { ApiError, errorMessage, type FieldErrors } from "@/lib/api";
import { cn } from "@/lib/cn";
import { useMe, type Role } from "@/features/auth/api";
import {
  useCreateTeam,
  useInviteUser,
  useReinviteUser,
  type InviteResult,
  useDeleteTeam,
  useMembers,
  useResetPassword,
  useSetUserTeams,
  useTeams,
  useUpdateUser,
  useUsers,
  type Team,
  type User,
} from "./api";

const roleLabel: Record<Role, string> = { admin: "Administrador", agent: "Atendente" };

function fieldsOf(err: unknown): FieldErrors {
  return err instanceof ApiError ? err.fields : {};
}

export function TeamPage() {
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const [params] = useSearchParams();
  const teamsList = useTeams();
  // Sem nenhuma equipe ainda, o gestor começa por Equipes (?aba=equipes também abre direto nela).
  const [chosen, setChosen] = useState<"members" | "teams" | null>(null);
  const tab: "members" | "teams" = !isAdmin
    ? "teams"
    : (chosen ?? (params.get("aba") === "equipes" || teamsList.data?.length === 0 ? "teams" : "members"));
  const setTab = setChosen;

  return (
    <div className="h-full overflow-y-auto bg-app">
      <div className="mx-auto max-w-5xl px-4 py-6 sm:px-8">
        <h1 className="text-xl font-semibold">Equipe</h1>
        <p className="mt-1 text-[13px] text-muted">
          {isAdmin
            ? "Cadastre atendentes e defina de quais equipes participam. A equipe determina quais filas cada pessoa vê."
            : "Equipes das quais você participa."}
        </p>

        {isAdmin && (
          <div role="tablist" className="mt-6 flex gap-1 border-b border-line">
            {(
              [
                ["members", "Pessoas"],
                ["teams", "Equipes"],
              ] as const
            ).map(([k, label]) => (
              <button
                key={k}
                role="tab"
                aria-selected={tab === k}
                onClick={() => setTab(k)}
                className={cn(
                  "-mb-px border-b-2 px-4 py-2 text-sm font-medium",
                  tab === k ? "border-accent text-fg" : "border-transparent text-muted hover:text-fg",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        )}

        <div className="mt-6">{isAdmin && tab === "members" ? <PeopleTab /> : <TeamsTab isAdmin={isAdmin} />}</div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pessoas (admin)

function PeopleTab() {
  const { data: me } = useMe();
  const users = useUsers(true);
  const [creating, setCreating] = useState(false);
  const [invited, setInvited] = useState<InviteResult | null>(null);
  const reinvite = useReinviteUser();
  const [reinviting, setReinviting] = useState<User | null>(null);
  const [editing, setEditing] = useState<User | null>(null);
  const [resetting, setResetting] = useState<User | null>(null);

  return (
    <>
      <div className="mb-4 flex justify-end">
        <Button onClick={() => setCreating(true)}>
          <UserPlus className="size-4" /> Convidar pessoa
        </Button>
      </div>
      {users.isPending && <Spinner className="mx-auto" />}
      {users.isError && <Alert>{errorMessage(users.error)}</Alert>}
      <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line bg-panel">
        {users.data?.map((u) => (
          <li key={u.id} className={cn("flex flex-col gap-2 px-4 py-3 md:flex-row md:items-center md:gap-3", !u.isActive && "opacity-60")}>
            <div className="flex min-w-0 flex-1 items-center gap-3">
              <Avatar name={u.name} />
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-2 font-medium">
                  <span className="truncate">{u.name}</span>
                  {u.id === me?.user.id && <span className="shrink-0 text-xs text-muted">(você)</span>}
                  {u.mfaEnabled && <ShieldCheck className="size-4 shrink-0 text-brand" aria-label="MFA ativo" />}
                </p>
                <p className="truncate text-sm text-muted">{u.email}</p>
              </div>
              <div className="flex shrink-0 md:order-last">
                <IconButton label={`Editar ${u.name}`} onClick={() => setEditing(u)}>
                  <Pencil className="size-4" />
                </IconButton>
                <IconButton label={`Novo código de acesso para ${u.name}`} onClick={() => setReinviting(u)}>
                  <Smartphone className="size-4" />
                </IconButton>
                <IconButton label={`Redefinir senha de ${u.name}`} onClick={() => setResetting(u)}>
                  <KeyRound className="size-4" />
                </IconButton>
              </div>
            </div>
            <div className="flex flex-wrap gap-1.5 pl-14 text-xs md:justify-end md:pl-0">
              <span className="rounded-full bg-hover px-2 py-0.5">{roleLabel[u.role]}</span>
              {!u.isActive &&
                (u.lastLoginAt ? (
                  <span className="rounded-full bg-danger/15 px-2 py-0.5 text-danger">Inativo</span>
                ) : (
                  <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-amber-700 dark:text-amber-300">Aguardando primeiro acesso</span>
                ))}
              {u.teams.map((t) => (
                <span key={t.id} className="rounded-full border border-line px-2 py-0.5">
                  {t.name}
                </span>
              ))}
              {u.teams.length === 0 && <span className="text-muted">Sem equipe</span>}
            </div>
          </li>
        ))}
      </ul>
      <InviteUserModal open={creating} onClose={() => setCreating(false)} onInvited={setInvited} />
      {invited && <InviteCodeModal result={invited} onClose={() => setInvited(null)} />}
      {reinviting && (
        <Modal open onClose={() => (reinvite.reset(), setReinviting(null))} title="Novo código de acesso">
          {reinvite.data ? (
            <InviteCodeBody result={reinvite.data} onClose={() => (reinvite.reset(), setReinviting(null))} />
          ) : (
            <div className="flex flex-col gap-4">
              <p className="text-sm text-muted">
                Gera um novo código para <strong className="text-fg">{reinviting.name}</strong> configurar o autenticador de novo. O
                autenticador atual, os códigos de recuperação e as sessões abertas dela serão <strong>encerrados agora</strong>.
              </p>
              {reinvite.isError && <Alert>{errorMessage(reinvite.error)}</Alert>}
              <div className="flex justify-end gap-2">
                <Button type="button" variant="secondary" onClick={() => setReinviting(null)}>
                  Cancelar
                </Button>
                <Button type="button" variant="danger" loading={reinvite.isPending} onClick={() => reinvite.mutate(reinviting.id)}>
                  Redefinir e gerar código
                </Button>
              </div>
            </div>
          )}
        </Modal>
      )}
      {editing && <EditUserModal user={editing} onClose={() => setEditing(null)} />}
      {resetting && <ResetPasswordModal user={resetting} onClose={() => setResetting(null)} />}
    </>
  );
}

function TeamCheckboxes({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const teams = useTeams();
  if (teams.isPending) return <Spinner />;
  if (!teams.data?.length) return <p className="text-sm text-muted">Nenhuma equipe cadastrada ainda.</p>;
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1.5 text-sm font-medium">Equipes</legend>
      {teams.data.map((t) => (
        <label key={t.id} className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-[var(--c-accent)]"
            checked={value.includes(t.id)}
            onChange={(e) => onChange(e.target.checked ? [...value, t.id] : value.filter((x) => x !== t.id))}
          />
          {t.name}
        </label>
      ))}
    </fieldset>
  );
}

function RoleSelect({ value, onChange }: { value: Role; onChange: (r: Role) => void }) {
  return (
    <fieldset className="flex flex-col gap-1.5">
      <legend className="mb-1.5 text-sm font-medium">Perfil</legend>
      <div className="flex gap-2">
        {(["agent", "admin"] as Role[]).map((r) => (
          <button
            key={r}
            type="button"
            aria-pressed={value === r}
            onClick={() => onChange(r)}
            className={cn(
              "flex-1 rounded-lg border px-3 py-2 text-sm",
              value === r ? "border-accent bg-accent/10 font-medium" : "border-line hover:bg-hover",
            )}
          >
            {roleLabel[r]}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

function InviteUserModal({ open, onClose, onInvited }: { open: boolean; onClose: () => void; onInvited: (r: InviteResult) => void }) {
  const invite = useInviteUser();
  const [form, setForm] = useState({ name: "", email: "", role: "agent" as Role, teamIds: [] as string[] });
  const fields = fieldsOf(invite.error);

  function close() {
    invite.reset();
    setForm({ name: "", email: "", role: "agent", teamIds: [] });
    onClose();
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    await invite.mutateAsync(form).then((r) => {
      close();
      onInvited(r);
    }, () => undefined);
  }

  return (
    <Modal open={open} onClose={close} title="Convidar pessoa">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <p className="text-sm text-muted">
          Você recebe um código de uso único para passar à pessoa. Ela o digita na tela de entrada, configura o app autenticador e
          entra já ativa e nas equipes escolhidas. Ninguém precisa de senha.
        </p>
        {invite.isError && Object.keys(fields).length === 0 && <Alert>{errorMessage(invite.error)}</Alert>}
        <Field label="Nome" value={form.name} error={fields.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        <Field label="E-mail" type="email" value={form.email} error={fields.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required />
        <RoleSelect value={form.role} onChange={(role) => setForm({ ...form, role })} />
        <TeamCheckboxes value={form.teamIds} onChange={(teamIds) => setForm({ ...form, teamIds })} />
        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="secondary" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" loading={invite.isPending}>
            Gerar convite
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function InviteCodeBody({ result, onClose }: { result: InviteResult; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const msg = `Seu acesso ao ChatHM: abra o app, toque em "Tenho um convite" e use o e-mail ${result.user.email} com o código ${result.inviteCode}. Vale até ${new Date(result.expiresAt).toLocaleString("pt-BR")}.`;
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted">
        Passe este código a <strong className="text-fg">{result.user.name}</strong>. Ele aparece só agora e vale por 72 horas, uma única vez.
      </p>
      <code className="block rounded-xl border border-line bg-elevated p-3 text-center font-mono text-2xl tracking-widest">{result.inviteCode}</code>
      <Button
        type="button"
        variant="secondary"
        onClick={() => void navigator.clipboard?.writeText(msg).then(() => setCopied(true))}
      >
        <Copy className="size-4" /> {copied ? "Mensagem copiada" : "Copiar mensagem pronta"}
      </Button>
      <div className="flex justify-end">
        <Button type="button" onClick={onClose}>
          Pronto
        </Button>
      </div>
    </div>
  );
}

function InviteCodeModal({ result, onClose }: { result: InviteResult; onClose: () => void }) {
  return (
    <Modal open onClose={onClose} title="Convite criado">
      <InviteCodeBody result={result} onClose={onClose} />
    </Modal>
  );
}

function EditUserModal({ user, onClose }: { user: User; onClose: () => void }) {
  const update = useUpdateUser();
  const setTeams = useSetUserTeams();
  const [name, setName] = useState(user.name);
  const [role, setRole] = useState<Role>(user.role);
  const [active, setActive] = useState(user.isActive);
  const current = user.teams.map((t) => t.id);
  const [teamIds, setTeamIds] = useState(current);
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await update.mutateAsync({ id: user.id, name, role, isActive: active });
      await setTeams.mutateAsync({ userId: user.id, current, next: teamIds });
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <Modal open onClose={onClose} title={`Editar ${user.name}`}>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {error && <Alert>{error}</Alert>}
        <Field label="Nome" value={name} onChange={(e) => setName(e.target.value)} required />
        <RoleSelect value={role} onChange={setRole} />
        <TeamCheckboxes value={teamIds} onChange={setTeamIds} />
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" className="size-4" checked={active} onChange={(e) => setActive(e.target.checked)} />
          Usuário ativo (desativar encerra as sessões imediatamente)
        </label>
        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" loading={update.isPending || setTeams.isPending}>
            Salvar
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function ResetPasswordModal({ user, onClose }: { user: User; onClose: () => void }) {
  const reset = useResetPassword();
  const [password, setPassword] = useState("");
  const fields = fieldsOf(reset.error);
  return (
    <Modal open onClose={onClose} title="Redefinir senha">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          reset.mutateAsync({ id: user.id, password }).then(onClose, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <p className="text-sm text-muted">
          Define uma nova senha para <strong className="text-fg">{user.name}</strong> e encerra todas as sessões abertas.
        </p>
        {reset.isError && !fields.password && <Alert>{errorMessage(reset.error)}</Alert>}
        <Field
          label="Nova senha"
          type="password"
          autoComplete="new-password"
          value={password}
          error={fields.password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" loading={reset.isPending}>
            Redefinir
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Equipes

function TeamsTab({ isAdmin }: { isAdmin: boolean }) {
  const teams = useTeams();
  const [creating, setCreating] = useState(false);
  return (
    <>
      {isAdmin && (
        <div className="mb-4 flex justify-end">
          <Button onClick={() => setCreating(true)}>
            <Plus className="size-4" /> Nova equipe
          </Button>
        </div>
      )}
      {teams.isPending && <Spinner className="mx-auto" />}
      {teams.data?.length === 0 && (
        <p className="rounded-xl border border-dashed border-line p-8 text-center text-sm text-muted">
          {isAdmin
            ? "Nenhuma equipe ainda. Crie a primeira com “Nova equipe” (ex.: Folha de pagamento, Fiscal, Atendimento). As equipes definem quais filas cada pessoa enxerga."
            : "Você ainda não participa de nenhuma equipe."}
        </p>
      )}
      <div className="grid gap-4 md:grid-cols-2">
        {teams.data?.map((t) => <TeamCard key={t.id} team={t} isAdmin={isAdmin} />)}
      </div>
      <CreateTeamModal open={creating} onClose={() => setCreating(false)} />
    </>
  );
}

function TeamCard({ team, isAdmin }: { team: Team; isAdmin: boolean }) {
  const members = useMembers(team.id);
  const remove = useDeleteTeam();
  const [confirming, setConfirming] = useState(false);
  return (
    <article className="rounded-xl border border-line bg-panel p-4">
      <header className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <h2 className="font-semibold">{team.name}</h2>
          {team.description && <p className="text-sm text-muted">{team.description}</p>}
          <p className="mt-1 text-xs text-muted">
            {team.memberCount} {team.memberCount === 1 ? "membro" : "membros"}
          </p>
        </div>
        {isAdmin && (
          <IconButton
            label={`Excluir equipe ${team.name}`}
            onClick={() => setConfirming(true)}
          >
            <Trash2 className="size-4" />
          </IconButton>
        )}
      </header>
      <Modal open={confirming} onClose={() => setConfirming(false)} title="Excluir equipe">
        <p className="text-sm">
          Excluir a equipe <strong>{team.name}</strong>? Os vínculos dos membros serão removidos.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => setConfirming(false)}>
            Cancelar
          </Button>
          <Button variant="danger" loading={remove.isPending} onClick={() => remove.mutate(team.id, { onSuccess: () => setConfirming(false) })}>
            Excluir
          </Button>
        </div>
      </Modal>
      <ul className="mt-3 flex flex-col gap-2">
        {members.data?.map((m) => (
          <li key={m.userId} className="flex items-center gap-2 text-sm">
            <Avatar name={m.name} size="sm" />
            <span className="truncate">{m.name}</span>
            <span className="ml-auto text-xs text-muted">{roleLabel[m.role]}</span>
          </li>
        ))}
      </ul>
    </article>
  );
}

function CreateTeamModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateTeam();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const fields = fieldsOf(create.error);
  function close() {
    create.reset();
    setName("");
    setDescription("");
    onClose();
  }
  return (
    <Modal open={open} onClose={close} title="Nova equipe">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutateAsync({ name, description }).then(close, () => undefined);
        }}
        className="flex flex-col gap-4"
      >
        <Field label="Nome" value={name} error={fields.name} onChange={(e) => setName(e.target.value)} required />
        <Field label="Descrição (opcional)" value={description} error={fields.description} onChange={(e) => setDescription(e.target.value)} />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" loading={create.isPending}>
            Criar
          </Button>
        </div>
      </form>
    </Modal>
  );
}
