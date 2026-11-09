# ChatHM

Central web de atendimento para escritórios contábeis, conectada ao WhatsApp, com
classificação automática de mensagens e distribuição por equipe.

> MVP de um escritório, com arquitetura preparada para evolução.
> Blueprint completo: [`docs/blueprint.md`](docs/blueprint.md) · Arquitetura: [`docs/architecture.md`](docs/architecture.md) · API: [`docs/api.md`](docs/api.md) · Frontend: [`docs/frontend.md`](docs/frontend.md)

## Estrutura

```
.
├── apps/
│   └── web/                 # React + TypeScript + Vite + Tailwind + TanStack Query
├── backend/                 # Monólito modular em Go (chi + pgx)
│   ├── cmd/
│   │   ├── api/             # servidor HTTP/WebSocket
│   │   └── migrate/         # CLI de migrações (goose)
│   ├── internal/
│   │   ├── app/             # montagem dos serviços
│   │   ├── config/          # variáveis de ambiente
│   │   ├── httpapi/         # roteador, middlewares transversais
│   │   ├── platform/        # database, httpx, secretbox (AES-GCM), testdb
│   │   ├── auth/            # login, sessões, Argon2id, CSRF, MFA
│   │   ├── users/  teams/   # usuários, perfis, equipes
│   │   ├── audit/           # audit_logs
│   │   ├── conversations/   # contatos, conversas, mensagens, atribuição
│   │   ├── segments/        # segmentos, palavras-chave, classificador determinístico
│   │   ├── tags/            # catálogo de etiquetas
│   │   ├── realtime/        # WebSocket (eventos para o navegador)
│   │   ├── whatsapp/        # WhatsAppProvider, Manager, adaptadores wameow (whatsmeow) e fake
│   │   └── storage/         # anexos em S3 privado
│   └── migrations/          # SQL versionado (embutido no binário)
├── docs/
├── docker-compose.yml       # Postgres + MinIO (dev)
└── Makefile
```

## Pré-requisitos

- Go 1.26+
- Node 22+ e pnpm 10 (`corepack enable`)
- Windows: o modo de desenvolvimento prepara PostgreSQL local automaticamente.
- Linux/macOS: Docker + Docker Compose, ou PostgreSQL local.

## Primeiros passos

No Windows, `npm run dev` prepara o PostgreSQL local e inicia o app sem Docker.
O fluxo `make` abaixo é a alternativa baseada em Docker.

```bash
make setup        # pnpm install, go mod download, cria .env com ENCRYPTION_KEY
make up           # Postgres (5432) e MinIO (9000 / console 9001)
make create-admin email=admin@escritorio.com.br name="Admin"   # pede a senha (mín. 12)
make api          # API em http://localhost:8080 (AUTO_MIGRATE=true aplica migrações)
                  # sem celular? WHATSAPP_PROVIDER=fake no .env simula conexão e mensagens
make web          # Frontend em http://localhost:5173 (proxy /api → :8080)
```

Verificação rápida: `curl localhost:8080/healthz` e `curl localhost:8080/readyz`.

## Comandos

Com as dependências instaladas, execute na raiz para iniciar o banco de
desenvolvimento, o backend e o frontend:

```bash
npm run dev
```

A API fica em http://localhost:8080 e o frontend em http://localhost:5173.
O frontend aguarda a API e o banco ficarem prontos antes de iniciar, evitando
erros de proxy durante a compilação inicial do Go.
O comando usa **`.env.development.local`**, criado a partir de
`.env.development.example`, com uma chave de criptografia própria. Ele não carrega
o `.env` de produção e recusa bancos remotos ou bancos sem o sufixo `_dev`/`_test`.
Mesmo se o terminal tiver `DATABASE_URL` de produção, o processo de desenvolvimento
usa o endereço local do arquivo de desenvolvimento.

O banco é `chathm_dev`, em `127.0.0.1:5433`. No Windows sem Docker, os binários
PostgreSQL 17.11 são baixados do distribuidor oficial EDB e os dados ficam em
`.local/pgdata-17`, sem serviço global ou privilégios de administrador. Com Docker,
é usado `docker-compose.dev.yml`, com volume exclusivo. Os dados persistem ao parar
e iniciar novamente. `npm run db:up` inicia o banco e `npm run db:down` apenas o para.

O WhatsApp é **simulado** por padrão e os anexos ficam em `.local/attachments`.
Assim, desenvolver não reconecta a conta do WhatsApp usada no deploy, nem usa os
arquivos, usuários ou migrações do Supabase em produção. O banco local começa
vazio e recebe suas próprias migrações; configure o escritório pela tela inicial.
As configurações e arquivos em `deploy/` continuam destinados ao servidor publicado.
Use `Ctrl+C` para encerrar a API e o frontend.

O login fica salvo neste navegador por até **30 dias**, inclusive ao fechá-lo.
O app restaura a sessão pelo cookie HttpOnly, sem guardar senha ou código do
autenticador. “Sair”, expiração e revogação pelo administrador encerram o acesso.
`SESSION_TTL=720h` e `SESSION_IDLE_TIMEOUT=720h` controlam a duração; ambientes
com valores antigos no `.env` precisam atualizar essas variáveis e reiniciar a API.
Sessões já emitidas mantêm a validade anterior até um novo login.
A sessão **renova com o uso**: quem abre o app ao menos uma vez a cada 30 dias não precisa entrar de novo, até o teto de
`SESSION_MAX_LIFETIME=2160h` (90 dias) desde o login. Se o navegador estiver configurado para apagar cookies ao fechar, o login
salvo não sobrevive a isso (é uma configuração do navegador, não do ChatHM).

| Comando | O que faz |
|---|---|
| `make test` | Testes Go (`-race`, integração no Postgres do compose) e web (Vitest) |
| `make lint` | `go vet`, ESLint, typecheck |
| `make migrate-up` / `migrate-down` / `migrate-status` | Migrações |
| `make migrate-new name=create_users` | Nova migração SQL |
| `make build` | Binários Go e bundle web |
| `docker compose --profile full up` | Sobe também a API containerizada |

## Roadmap

1. ✅ **Fundação e acesso seguro** — login, sessões, perfis, equipes, MFA de admin (backend).
2. ✅ **WhatsApp** — QR Code, sessão criptografada, recepção de mensagens em tempo real ([`docs/whatsapp.md`](docs/whatsapp.md)).
3. ✅ **Segmentação e filas** — classificador determinístico, filas por equipe, atribuição atômica, etiquetas
   ([`docs/queues.md`](docs/queues.md)).
4. **Chat completo** — layout em painéis, mídias, temas.
5. **Validação e piloto** — E2E, backups, LGPD, operação.
