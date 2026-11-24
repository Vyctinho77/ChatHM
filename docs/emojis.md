# Reações e emojis

Status: **reações prontas** (migração 46). A aba "Figurinhas" com os ícones próprios ainda não existe (veja "Próximos passos").

## 1. Reações às mensagens

Como no WhatsApp: botão direito numa mensagem → barra com 👍 ❤️ 😂 😮 😢 🙏 e "+" (cartela completa). Também há um botãozinho ao lado do balão ao passar o mouse
(no toque, o toque longo abre a mesma barra). Os chips ficam embaixo do balão; clicar no chip da sua reação a remove. Escolher o mesmo emoji de novo remove; outro troca.

- **Uma reação por conta.** O escritório tem uma conta só no WhatsApp: todas as reações dele (feitas aqui ou no celular) são uma só por mensagem (`reactor = 'office'`).
  Quem reagiu pelo ChatHM fica em `message_reactions.user_id`. O cliente (ou cada integrante de um grupo) também tem uma.
- **Tabela** `message_reactions (message_id, reactor, emoji, user_id, ...)`, chave (mensagem, reactor), apaga junto com a mensagem.
- **API** `PUT /api/v1/messages/{id}/reaction {"emoji": "👍"}` (emoji vazio remove). Devolve a mensagem, que traz `reactions: [{emoji, count, mine}]`.
  A mesma lista vai em tempo real (`message.updated`) e na lista de mensagens.
- **Quem pode reagir:** a mesma regra de responder (conversa aberta e sem outra pessoa atendendo). Mensagem apagada ou ainda não enviada não recebe reação.
- **Envio:** `Provider.SendReaction` (whatsmeow `BuildReaction`). **Recebimento:** `ReactionMessage` vira `InboundMessage{ReactTo, Reaction}` (vazio = remoção)
  e `Service.ingestReaction` grava; não cria mensagem, não conta como não lida. Reação a mensagem que o ChatHM não conhece é ignorada.
- **Validação:** só emoji (`validReaction`): sem letras, sem texto, até 10 caracteres Unicode.

## 2. Emojis na tela

- **Arte:** Noto Color Emoji (Google, SIL OFL 1.1), do pacote `@fontsource/noto-color-emoji`, igual em qualquer computador. Registrada por `emojiFont.ts` e usada pela classe
  `emoji-font` (index.css). **É o arquivo completo (5,8 MB)**, baixado só quando aparece o primeiro emoji (barra de reações, chip ou cartela) e guardado por um ano.
  Não use os 11 pedaços por faixa de caracteres (`noto-color-emoji-N-400-normal.woff2`): medido num navegador, deixam ~400 dos 1.932 emojis em branco (👍 ❤️ 😐 ✋...).
  Se atualizar o pacote, repita a conferência (renderizar todos e contar os vazios).
- **Nomes e busca:** `emojibase-data` em português (MIT; dados do Unicode CLDR), carregados só ao abrir a cartela (`emojiData.ts`). A busca ignora acento e caixa e entende
  apelidos ("kkk" acha 😂, "dedão" acha 👍, "valeu" também). Sem tons de pele (um emoji por conceito). Recentes ficam só neste navegador.
- O texto das mensagens e o campo de escrever continuam usando o emoji do sistema (não mudou).

## 3. O que o cliente vê
Reação e emoji são caracteres Unicode: o WhatsApp do cliente desenha com a arte dele. A arte escolhida aqui muda só o que a equipe vê.
Para o cliente ver uma arte própria, só como **figurinha** (imagem WebP 512×512 com fundo transparente). O backend já envia figurinha.

## 4. Próximos passos
1. Aba **Figurinhas** com os ícones próprios (recortar cada um em WebP 512×512 transparente; só os que não existem no Unicode precisam ser figurinha).
2. Usar a mesma cartela no campo de mensagem (inserir emoji no texto).
3. Tons de pele (pressionar e segurar) e quem reagiu (lista no clique do chip).
