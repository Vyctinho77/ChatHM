# Agenda

A agenda está disponível em `/agenda` no app normal. Compromissos e prazos são
persistidos em `agenda_items`, no PostgreSQL/Supabase. A demonstração continua
usando apenas seus exemplos em memória.

Cada item pertence a uma equipe. Administradores podem consultar e alterar todos
os itens; atendentes podem consultar, criar, editar, concluir, reabrir e excluir
somente itens das equipes de que participam. Trocar a equipe exige acesso à
equipe original e à nova. Uma equipe com itens vinculados não pode ser excluída.

O cliente vinculado é opcional e aponta para uma conversa existente. O backend
valida o acesso à conversa e oculta esse vínculo de quem perdeu acesso ao
atendimento. Nomes de clientes não são aceitos como fonte de autoridade.

## API

Todas as rotas exigem sessão completa e as alterações exigem CSRF.

| Método | Rota | Resultado |
|---|---|---|
| GET | `/api/v1/agenda` | `{items: [...]}` filtrado pelas equipes |
| PUT | `/api/v1/agenda/{uuid}` | Cria ou substitui o item; retorna o item salvo |
| PATCH | `/api/v1/agenda/{uuid}/done` | `{done: true/false}`; retorna 204 |
| DELETE | `/api/v1/agenda/{uuid}` | Exclui o item; retorna 204 |

O corpo do PUT contém `kind` (`appointment` ou `deadline`), `title` (até 120
caracteres), `date` (`YYYY-MM-DD`), `time` opcional (`HH:MM`), `teamId`,
`conversationId` opcional, `note` (até 2000 caracteres) e `done`.
Prazos não têm horário. Datas e horários são valores locais de calendário,
sem conversão automática de fuso. O UUID é criado pelo cliente para permitir
repetição do salvamento sem duplicar o item.

A lista é atualizada após alterações e a cada 15 segundos para incorporar
mudanças de outras pessoas. A tabela tem RLS e não concede acesso à API pública
do Supabase; a autorização por equipe ocorre no backend Go. Alterações são
registradas em `audit_logs`.
