# Perfis de acesso

Três perfis, guardados em `users.role` (`admin`, `manager`, `agent`). O perfil é lido do banco a cada requisição: promover ou rebaixar
vale na hora, sem novo login. Só o administrador muda perfis (Equipe → Pessoas).

## O que cada perfil faz

| | Atendente | Gestor | Administrador |
|---|:-:|:-:|:-:|
| Ver conversas das filas das equipes de que participa (e as sem equipe) | ✔ | ✔ | ✔ (todas) |
| Atender, responder, etiquetar, agenda e lembretes | ✔ | ✔ | ✔ |
| Transferir / encerrar / adiar o atendimento **de outra pessoa** (que ele já enxerga) | — | ✔ | ✔ |
| Assumir uma conversa de colega (transferir para si) | — | ✔ | ✔ |
| Adicionar e remover pessoas **das equipes de que participa** | — | ✔ | ✔ (qualquer equipe) |
| Criar, renomear e excluir equipes | — | — | ✔ |
| Convidar, criar, editar e desativar pessoas; mudar perfis | — | — | ✔ |
| Filas/segmentos, etiquetas, conexão do WhatsApp, instância e integrações de IA | — | — | ✔ |

Regras que não mudam com o gestor:

- **Visibilidade é pela equipe**, igual à do atendente. Gestor de outra equipe não vê nem age em conversa alheia (responde 404, sem
  revelar que ela existe). Só o administrador vê tudo.
- **Ninguém fala em nome de outra pessoa**: para responder a uma conversa que está com um colega, o gestor primeiro a assume (transfere
  para si). Enquanto ela estiver com outro, o envio responde `conversation_taken`.
- O gestor não se promove a administrador nem altera perfis; a proteção da última administradora continua valendo.
- Toda ação de supervisão entra na auditoria com o gestor como autor (`conversation.transfer`, `team.member.add|remove`...).
- Gestor não é obrigado a ter MFA (a exigência `REQUIRE_ADMIN_MFA` vale só para administradores).

## MCP / IA

Um token de IA age como a pessoa dona dele. Token de gestor com escopo de alterações pode transferir, adiar e encerrar conversas de
colegas da equipe, como o gestor faria no app; sem esse escopo, só lê. As opções de privacidade e as chaves do administrador valem
igual para todos os perfis.

## Código

- Backend: `auth.RoleManager`, `Session.CanSupervise()`, `conversations.Viewer.Supervisor` (permissão de agir; **não** amplia a
  visibilidade, que continua em `visibleExpr`), `auth.RequireSupervisor`, `teams.canManageMembers`. Migração `00024_role_manager.sql`.
- Frontend: `canSupervise(role)` em `features/auth/api.ts`; `Equipe` mostra ao gestor só as equipes dele, com adicionar/remover pessoas.
- Testes: `backend/internal/httpapi/manager_test.go`.
