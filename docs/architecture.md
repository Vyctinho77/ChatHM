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
| Sessão | cookie HttpOnly/Secure/SameSite + tabela `sessions` | revogação server-side; CSRF via token de cabeçalho |
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

## Convenções

- Migrações: `backend/migrations/NNNNN_descricao.sql`, sempre com `-- +goose Down`.
- Autorização sempre validada no backend; o frontend apenas esconde o que o usuário não pode ver.
- Testes ao lado do código (`*_test.go`, `*.test.ts(x)`).
