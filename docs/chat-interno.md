# Chat interno

Um grupo do escritório, no estilo do WhatsApp, para a equipe tirar dúvidas. Nada aqui passa pelo
WhatsApp, aparece para clientes, entra nas ferramentas do MCP ou alimenta as sugestões da IA.

## Na tela

- **Onde fica:** a aba **Chat interno** está no menu lateral, logo abaixo de **Envios automáticos** (que só o
  administrador vê; para os demais perfis ela vem depois de **MCP**). Todos os perfis têm a aba.
- **Como abre:** ao clicar, o chat ocupa o painel onde as conversas normais aparecem, e a lista de contatos continua
  à vista ao lado. Ao sair (Conversas, ou escolher um contato), o painel volta ao estado vazio de sempre; ao voltar,
  o histórico está lá. Em telas pequenas, ele ocupa a tela toda, com um botão de voltar.
- **Quem escreveu:** acima da primeira mensagem de cada sequência aparecem o nome (com uma cor própria da pessoa), o
  perfil (Administrador, Gestor ou Atendente) e a equipe. Pessoas desativadas aparecem como "(desativado)". O cabeçalho
  mostra quantas pessoas há no escritório, e clicar nele lista todo mundo com perfil e equipes.
- **Notificação:** a aba mostra um selo com o número de mensagens não lidas (99+ acima disso), atualizado ao vivo, e o
  leitor de tela o anuncia ("Chat interno: 3 mensagens não lidas"). Com o chat aberto e a rolagem no fim, o que
  chega é marcado como lido; lendo mais acima, aparece o aviso "N novas mensagens" para descer.
- **Escrever:** o mesmo campo do chat principal: texto, emojis, fotos, documentos (também arrastando ou colando),
  mensagem de voz e localização, com o rascunho guardado. Clique direito (ou toque longo) numa mensagem abre
  **Responder**, **Copiar**, **Baixar** (arquivos), **Editar** (as suas, até 24 horas) e **Apagar** (as suas; gestor e
  administrador apagam qualquer uma). Não há tiques de entrega/leitura, encaminhar nem "dados da mensagem", que só fazem
  sentido no WhatsApp. Mensagem não enviada mostra "Tentar de novo".
- **Histórico:** abre nas 50 mensagens mais recentes; as anteriores carregam ao rolar para cima.
- **Demonstração:** a versão de demonstração tem uma sala de exemplo só de texto (sem anexos).

## Regras do servidor

- **Quem usa:** toda pessoa autenticada (com a verificação em duas etapas concluída), de qualquer perfil.
- **Uma sala só**, "Escritório". O banco já guarda o tipo da sala (`internal_rooms.kind`), então salas por equipe
  podem vir depois sem mudar as mensagens.
- **Quem escreveu:** cada mensagem traz `authorId`, `authorName`, `authorRole` (`admin`, `manager` ou `agent`) e
  `authorActive`. Os dados são lidos na hora, não gravados na mensagem: se o perfil ou o nome mudar, a conversa toda
  acompanha, e uma pessoa desativada aparece como inativa.
- **Tipos:** texto (até 4000 caracteres), imagem, áudio, documento e localização, com resposta citada (`replyTo`).
  Anexos passam pelo mesmo processamento e limite de tamanho do chat principal (bloqueio de executáveis, verificação,
  conversão de áudio) e ficam no mesmo armazenamento privado, sem URL pública. Não há sticker.
- **Editar:** só a autora, só texto, até 24 horas depois do envio. A mensagem ganha `editedAt`.
- **Apagar:** a autora apaga a própria; gestor e administrador apagam a de qualquer pessoa. O conteúdo e o anexo somem
  (fica a marca `deleted`), e quem apaga a mensagem de outra pessoa gera um registro de auditoria
  `internal_chat.message.moderate` com a autora, nunca com o texto. Uma mensagem citada que foi apagada aparece como apagada.
- **Não lidas:** cada pessoa tem um marcador (`internal_reads.last_read_seq`). Só contam mensagens de outras pessoas
  que não foram apagadas. Quem entra no escritório, ou abre o chat pela primeira vez, começa sem não lidas: o histórico
  anterior não vira selo.

## API

Rotas autenticadas, com CSRF nas alterações.

| Rota | O que faz |
|---|---|
| `GET /api/v1/internal-chat/room` | Sala e estado de leitura: `{ id, name, unread, lastReadSeq, lastSeq }` |
| `GET /api/v1/internal-chat/messages?limit=&before=` | Até `limit` mensagens (padrão 50, máximo 100) em ordem cronológica; sem `before` as mais recentes, com `before=<seq>` as anteriores. `hasMore` indica mensagens mais antigas |
| `POST /api/v1/internal-chat/messages` | JSON `{ id?, text, replyTo? }` ou `{ kind: "location", lat, lng, name?, address? }`; anexo em `multipart/form-data` (`id`, `kind`, `text`, `hd`, `waveform`, `replyTo`, `file`). 201 nova, 200 repetição do mesmo `id` (idempotente), 409 se o `id` é de outra pessoa |
| `PATCH /api/v1/internal-chat/messages/{id}` | `{ text }` |
| `DELETE /api/v1/internal-chat/messages/{id}` | Apaga para todos (204) |
| `POST /api/v1/internal-chat/read` | Marca tudo como lido e devolve a sala |
| `GET /api/v1/internal-chat/attachments/{id}` | Arquivo do anexo; sessão conferida a cada pedido |

As mensagens vêm no mesmo formato "achatado" do chat principal (`id`, `kind`, `text`/`caption`, `url`, `quoted`,
`sentAt`…), com `seq` (ordem e cursor) e sem `direction`: como o mesmo evento vai a todas as sessões, quem desenha
decide se a mensagem é sua comparando `authorId`. `conversationId` é o id da sala.

## Tempo real

| Evento | Quem recebe | Conteúdo |
|---|---|---|
| `internal.message.created` | todas as sessões | a mensagem |
| `internal.message.updated` | todas as sessões | a mensagem editada ou apagada |
| `internal.read` | só as abas da pessoa que leu | `{ unread, lastReadSeq }` |

## Dados

Migração `backend/migrations/00034_internal_chat.sql`: `internal_rooms`, `internal_messages` (com `seq` único e
crescente), `internal_attachments` e `internal_reads`. Todas com RLS ativado e acesso direto revogado dos papéis
públicos; o acesso passa pela API.
