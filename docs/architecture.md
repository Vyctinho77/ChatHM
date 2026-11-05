# Arquitetura

## Visão geral

- **Monólito modular em Go**: um binário (`cmd/api`) com módulos em `internal/`. Sem microsserviços no MVP.
  Cada módulo expõe seu próprio `Routes()` montado em `/api/v1` e acessa o banco apenas pelas suas tabelas;
  comunicação entre módulos via interfaces Go, não via HTTP.
- **Frontend SPA** (`apps/web`) servido separadamente. Em dev, o Vite faz proxy de `/api` para a API,
  mantendo *same-origin* para cookies de sessão. Em produção, um proxy reverso (TLS) serve ambos no mesmo domínio.
- **PostgreSQL** como fonte da verdade, inclusive para jobs persistidos (sem Redis/fila até o volume justificar).
- **S3 compatível** (MinIO em dev) para anexos em bucket privado; acesso apenas via URLs assinadas de curta duração.

## Decisões

| Tema | Decisão | Motivo |
|---|---|---|
| Router HTTP | `go-chi/chi` | idiomático, compatível com `net/http`, middlewares simples |
| Driver PG | `jackc/pgx/v5` (pool) | desempenho, suporte nativo a tipos PG, `SELECT ... FOR UPDATE` |
| Migrações | `pressly/goose`, SQL puro, embutidas no binário | versionadas, reversíveis, sem dependência externa em produção |
| Logs | `log/slog` (texto em dev, JSON em prod) | estruturado, stdlib; nunca registrar corpo de mensagens ou segredos |
| Sessão | cookie HttpOnly/Secure/SameSite=Strict + tabela `sessions` (só o SHA-256 do token) | revogação server-side, expiração absoluta e por inatividade |
| CSRF | token sincronizador por sessão no cabeçalho `X-CSRF-Token` + checagem de `Origin` + exigência de JSON | defesa em camadas, inclusive no login |
| Força bruta | tabela `login_attempts` (por e-mail desde o último sucesso, e por IP) | sobrevive a reinícios, sem Redis |
| MFA | TOTP (RFC 6238) próprio, segredo cifrado, anti-replay por `mfa_last_step`, token rotacionado após verificação | obrigatório para admin fora de dev |
| Senhas | Argon2id | recomendação OWASP |
| Segredos WhatsApp | AES-256-GCM, chave em `ENCRYPTION_KEY` (fora do banco) | material de sessão protegido em repouso |
| WhatsApp | interface `WhatsAppProvider`; 1º adaptador whatsmeow | desacopla do provedor; migração futura para a API oficial |
| Tempo real | WebSocket a partir da API | filas e atribuições refletidas sem polling |

## Módulos (`backend/internal`)

- `auth` — login, logout, sessões, rate limit/brute force, CSRF, MFA de admin.
- `users`, `teams` — perfis `admin`/`atendente`, equipes e membros.
- `audit` — trilha de auditoria sem conteúdo sensível.
- `whatsapp` — `WhatsAppProvider`, estados (desconectado, aguardando QR, conectado, erro), eventos de entrada.
- `conversations` — contatos, conversas, mensagens (dedupe por ID externo), atribuição atômica ao primeiro que responder.
- `segments` — segmentos, palavras-chave com prioridade, normalização (caixa/acentos, palavras completas), fila geral em caso de conflito.
- `storage` — upload/download de anexos, validação de tipo/tamanho, URLs de curta duração.

## Autorização

Toda checagem é feita no Go. Camadas em `internal/httpapi/router.go`:

1. `LoadSession` — resolve o cookie (papel e status lidos do banco a cada requisição).
2. `RequireSession` + `CSRF` — rotas de sessão (`/auth/me`, `logout`, MFA).
3. `RequireMFA` — bloqueia segundo fator pendente e admin sem MFA quando exigido.
4. `RequireAdmin` — rotas administrativas; regras por equipe ficam nos serviços (ex.: `teams.canView`).

Recursos fora do alcance do usuário respondem `404` (não revelam existência).

## Testes

- Unitários: Argon2id, TOTP (vetores da RFC 6238), AES-GCM.
- Integração (`internal/httpapi/api_test.go`): HTTP real contra PostgreSQL real, um banco isolado por teste
  (`TEST_DATABASE_URL`), cobrindo login/logout, CSRF/Origin, força bruta, permissões por perfil e equipe,
  troca/reset de senha, expiração por inatividade, MFA e concorrência na remoção do último admin.

## Limitações conhecidas (Fase 1)

- O bloqueio por e-mail permite que alguém trave temporariamente (15 min) o login de uma conta conhecida.
  Mitigação futura: CAPTCHA ou atraso progressivo em vez de bloqueio.
- Não há códigos de recuperação de MFA; a recuperação é pela CLI (`reset-mfa`), com acesso ao servidor.

## Convenções

- Migrações: `backend/migrations/NNNNN_descricao.sql`, sempre com `-- +goose Down`.
- Autorização sempre validada no backend; o frontend apenas esconde o que o usuário não pode ver.
- Testes ao lado do código (`*_test.go`, `*.test.ts(x)`).
