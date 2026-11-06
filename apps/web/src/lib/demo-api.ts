// API simulada em memória para a versão de demonstração (VITE_DEMO=1).
// Reproduz as regras principais do backend: perfis, visibilidade por equipe,
// validações e o último admin. Nada é persistido.
import { classify, normalize, type DemoKeyword } from "./classify";
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

// --- Segmentos e etiquetas (mesmos padrões do backend) ----------------------
type DKeyword = { id: string; phrase: string; normalized: string; priority: number };
type DSegment = { id: string; key: string; name: string; teamId: string | null; isActive: boolean; keywords: DKeyword[] };

const seg = (key: string, name: string, teamIdx: number, phrases: string[]): DSegment => ({
  id: uid(), key, name, teamId: teams[teamIdx]!.id, isActive: true,
  keywords: phrases.map((phrase) => ({ id: uid(), phrase, normalized: normalize(phrase), priority: 0 })),
});
const segmentsDb: DSegment[] = [
  seg("clientes_novos", "Clientes novos", 0, ["orçamento", "orçar", "proposta", "contratar", "contratação", "novo cliente", "abrir empresa", "abrir mei", "quanto custa", "honorários", "trocar de contador"]),
  seg("folha", "Folha de pagamento", 2, ["folha", "folha de pagamento", "férias", "13º salário", "décimo terceiro", "rescisão", "admissão", "demissão", "holerite", "contracheque", "esocial", "fgts"]),
  seg("notas", "Emissão de notas", 1, ["emitir nota", "emissão de nota", "nota fiscal", "nfe", "nfse", "cancelar nota", "gerar nota"]),
];
const tagsDb: { id: string; name: string }[] = ["PENDENTE", "URGENTE", "PGDAS", "CERTIFICADO", "ACOMPANHAMENTO"].map((name) => ({ id: uid(), name }));

const segmentView = (s: DSegment, withKeywords: boolean) => ({
  id: s.id, key: s.key, name: s.name, teamId: s.teamId,
  teamName: teams.find((t) => t.id === s.teamId)?.name ?? "",
  isActive: s.isActive,
  ...(withKeywords ? { keywords: [...s.keywords].sort((a, b) => b.priority - a.priority || a.phrase.localeCompare(b.phrase)).map(({ id, phrase, priority }) => ({ id, phrase, priority })) } : {}),
});
const slug = (n: string) =>
  n.toLowerCase().normalize("NFKD").replace(/\p{M}/gu, "").replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 36) || "segmento";
