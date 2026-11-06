# Adiar atendimento

No menu de três pontos do chat, use **Adiar atendimento**. Escolha **Em 1 hora**,
**Amanhã às 9h** ou uma data e hora. O horário exibido acompanha o fuso do navegador;
a API recebe um instante ISO 8601 com fuso e o PostgreSQL guarda `timestamptz`.

A conversa sai de Minhas, Não atribuídos e Todas e fica no filtro **Adiadas**, ordenada pelo
horário de retorno. O chat continua disponível para leitura. Para responder,
use **Retomar agora**. Também é possível alterar o horário.

- Apenas o responsável ou um administrador pode adiar, reagendar ou retomar.
- Equipe, responsável, status do atendimento, mensagens e etiquetas são preservados.
- Uma mensagem nova do cliente retoma o atendimento imediatamente. Importação de
  histórico, mensagens do próprio escritório e eventos duplicados não cancelam o adiamento.
- Nenhuma mensagem é enviada ao cliente ao adiar ou retomar.
- Encerrar um atendimento limpa o adiamento; atendimentos encerrados não podem ser adiados.
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
| `POST /api/v1/conversations/{id}/snooze` | `{ "until": "2026-11-07T09:00:00-04:00" }` | Adia ou altera o horário |
| `POST /api/v1/conversations/{id}/resume` | vazio | Retoma agora |

Ações registradas na auditoria: `conversation.snooze` e `conversation.resume`.
A interface de demonstração também permite adiar e retomar, com dados em memória.
