// API simulada em memória para a versão de demonstração (VITE_DEMO=1).
// Reproduz as regras principais do backend: perfis, visibilidade por equipe,
// validações e o último admin. Nada é persistido.
import type { Theme } from "./theme";

type Role = "admin" | "agent";
type U = { id: string; email: string; name: string; role: Role; isActive: boolean; mfaEnabled: boolean; theme: Theme; password: string; lastLoginAt: string | null; createdAt: string };
type T = { id: string; name: string; description: string; createdAt: string };

export class DemoError extends Error {
  constructor(public status: number, public code: string, message: string, public fields: Record<string, string> = {}) {
    super(message);
  }
}

const now = () => new Date().toISOString();
const uid = () => crypto.randomUUID();

const users: U[] = [
  { id: uid(), email: "admin@hm.com.br", name: "Admin HM", role: "admin", isActive: true, mfaEnabled: false, theme: "system", password: "demo", lastLoginAt: null, createdAt: now() },
  { id: uid(), email: "heliton@hm.com.br", name: "Heliton Souza", role: "agent", isActive: true, mfaEnabled: false, theme: "system", password: "demo", lastLoginAt: null, createdAt: now() },
  { id: uid(), email: "paula@hm.com.br", name: "Paula Reis", role: "agent", isActive: true, mfaEnabled: false, theme: "system", password: "demo", lastLoginAt: null, createdAt: now() },
];
const teams: T[] = ["Clientes novos", "Emissão de notas", "Folha de pagamento"].map((name) => ({ id: uid(), name, description: "", createdAt: now() }));
const members = new Set<string>([`${teams[2]!.id}:${users[1]!.id}`, `${teams[1]!.id}:${users[2]!.id}`]);

let session: U | null = null;

// Conexão WhatsApp simulada: o "celular" lê o QR alguns segundos após conectar.
let wa: { state: string; phone: string; displayName: string; connectedAt?: string; pairAt?: number; lastError?: string } = {
  state: "disconnected",
  phone: "",
  displayName: "",
};
function waView(admin: boolean) {
  if (wa.state === "pending_qr" && wa.pairAt && Date.now() >= wa.pairAt) {
    wa = { state: "connected", phone: "+55 11 90000-0000", displayName: "Escritório HM (demonstração)", connectedAt: now() };
  }
  const { pairAt, ...pub } = wa;
  const qr =
    admin && wa.state === "pending_qr"
      ? { code: "chathm-demonstracao-sem-celular-" + Math.floor((pairAt ?? 0) / 1000), expiresAt: new Date((pairAt ?? Date.now()) + 2000).toISOString() }
      : null;
  return { ...pub, qr };
}
const csrf = "demo-csrf";

