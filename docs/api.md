# API (Fases 1 a 4)

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

O cadastro por conta própria (`POST /auth/register`) vem **desligado** (`ALLOW_SELF_REGISTER=false`; responde `404 registration_disabled`).
Ligado, cria um atendente **inativo**, sem equipe e **sem sessão** (`202 {pending: true}`): a pessoa só consegue entrar
depois que um administrador ativar a conta em Equipe. Não aceita perfil nem equipes no corpo. Exige senha de 12 a 128
caracteres e permite até cinco tentativas por IP a cada 15 minutos. `GET /auth/options` (público) informa `{selfRegister}`
para a tela de entrada mostrar ou esconder "Solicitar acesso". O caminho normal é o administrador criar o acesso (`POST /users`).
As permissões existentes de atendente se aplicam imediatamente, incluindo filas
gerais ou sem equipe; o cadastro não cria um escritório separado.

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
| POST | `/auth/register` | público (se habilitado) | `{name, email, password}` | `202 {pending}` · `404 registration_disabled` · `409 email_unavailable` · `422 validation_failed` · `429 too_many_attempts` |
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

## WhatsApp (Fase 2)

| Método | Rota | Acesso | Resposta |
|---|---|---|---|
| GET | `/whatsapp/connection` | completo | `200 Connection` (o QR só vem para admin) |
| POST | `/whatsapp/connect` | admin | `202 Connection` · `409 already_connected` · `502 provider_error` |
| POST | `/whatsapp/disconnect` | admin | `200 Connection` |
| POST | `/dev/whatsapp/inbound` | admin, só com provedor `fake` em development | `204` |

```ts
type Connection = {
  state: "disconnected" | "pending_qr" | "connected" | "error";
  phone: string; displayName: string; lastError?: string; connectedAt?: string;
  qr?: { code: string; expiresAt: string } | null; // somente admin
  simulator?: boolean;                               // somente admin
};
```

## Conversas

Todas as rotas respeitam a visibilidade por equipe ([`queues.md`](queues.md)); o que o usuário não pode ver responde `404`.

| Método | Rota | Corpo | Resposta |
|---|---|---|---|
| GET | `/conversations` | — | `200 {conversations: Conversation[]}` (até 300, mais recentes primeiro) |
| GET | `/conversations/{id}` | — | `200 Conversation` · `404` |
| GET | `/conversations/{id}/messages` | — | `200 {messages: Message[]}` (até 300 mais recentes, em ordem cronológica) |
| POST | `/conversations/{id}/messages` | `{id?: uuid, text}` | `201 Message` (nova) · `200 Message` (repetição) · `409 conversation_taken` · `409 conversation_closed` · `409 id_conflict` · `422` · `503 whatsapp_unavailable` |
| POST | `/conversations/{id}/read` | — | `204` |
| POST | `/conversations/{id}/close` | — | `204` (responsável ou admin; idempotente) · `403` |
| POST | `/conversations/{id}/transfer` | `{userId}` | `204` (responsável, admin ou conversa sem responsável) · `409 conversation_taken` · `409 conversation_closed` · `422` (pessoa inexistente/desativada) |
| GET | `/people` | — | `200 {people: {id, name, role, teams: string[]}[]}` — usuários ativos (qualquer autenticado; sem e-mail) |
| PUT | `/conversations/{id}/tags/{tagId}` | — | `204` (idempotente) · `404 tag_not_found` |
| DELETE | `/conversations/{id}/tags/{tagId}` | — | `204` |
| PATCH | `/contacts/{id}` | `{name?, company?, note?}` | `204` · `422` |

Conversas **encerradas não aparecem** em `GET /conversations` (o detalhe por id continua funcionando) e o servidor
publica `conversation.removed`. Quando o cliente volta, abre-se um novo atendimento (reclassificado). `GET .../messages`
devolve o histórico de todos os atendimentos do mesmo contato que o usuário pode ver; cada mensagem traz seu
`conversationId` (a interface mostra "Novo atendimento" na troca). Transferir encerra a atribuição atual com motivo
`transferred`, atribui à pessoa escolhida (status `in_progress`) e registra auditoria `conversation.transfer`.

### Anexos e localização (Fase 4)

`POST /conversations/{id}/messages` aceita, além de JSON de texto:

- **Localização** (JSON): `{id?, kind: "location", lat, lng, name?, address?}`.
- **Anexo** (`multipart/form-data`): campos `id?`, `kind` (`image` \| `audio` \| `document`), `text` (legenda; não vale para áudio),
  `hd` (`true`: imagem original), `waveform` (JSON, 0–1) e o arquivo em `file`. `422` com `fields.file` (tipo/conteúdo
  inválido, vazio, executável, antivírus), `413` acima do limite (`MAX_ATTACHMENT_MB`, padrão 25), `503` se o WhatsApp está desconectado.

