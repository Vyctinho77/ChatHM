# Perfis de acesso

Três perfis, guardados em `users.role` (`admin`, `manager`, `agent`). O perfil é lido do banco a cada requisição: promover ou rebaixar
vale na hora, sem novo login. Só o administrador muda perfis (Equipe → Pessoas).

## O que cada perfil faz

| | Atendente | Gestor | Administrador |
|---|:-:|:-:|:-:|
| Ver conversas das filas das equipes de que participa (e as sem equipe) | ✔ | ✔ | ✔ (todas) |
| Atender, responder, etiquetar, agenda e lembretes | ✔ | ✔ | ✔ |
| Transferir / encerrar para todos / adiar o atendimento **de outra pessoa** (que ele já enxerga) | — | ✔ | ✔ |
| Encerrar a **própria parte** de um atendimento em que está (responsável ou participante) | ✔ | ✔ | ✔ |
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

## Cor do nome das pessoas (11/2026)

O nome de quem atende aparece **colorido, só o texto, sem balão**: na lista de conversas ("Heliton + você"), no cabeçalho da conversa, no painel do contato, no aviso "Em atendimento por…",
no chat interno e na lista de pessoas do escritório. A cor é **da pessoa, não do perfil**: administrador, gestor e atendente usam a mesma paleta.

- **Paleta:** 12 cores (verde, azul, roxo, rosa, laranja, âmbar, ciano, vermelho, lima, índigo, marrom, grafite). Cada uma tem um tom para o tema claro e outro para o escuro
  (`apps/web/src/index.css`, variáveis `--pc-<cor>`), todos com **contraste de pelo menos 4,6:1** sobre o painel, a lista selecionada e o balão. No balão recebido (claro nos dois temas) vale o tom `-ink`.
- **Quem escolhe:** o **administrador**, em *Equipe → ⋮ → Editar pessoa → Cor do nome*. Sem escolha, vale uma cor **automática**, fixa para a mesma pessoa (calculada pelo id).
  Todos veem a escolha (vem em `/people`); quem não é administrador não altera nem a própria cor.
- **Guardado como chave** (`users.color`, migração `00040`; validada em `backend/internal/users/color.go`), nunca como código de cor, para o app ajustar o tom ao tema.
- **Não colorido:** o nome do atendente dentro do balão **enviado** (o fundo desse balão é personalizável por cada pessoa, e uma cor fixa poderia ficar ilegível).