const findSegment = (id: string | undefined) => {
  const s = segmentsDb.find((x) => x.id === id);
  if (!s) throw new DemoError(404, "not_found", "Segmento não encontrado.");
  return s;
};

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
    case "POST /auth/register": {
      const email = String(body.email ?? "").trim().toLowerCase();
      const name = String(body.name ?? "").trim();
      const password = String(body.password ?? "");
      const fields: Record<string, string> = {};
      if (!name || [...name].length > 120) fields.name = "Informe um nome de até 120 caracteres.";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || email.length > 254) fields.email = "E-mail inválido.";
      if ([...password].length < 12 || [...password].length > 128) fields.password = "Use de 12 a 128 caracteres.";
      if (Object.keys(fields).length) throw invalid(fields);
      if (users.some((u) => u.email === email)) throw new DemoError(409, "email_unavailable", "Não foi possível usar este e-mail. Entre com sua conta ou use outro endereço.");
      users.push({ id: uid(), name, email, password, role: "agent", isActive: false, mfaEnabled: false, theme: "system", lastLoginAt: null, createdAt: now() });
      return { pending: true };
    }
    case "GET /auth/options":
      return { selfRegister: true, needsSetup: false, setupCode: false };
    case "POST /auth/join/start": {
      const email = String(body.email ?? "").trim().toLowerCase();
      if (String(body.officeToken ?? "").replace(/[^A-Za-z0-9]/g, "").toUpperCase() !== "DEMOK2M9X7QP") throw new DemoError(401, "invalid_token", "Token inválido. Peça o token atual ao administrador.");
      if (users.some((u) => u.email === email)) throw new DemoError(409, "email_unavailable", "Não foi possível usar este e-mail. Entre com sua conta ou use outro endereço.");
      return { secret: "JBSWY3DPEHPK3PXP", otpauthUrl: "otpauth://totp/ChatHM:demo?secret=JBSWY3DPEHPK3PXP&issuer=ChatHM", token: JSON.stringify({ name: body.name, email }), name: body.name };
    }
    case "POST /auth/join/finish": {
      const c = JSON.parse(String(body.token)) as { name: string; email: string };
      const u: U = { id: uid(), name: c.name, email: c.email, password: uid(), role: "agent", isActive: true, mfaEnabled: true, theme: "system", lastLoginAt: now(), createdAt: now() };
      users.push(u);
      session = u;
      return { ...me(u), recoveryCodes: Array.from({ length: 8 }, () => `${uid().slice(0, 5)}-${uid().slice(0, 5)}`.toUpperCase()) };
    }
    case "POST /office/handover": {
      requireAdmin();
      if (String(body.confirm ?? "").trim().toUpperCase() !== "ENTREGAR") throw invalid({ confirm: "Digite ENTREGAR para confirmar." });
      session = null;
      return { installCode: "DEMO7-K2M9X" };
    }
    case "GET /office/access":
      requireAdmin();
      return { token: "DEMO-K2M9-X7QP", allowedDomain: "" };
    case "POST /office/access/rotate":
      requireAdmin();
      return { token: "NOVO-T0KE-N123", allowedDomain: "" };
    case "PATCH /office/access":
      requireAdmin();
      return { token: "DEMO-K2M9-X7QP", allowedDomain: String(body.allowedDomain ?? "") };
    case "POST /auth/recovery-codes":
      requireSession();
      return { recoveryCodes: Array.from({ length: 8 }, () => `${uid().slice(0, 5)}-${uid().slice(0, 5)}`.toUpperCase()) };
    case "POST /users/invite": {
      requireAdmin();
      const email = String(body.email ?? "").trim().toLowerCase();
      const name = String(body.name ?? "").trim();
      const f: Record<string, string> = {};
      if (!name) f.name = "Informe um nome de até 120 caracteres.";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) f.email = "E-mail inválido.";
      else if (users.some((u) => u.email === email)) f.email = "E-mail já cadastrado.";
      if (Object.keys(f).length) throw invalid(f);
      const u: U = { id: uid(), name, email, password: uid(), role: body.role === "admin" ? "admin" : "agent", isActive: false, mfaEnabled: false, theme: "system", lastLoginAt: null, createdAt: now() };
      users.push(u);
      for (const t of (body.teamIds as string[] | undefined) ?? []) members.add(`${t}:${u.id}`);
      return { user: userView(u), inviteCode: "DEMO7-K2M9X", expiresAt: new Date(Date.now() + 72 * 3600_000).toISOString() };
    }
    case "GET /auth/me":
      return me(requireSession());
    case "POST /auth/login": {
      const u = users.find((x) => x.email === String(body.email ?? "").trim().toLowerCase());
      // Na demonstração, qualquer código de 6 dígitos vale como código do autenticador.
      const byCode = !body.password && /^\d{6}$/.test(String(body.code ?? ""));
      if (!u || !u.isActive || (!byCode && u.password !== body.password)) throw new DemoError(401, "invalid_credentials", body.password ? "E-mail ou senha inválidos." : "E-mail ou código inválidos.");
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

    case "GET /segments": {
      const admin = requireSession().role === "admin";
      return { segments: segmentsDb.map((s) => segmentView(s, admin)) };
    }
    case "POST /segments/examples":
      requireAdmin();
      return undefined;
    case "POST /segments": {
      requireAdmin();
      const name = String(body.name ?? "").trim();
      if (!name) throw invalid({ name: "Informe um nome de até 80 caracteres." });
      if (segmentsDb.some((s) => s.name.toLowerCase() === name.toLowerCase())) throw invalid({ name: "Já existe um segmento com esse nome." });
      const teamId = (body.teamId as string | null) ?? null;
      if (teamId && !teams.some((t) => t.id === teamId)) throw invalid({ teamId: "Equipe não encontrada." });
      let key = slug(name);
      for (let i = 2; segmentsDb.some((s) => s.key === key); i++) key = `${slug(name)}_${i}`;
      const s: DSegment = { id: uid(), key, name, teamId, isActive: true, keywords: [] };
      segmentsDb.push(s);
      return segmentView(s, true);
    }
    case "POST /segments/classify": {
      requireAdmin();
      const kws: DemoKeyword[] = segmentsDb.filter((s) => s.isActive).flatMap((s) =>
        s.keywords.map((k) => ({ segmentId: s.id, segmentKey: s.key, phrase: k.phrase, normalized: k.normalized, priority: k.priority })),
      );
      return classify(String(body.text ?? ""), kws);
    }
    case "PATCH /segments/:id": {
      requireAdmin();
      const s = findSegment(id);
      if (typeof body.name === "string") {
        const name = body.name.trim();
        if (!name) throw invalid({ name: "Informe um nome de até 80 caracteres." });
        if (segmentsDb.some((x) => x.id !== s.id && x.name.toLowerCase() === name.toLowerCase())) throw invalid({ name: "Já existe um segmento com esse nome." });
        s.name = name;
      }
      if (typeof body.isActive === "boolean") s.isActive = body.isActive;
      return segmentView(s, true);
    }
    case "PUT /segments/:id/team": {
      requireAdmin();
      const s = findSegment(id);
      const teamId = (body.teamId as string | null) ?? null;
      if (teamId && !teams.some((t) => t.id === teamId)) throw invalid({ teamId: "Equipe não encontrada." });
      s.teamId = teamId;
      return segmentView(s, true);
    }
    case "DELETE /segments/:id": {
      requireAdmin();
      segmentsDb.splice(segmentsDb.indexOf(findSegment(id)), 1);
      return undefined;
    }
    case "POST /segments/:id/keywords": {
      requireAdmin();
      const s = findSegment(id);
      const phrase = String(body.phrase ?? "").trim();
      const priority = Number(body.priority ?? 0);
      const norm = normalize(phrase);
      if (!phrase || phrase.length > 120) throw invalid({ phrase: "Informe uma palavra ou expressão de até 120 caracteres." });
      if (!norm) throw invalid({ phrase: 'Use ao menos uma palavra com significado (palavras como "de", "uma" e "para" são ignoradas).' });
      if (!(priority >= 0 && priority <= 100)) throw invalid({ priority: "Prioridade deve ficar entre 0 e 100." });
      if (s.keywords.some((k) => k.normalized === norm)) throw invalid({ phrase: "Essa palavra já está cadastrada neste segmento." });
      const k = { id: uid(), phrase, normalized: norm, priority };
      s.keywords.push(k);
      return { id: k.id, phrase, priority };
    }
    case "PATCH /segments/:id/keywords/:id": {
      requireAdmin();
      const k = findSegment(id).keywords.find((x) => x.id === sub);
      if (!k) throw new DemoError(404, "keyword_not_found", "Palavra-chave não encontrada.");
      const priority = Number(body.priority);
      if (!(priority >= 0 && priority <= 100)) throw invalid({ priority: "Prioridade deve ficar entre 0 e 100." });
      k.priority = priority;
      return undefined;
    }
    case "DELETE /segments/:id/keywords/:id": {
      requireAdmin();
      const s = findSegment(id);
      const i = s.keywords.findIndex((x) => x.id === sub);
      if (i < 0) throw new DemoError(404, "keyword_not_found", "Palavra-chave não encontrada.");
      s.keywords.splice(i, 1);
      return undefined;
    }

    case "GET /tags":
      requireSession();
      return { tags: [...tagsDb].sort((a, b) => a.name.localeCompare(b.name)).map((t) => ({ ...t, inUse: 0 })) };
    case "POST /tags": {
      requireAdmin();
      const name = String(body.name ?? "").trim().replace(/\s+/g, " ").toUpperCase();
      if (!name || name.length > 30) throw invalid({ name: "Informe um nome de até 30 caracteres." });
      if (tagsDb.some((t) => t.name === name)) throw invalid({ name: "Já existe uma etiqueta com esse nome." });
      const t = { id: uid(), name };
      tagsDb.push(t);
      return { ...t, inUse: 0 };
    }
    case "PATCH /tags/:id": {
      requireAdmin();
      const t = tagsDb.find((x) => x.id === id);
      if (!t) throw notFound("Etiqueta");
      const name = String(body.name ?? "").trim().replace(/\s+/g, " ").toUpperCase();
      if (!name || name.length > 30) throw invalid({ name: "Informe um nome de até 30 caracteres." });
      if (tagsDb.some((x) => x.id !== t.id && x.name === name)) throw invalid({ name: "Já existe uma etiqueta com esse nome." });
      t.name = name;
      return { ...t, inUse: 0 };
    }
    case "DELETE /tags/:id": {
      requireAdmin();
      const i = tagsDb.findIndex((x) => x.id === id);
      if (i < 0) throw notFound("Etiqueta");
      tagsDb.splice(i, 1);
      return undefined;
    }

    case "GET /people":
      return {
        people: users
          .filter((u) => u.isActive)
          .map((u) => ({ id: u.id, name: u.name, role: u.role, teams: teamsOf(u).map((t) => t.name) }))
          .sort((a, b) => a.name.localeCompare(b.name)),
      };
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
    case "POST /users/:id/invite": {
      requireAdmin();
      const u = users.find((x) => x.id === id);
      if (!u) throw notFound("Usuário");
      u.mfaEnabled = false;
      return { user: userView(u), inviteCode: "DEMO7-K2M9X", expiresAt: new Date(Date.now() + 72 * 3600_000).toISOString() };
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
      if (segmentsDb.some((s) => s.teamId === id)) {
        throw new DemoError(409, "team_in_use", "Esta equipe está vinculada a segmentos (filas). Desvincule-os em Filas / Segmentos antes de excluir.");
      }
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
