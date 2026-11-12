# Lembretes pessoais

Na aba **Minhas**, cada conversa tem **Adicionar lembrete** abaixo da prévia da
mensagem. Clique no lembrete salvo para editar o texto, escolher outra cor ou remover.
O texto pode ter até 240 caracteres. Cores: verde, amarelo, azul, rosa, lilás e neutro.

As anotações ficam no banco, vinculadas ao usuário e ao atendimento. Apenas quem
está responsável pelo atendimento pode criar ou editar seu próprio lembrete.
Administradores não têm acesso aos lembretes de outras pessoas. Conversas encerradas
não aparecem nessa lista.

## Transferência

Por padrão, uma transferência não compartilha a anotação com o próximo atendente; se a
conversa voltar ao autor, seu lembrete volta a aparecer. Quem está atendendo e tem um
lembrete na conversa vê, ao escolher a pessoa, o aviso **Levar seu lembrete junto?** com
o texto e três opções: **Transferir com o lembrete**, **Transferir sem o lembrete** e
**Cancelar**. Sem lembrete, a transferência é direta, como sempre.

Com o lembrete, quem recebe ganha uma cópia (mesmo texto e cor) que pode editar à vontade,
e o texto original fica gravado no novo atendimento (`handover_note`, `handover_color`,
`handover_by` em `conversation_assignments`), então o contexto da transferência não se perde
mesmo que o lembrete seja apagado depois. A cópia de quem transferiu continua só dele.

Marcando **Sempre levar meu lembrete e não perguntar de novo**, as próximas transferências
levam o lembrete sem o aviso. A escolha fica no navegador, por conta, e se desfaz em
**Configurações → Transferências**. Cancelar ou transferir sem o lembrete nunca grava essa
preferência.

Só o lembrete de quem está atendendo pode ser compartilhado: um administrador que transfere
a conversa de outra pessoa não lê nem repassa o lembrete dela. A API aceita
`POST /api/v1/conversations/{id}/transfer` com `{ "userId": "...", "shareReminder": true }`;
a transferência pelo MCP não compartilha lembretes. A auditoria registra apenas
`reminderShared` (sim/não), nunca o texto.

O conteúdo não faz parte dos dados compartilhados da conversa, dos eventos em tempo
real ou da auditoria. Um evento privado avisa as outras abas do mesmo usuário para
recarregar os lembretes. Não há envio de mensagens ao cliente.

Rotas autenticadas: `GET /api/v1/conversation-reminders`,
`PUT /api/v1/conversations/{id}/reminder` com `{ "text": "...", "color": "olive" }`
e `DELETE /api/v1/conversations/{id}/reminder`. Alterações exigem CSRF.

Migrações: `backend/migrations/00012_conversation_reminders.sql` e `00033_transfer_reminder.sql` (registro do repasse). A tabela tem RLS
ativado e acesso direto revogado para os papéis públicos; o acesso passa pela API.
