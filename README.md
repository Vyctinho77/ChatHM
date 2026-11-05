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
│   │   ├── segments/        # segmentos, palavras-chave, classificador
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
- Docker + Docker Compose

## Primeiros passos

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
3. **Segmentação e filas** — classificador determinístico, fila geral, atribuição atômica.
4. **Chat completo** — layout em painéis, mídias, temas.
5. **Validação e piloto** — E2E, backups, LGPD, operação.
