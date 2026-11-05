# API — Fase 1 (autenticação, usuários e equipes)

Base: `/api/v1`. Todas as respostas são JSON. Erros seguem o formato:

```json
{ "error": { "code": "validation_failed", "message": "Dados inválidos.", "fields": { "email": "E-mail inválido." } } }
```

## Sessão e segurança (contrato com o frontend)

- **Cookie de sessão** `chathm_session`: `HttpOnly`, `SameSite=Strict`, `Secure` em produção. O frontend não lê nem manipula o cookie; basta usar `credentials: "include"` (ou mesma origem).
- **CSRF**: `login`, `me`, `mfa/verify` e `mfa/enable` devolvem `csrfToken`. Toda requisição `POST/PUT/PATCH/DELETE` autenticada deve enviar o cabeçalho `X-CSRF-Token: <csrfToken>`. Guarde o token em memória e atualize-o a cada resposta que o trouxer (ele muda após o MFA).
- **Origin**: requisições que alteram estado precisam vir de uma origem em `ALLOWED_ORIGINS`.
- **Content-Type**: corpos devem ser `application/json` (caso contrário, `415`).
- **Expiração**: sessão expira após `SESSION_TTL` (padrão 12h) ou `SESSION_IDLE_TIMEOUT` sem uso (padrão 2h). Qualquer `401 unauthenticated` → redirecionar para o login.

### Estados após o login

| `mfaRequired` | `mfaSetupRequired` | O que a interface deve fazer |
|---|---|---|
| `false` | `false` | Entrar no painel. |
| `true` | — | Pedir o código de 6 dígitos → `POST /auth/mfa/verify`. |
| `false` | `true` | Admin sem MFA (política exige): `POST /auth/mfa/setup` → exibir QR do `otpauthUrl` → `POST /auth/mfa/enable`. |

Enquanto um desses estados estiver pendente, as demais rotas respondem `403` com `mfa_required` ou `mfa_setup_required`.

## Autenticação

| Método | Rota | Acesso | Corpo | Resposta |
|---|---|---|---|---|
| POST | `/auth/login` | público | `{email, password}` | `200 Me` · `401 invalid_credentials` · `429 too_many_attempts` |
| GET | `/auth/me` | sessão | — | `200 Me` |
| POST | `/auth/logout` | sessão | — | `204` |
| POST | `/auth/mfa/setup` | sessão | — | `200 {secret, otpauthUrl}` · `409 mfa_already_enabled` |
| POST | `/auth/mfa/enable` | sessão | `{code}` | `200 Me` · `422 invalid_code` |
| POST | `/auth/mfa/verify` | sessão | `{code}` | `200 Me` (cookie e CSRF rotacionados) · `422 invalid_code` · `429` após 5 erros (sessão encerrada) |
| POST | `/auth/password` | completo | `{currentPassword, newPassword}` | `204` (outras sessões encerradas) · `422` |

```ts
type Me = {
  user: { id: string; email: string; name: string; role: "admin" | "agent"; theme: "light" | "dark" | "system"; mfaEnabled: boolean };
  csrfToken: string;
  mfaRequired: boolean;
  mfaSetupRequired: boolean;
};
```

Limites contra força bruta: 5 falhas por e-mail (desde o último sucesso) ou 30 por IP em 15 min → `429` com `Retry-After`.

## Preferências do usuário

| Método | Rota | Acesso | Corpo | Resposta |
|---|---|---|---|---|
| PATCH | `/me/preferences` | completo | `{theme: "light" \| "dark" \| "system"}` | `200 {theme}` |

## Usuários (somente admin)

| Método | Rota | Corpo | Resposta |
|---|---|---|---|
| GET | `/users` | — | `200 {users: User[]}` |
| POST | `/users` | `{email, name, password, role, teamIds?}` | `201 User` · `422` (e-mail duplicado, senha fraca, equipe inexistente) |
| GET | `/users/{id}` | — | `200 User` · `404` |
| PATCH | `/users/{id}` | `{name?, role?, isActive?}` | `200 User` · `409 last_admin` · `409 self_deactivate` |
| POST | `/users/{id}/password` | `{password}` | `204` (sessões do usuário encerradas) |

```ts
type User = {
  id: string; email: string; name: string; role: "admin" | "agent";
  isActive: boolean; mfaEnabled: boolean; lastLoginAt: string | null; createdAt: string;
  teams: { id: string; name: string }[];
};
```

Desativar um usuário encerra todas as sessões dele na hora. Mudanças de perfil valem já na próxima requisição.

## Equipes

Leitura: admin vê todas; atendente vê apenas as equipes de que participa (as outras respondem `404`). Escrita: só admin.

| Método | Rota | Acesso | Corpo | Resposta |
|---|---|---|---|---|
| GET | `/teams` | completo | — | `200 {teams: Team[]}` |
| GET | `/teams/{id}` | membro/admin | — | `200 Team` |
| GET | `/teams/{id}/members` | membro/admin | — | `200 {members: Member[]}` |
| POST | `/teams` | admin | `{name, description?}` | `201 Team` · `422` (nome duplicado, sem diferenciar maiúsculas) |
| PATCH | `/teams/{id}` | admin | `{name?, description?}` | `200 Team` |
| DELETE | `/teams/{id}` | admin | — | `204` |
| PUT | `/teams/{id}/members/{userId}` | admin | — | `204` (idempotente) |
| DELETE | `/teams/{id}/members/{userId}` | admin | — | `204` |

```ts
type Team = { id: string; name: string; description: string; memberCount: number; createdAt: string };
type Member = { userId: string; name: string; email: string; role: "admin" | "agent"; isActive: boolean; joinedAt: string };
```

## CLI de administração

```bash
make create-admin email=admin@escritorio.com.br name="Nome do Admin"
cd backend && go run ./cmd/admin reset-password -email usuario@escritorio.com.br
cd backend && go run ./cmd/admin reset-mfa -email admin@escritorio.com.br   # recuperação se perder o celular
```
