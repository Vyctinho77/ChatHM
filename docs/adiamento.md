# Adiar atendimento

No menu de três pontos do chat, use **Adiar atendimento**. Escolha **Em 1 hora**,
**Amanhã às 9h** ou uma data e hora. O horário exibido acompanha o fuso do navegador;
a API recebe um instante ISO 8601 com fuso e o PostgreSQL guarda `timestamptz`.

A conversa sai de Minhas, Não atribuídos e Todas e fica no filtro **Adiadas**, ordenada pelo
horário de retorno. O chat continua disponível para leitura. Para responder,
use **Retomar agora**. Também é possível alterar o horário.

- **Só para mim** (padrão) ou **Para todos**: ao adiar, escolha para quem. "Só para mim" tira a conversa
  apenas da sua lista; as outras pessoas continuam vendo e respondendo, e você mesmo ainda pode responder
  (responder tira o adiamento). Qualquer pessoa que vê a conversa pode adiar para si. "Para todos" vale para
  a equipe inteira, bloqueia as respostas até retomar e continua só com o responsável, gestor ou administrador.
  O adiamento para todos tem prioridade sobre o individual.
- Equipe, responsável, status do atendimento, mensagens e etiquetas são preservados.
- Uma mensagem nova do cliente retoma o atendimento imediatamente, inclusive os adiamentos individuais. Importação de
  histórico, mensagens do próprio escritório e eventos duplicados não cancelam o adiamento.
- Nenhuma mensagem é enviada ao cliente ao adiar ou retomar.
- Encerrar um atendimento limpa os adiamentos (geral e individuais); atendimentos encerrados não podem ser adiados.
- Datas precisam estar no futuro, dentro de um ano.

O servidor verifica vencimentos a cada 15 segundos e também ao iniciar, recuperando
retornos que venceram enquanto estava parado. Cada lote processa até 100 conversas
com `FOR UPDATE SKIP LOCKED`, permitindo mais de uma instância sem retomar duas vezes.
As atualizações seguem a visibilidade existente por equipe e são publicadas pelo WebSocket.
As listas têm limites separados de 300 ativas e 300 adiadas. Contadores de filas e de
conversas não lidas na navegação consideram apenas as ativas.

Migração: `backend/migrations/00011_conversation_snooze.sql`. Campos novos:
`snoozed_until` e `snoozed_by`; o status existente não muda. Rotas autenticadas com CSRF:

| Rota | Corpo | Efeito |
|---|---|---|
| `POST /api/v1/conversations/{id}/snooze` | `{ "until": "2026-11-07T09:00:00-04:00", "scope": "me" \| "all" }` | Adia ou altera o horário (`scope` omitido = `all`) |
| `POST /api/v1/conversations/{id}/resume` | vazio ou `{ "scope": "me" \| "all" }` | Retoma agora |

Na conversa, `snoozedUntil` é o adiamento que vale **para quem está vendo** e `snoozedFor` diz se é `"all"` ou `"me"`.
As ferramentas MCP `snooze` e `resume` continuam sendo "para todos".

## Não lidas por pessoa

Cada pessoa tem o seu próprio "lido até": abrir a conversa zera só o contador de quem abriu, e o das demais
continua. Uma mensagem nova conta como não lida para todas as pessoas que veem a conversa. **Responder** ao
cliente atende a conversa para todos e zera todos os contadores. O estado individual fica em
`conversation_user_state` (migração `00037`): `read_through` (mensagens do cliente até esse instante já foram lidas) e
`snoozed_until` (adiamento só dessa pessoa). Sem linha, vale o contador geral da conversa. O WebSocket envia a cada
pessoa a conversa com o contador e o adiamento dela.

Ações registradas na auditoria: `conversation.snooze` e `conversation.resume`.
A interface de demonstração também permite adiar e retomar, com dados em memória.