const isMember = (t: string, u: string) => members.has(`${t}:${u}`);
const teamsOf = (u: U) => teams.filter((t) => isMember(t.id, u.id)).map(({ id, name }) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
const memberCount = (t: T) => users.filter((u) => isMember(t.id, u.id)).length;

const me = (u: U) => ({
  user: { id: u.id, email: u.email, name: u.name, role: u.role, theme: u.theme, mfaEnabled: u.mfaEnabled },
  csrfToken: csrf,
  mfaRequired: false,
  mfaSetupRequired: false,
});
const userView = (u: U) => ({ id: u.id, email: u.email, name: u.name, role: u.role, isActive: u.isActive, mfaEnabled: u.mfaEnabled, lastLoginAt: u.lastLoginAt, createdAt: u.createdAt, teams: teamsOf(u) });
const teamView = (t: T) => ({ ...t, memberCount: memberCount(t) });

function requireSession(): U {
  if (!session) throw new DemoError(401, "unauthenticated", "Faça login para continuar.");
  return session;
}
function requireAdmin(): U {
  const s = requireSession();
  if (s.role !== "admin") throw new DemoError(403, "forbidden", "Acesso restrito a administradores.");
  return s;
}
const invalid = (fields: Record<string, string>) => new DemoError(422, "validation_failed", "Dados inválidos.", fields);
const notFound = (what: string) => new DemoError(404, "not_found", `${what} não encontrado(a).`);

function checkPassword(pw: string) {
  if ([...pw].length < 12) return "A senha deve ter pelo menos 12 caracteres.";
  return null;
}

function canSeeTeam(s: U, id: string) {
  const t = teams.find((x) => x.id === id);
  if (!t || (s.role !== "admin" && !isMember(id, s.id))) throw notFound("Equipe");
  return t;
}

type Body = Record<string, unknown>;

export async function demoApi(method: string, path: string, body: Body = {}): Promise<unknown> {
  await new Promise((r) => setTimeout(r, 120));
  const seg = path.split("/").filter(Boolean);
  const route = `${method} /${seg.map((s, i) => (i > 0 && /^[0-9a-f-]{36}$/.test(s) ? ":id" : s)).join("/")}`;
  const id = seg[1];
  const sub = seg[3];

  switch (route) {
    case "GET /auth/me":
      return me(requireSession());
    case "POST /auth/login": {
      const u = users.find((x) => x.email === String(body.email ?? "").trim().toLowerCase());
      if (!u || !u.isActive || u.password !== body.password) throw new DemoError(401, "invalid_credentials", "E-mail ou senha inválidos.");
      u.lastLoginAt = now();
      session = u;
      return me(u);
    }
    case "POST /auth/logout":
      session = null;
      return undefined;
    case "POST /auth/password": {
      const s = requireSession();
      if (body.currentPassword !== s.password) throw invalid({ currentPassword: "Senha atual incorreta." });
      const err = checkPassword(String(body.newPassword ?? ""));
      if (err) throw invalid({ newPassword: err });
      s.password = String(body.newPassword);
      return undefined;
    }
    case "POST /auth/mfa/setup":
    case "POST /auth/mfa/enable":
    case "POST /auth/mfa/verify":
      throw new DemoError(409, "demo", "A verificação em duas etapas não está disponível na demonstração.");
    case "PATCH /me/preferences":
      requireSession().theme = body.theme as Theme;
      return { theme: body.theme };

    case "GET /whatsapp/connection":
      return waView(requireSession().role === "admin");
    case "POST /whatsapp/connect": {
      requireAdmin();
      if (wa.state === "connected") throw new DemoError(409, "already_connected", "O WhatsApp já está conectado.");
      if (wa.state !== "pending_qr") wa = { state: "pending_qr", phone: "", displayName: "", pairAt: Date.now() + 7000 };
      return waView(true);
    }
    case "POST /whatsapp/disconnect":
      requireAdmin();
      wa = { state: "disconnected", phone: "", displayName: "" };
      return waView(true);

    case "GET /users":
      requireAdmin();
      return { users: users.map(userView).sort((a, b) => a.name.localeCompare(b.name)) };
    case "POST /users": {
      const a = requireAdmin();
      void a;
      const f: Record<string, string> = {};
      const email = String(body.email ?? "").trim().toLowerCase();
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) f.email = "E-mail inválido.";
      else if (users.some((u) => u.email === email)) f.email = "E-mail já cadastrado.";
      const name = String(body.name ?? "").trim();
      if (!name) f.name = "Informe um nome de até 120 caracteres.";
      const pwErr = checkPassword(String(body.password ?? ""));
      if (pwErr) f.password = pwErr;
      if (Object.keys(f).length) throw invalid(f);
      const u: U = { id: uid(), email, name, role: body.role as Role, isActive: true, mfaEnabled: false, theme: "system", password: String(body.password), lastLoginAt: null, createdAt: now() };
      users.push(u);
      for (const t of (body.teamIds as string[]) ?? []) members.add(`${t}:${u.id}`);
      return userView(u);
    }
    case "PATCH /users/:id": {
      const a = requireAdmin();
      const u = users.find((x) => x.id === id);
      if (!u) throw notFound("Usuário");
      if (body.isActive === false && u.id === a.id) throw new DemoError(409, "self_deactivate", "Você não pode desativar o próprio usuário.");
      const losing = u.role === "admin" && u.isActive && ((body.role && body.role !== "admin") || body.isActive === false);
      if (losing && users.filter((x) => x.role === "admin" && x.isActive).length <= 1) throw new DemoError(409, "last_admin", "Deve existir ao menos um administrador ativo.");
      if (typeof body.name === "string") u.name = body.name.trim();
      if (body.role) u.role = body.role as Role;
      if (typeof body.isActive === "boolean") u.isActive = body.isActive;
      if (session?.id === u.id && u.role !== "admin") session = u;
      return userView(u);
    }
    case "POST /users/:id/password": {
      requireAdmin();
      const u = users.find((x) => x.id === id);
      if (!u) throw notFound("Usuário");
      const err = checkPassword(String(body.password ?? ""));
      if (err) throw invalid({ password: err });
      u.password = String(body.password);
      return undefined;
    }

    case "GET /teams": {
      const s = requireSession();
      const list = s.role === "admin" ? teams : teams.filter((t) => isMember(t.id, s.id));
      return { teams: [...list].sort((a, b) => a.name.localeCompare(b.name)).map(teamView) };
    }
    case "POST /teams": {
      requireAdmin();
      const name = String(body.name ?? "").trim();
      if (!name) throw invalid({ name: "Informe um nome de até 80 caracteres." });
      if (teams.some((t) => t.name.toLowerCase() === name.toLowerCase())) throw invalid({ name: "Já existe uma equipe com esse nome." });
      const t = { id: uid(), name, description: String(body.description ?? "").trim(), createdAt: now() };
      teams.push(t);
      return teamView(t);
    }
    case "GET /teams/:id":
      return teamView(canSeeTeam(requireSession(), id!));
    case "GET /teams/:id/members": {
      const t = canSeeTeam(requireSession(), id!);
      return {
        members: users
          .filter((u) => isMember(t.id, u.id))
          .map((u) => ({ userId: u.id, name: u.name, email: u.email, role: u.role, isActive: u.isActive, joinedAt: u.createdAt })),
      };
    }
    case "DELETE /teams/:id": {
      requireAdmin();
      const i = teams.findIndex((t) => t.id === id);
      if (i < 0) throw notFound("Equipe");
      teams.splice(i, 1);
      return undefined;
    }
    case "PUT /teams/:id/members/:id":
      requireAdmin();
      members.add(`${id}:${sub}`);
      return undefined;
    case "DELETE /teams/:id/members/:id":
      requireAdmin();
      members.delete(`${id}:${sub}`);
      return undefined;
  }
  throw new DemoError(404, "not_found", "Rota não encontrada.");
}