O tipo vem do **conteúdo**, não do nome. Imagens (JPEG, PNG, WebP) sem `hd` são reduzidas a 1600 px; com `hd` seguem
originais. Áudio de qualquer formato que o ffmpeg leia é convertido para Ogg/Opus (mensagem de voz). Reenviar com o mesmo `id`
(mensagem `failed`) tenta de novo **relendo o arquivo guardado** — não duplica.

`GET /attachments/{id}`: entrega o arquivo só se o usuário pode ver a conversa (`404` caso contrário; `401` sem sessão),
com `Cache-Control: private, no-store`, `nosniff` e CSP `sandbox`. Imagens e áudios são `inline`; o resto vai como
download (`application/octet-stream`). Não há URL pública nem assinada: a autorização é refeita a cada requisição.
A mensagem traz `url` quando tem arquivo. Mídia **recebida** é baixada em segundo plano: enquanto isso a mensagem
vem sem `url`; se falhar, `mediaState: "failed"` (ou `"blocked"` se não passou na verificação), e o WebSocket envia `message.updated`.

`POST .../messages` **assume o atendimento** quando a conversa não tem responsável. Envie o `id` (UUID gerado no
cliente): repetir a requisição não duplica. A resposta pode trazer `status: "failed"` se o WhatsApp recusou; reenviar
com o mesmo `id` tenta de novo.

`Conversation` e `Message` seguem os tipos de `apps/web/src/features/conversations/types.ts`. Campos novos:
`segment` (chave; `"geral"` = fila geral), `segmentName`, `classification` (`keyword` | `conflict` | `none`),
`tags` (nomes) e, nas mensagens enviadas, `deliveredAt`, `readAt`, `authorName`.

## Segmentos (filas)

| Método | Rota | Acesso | Corpo | Resposta |
|---|---|---|---|---|
| GET | `/segments` | completo | — | `200 {segments}` (palavras-chave só para admin) |
| POST | `/segments` | admin | `{name, teamId?}` | `201 Segment` · `422` |
| PATCH | `/segments/{id}` | admin | `{name?, isActive?}` | `200 Segment` |
| PUT | `/segments/{id}/team` | admin | `{teamId: uuid \| null}` | `200 Segment` (null = sem equipe, visível a todos) |
| DELETE | `/segments/{id}` | admin | — | `204` (conversas voltam à fila geral) |
| POST | `/segments/{id}/keywords` | admin | `{phrase, priority?}` | `201 Keyword` · `422` (vazia, só palavras de ligação, duplicada, prioridade fora de 0–100) |
| PATCH | `/segments/{id}/keywords/{kid}` | admin | `{priority}` | `204` |
| DELETE | `/segments/{id}/keywords/{kid}` | admin | — | `204` |
| POST | `/segments/classify` | admin | `{text}` | `200 {segmentKey?, reason, matches[]}` |

```ts
type Segment = { id: string; key: string; name: string; teamId: string | null; teamName?: string;
                 isActive: boolean; keywords?: { id: string; phrase: string; priority: number }[] };
```

Excluir uma equipe vinculada a segmentos responde `409 team_in_use`.

## Etiquetas

| Método | Rota | Acesso | Corpo | Resposta |
|---|---|---|---|---|
| GET | `/tags` | completo | — | `200 {tags: {id, name, inUse}[]}` |
| POST | `/tags` | admin | `{name}` (vira MAIÚSCULAS) | `201` · `422` (duplicada) |
| PATCH | `/tags/{id}` | admin | `{name}` | `200` |
| DELETE | `/tags/{id}` | admin | — | `204` |

## Tempo real

`GET /api/v1/ws` (WebSocket; exige sessão completa e origem permitida). Mensagens do servidor:

```ts
{ type: "conversation.updated", data: Conversation }   // só para quem pode ver a conversa
{ type: "conversation.removed", data: { id: string } } // perdeu o acesso: remover da lista
{ type: "message.created",      data: Message }
{ type: "message.updated",      data: Message }        // recibos e falhas de envio
{ type: "whatsapp.status",      data: Omit<Connection, "qr" | "simulator"> }
{ type: "whatsapp.qr",          data: { code, expiresAt } | null }   // somente admin
```

A sessão é revalidada a cada minuto; logout ou desativação encerram a conexão (código `4401`).
O cliente reconecta com espera exponencial e, ao reconectar, recarrega a lista.
