# Lembretes pessoais

Na aba **Minhas**, cada conversa tem **Adicionar lembrete** abaixo da prévia da
mensagem. Clique no lembrete salvo para editar o texto, escolher outra cor ou remover.
O texto pode ter até 240 caracteres. Cores: verde, amarelo, azul, rosa, lilás e neutro.

As anotações ficam no banco, vinculadas ao usuário e ao atendimento. Apenas quem
está responsável pelo atendimento pode criar ou editar seu próprio lembrete.
Administradores não têm acesso aos lembretes de outras pessoas. Uma transferência
não compartilha a anotação com o próximo atendente; se a conversa voltar ao autor,
seu lembrete volta a aparecer. Conversas encerradas não aparecem nessa lista.

O conteúdo não faz parte dos dados compartilhados da conversa, dos eventos em tempo
real ou da auditoria. Um evento privado avisa as outras abas do mesmo usuário para
recarregar os lembretes. Não há envio de mensagens ao cliente.

Rotas autenticadas: `GET /api/v1/conversation-reminders`,
`PUT /api/v1/conversations/{id}/reminder` com `{ "text": "...", "color": "olive" }`
e `DELETE /api/v1/conversations/{id}/reminder`. Alterações exigem CSRF.

Migração: `backend/migrations/00012_conversation_reminders.sql`. A tabela tem RLS
ativado e acesso direto revogado para os papéis públicos; o acesso passa pela API.
