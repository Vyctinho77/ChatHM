# Raia da IA (atendente autônomo com paredes)

Status: **servidor (migração 43) e tela prontos, tudo desligado.** Falta o aviso que acorda a IA quando o cliente responde. Veja "Próximos passos".

## 1. A ideia

O atendente autônomo livre (`send_as_attendant`, escopo `attend`) manda o que quiser, a qualquer hora. A raia troca a "gordura" de regras
(assuntos, níveis, janela de horário, aprovação) por **poucas paredes que o servidor aplica a cada envio**. Dentro da raia a IA é livre.

> A IA não é a barreira de segurança. O servidor é.

Cada **raia** é uma configuração (tabela `ai_lanes`). Um **token** do atendente autônomo pode ser **preso** a uma raia (`api_tokens.lane_key`):
preso, só a raia vale para ele. Token sem raia continua exatamente como era (nada muda para quem não ligar).

Duas raias nascem prontas, **desligadas e em sombra**:

| Raia | Para quê | Nasce assim |
|---|---|---|
| `certificado` | Atendimento inicial de certificado digital, como uma atendente mínima | só responde (não inicia); procura conversas paradas há 24 h |
| `cobranca` | Lembrete de cobrança de honorários | inicia; só links do Asaas (`asaas.com`); 1 mensagem seguida, 2 por cliente por dia |

## 2. As poucas paredes (valem a cada envio)

A IA é livre: escreve como quiser, **sem aviso de "sou uma IA"** (o nome do agente já aparece acima da mensagem), sem horário, sem lista de assuntos,
sem trava de valores, números, CPF ou e-mail. O que sobra:

**Da conversa (a primeira que falhar decide)**: raia ligada · conversa aberta · da fila da raia (se tiver fila) · sem passagem pendente para humano ·
sem atendente humano (responsável, ou resposta de uma pessoa nos últimos *N* minutos, padrão 60) · não adiada.

**Do cliente → passa para um humano**: mandou foto ou documento (a IA não vê o conteúdo) ou pediu um atendente / reclamou
("quero falar com uma pessoa", "procon", "advogado", "cancelar"...).

**Orçamento (folgado, ajustável)**: 12 rodadas da IA por conversa (depois passa para humano), 3 mensagens seguidas sem resposta do cliente
(recusa `aguarde_cliente`), 10 por cliente por dia (`limite_contato`).

**Do texto**: nunca pedir nem citar senha, código de verificação, cartão ou chave PIX (`credencial`); links só dos domínios da lista da raia
(lista **vazia = qualquer link**; na cobrança é só `asaas.com`). Documento da biblioteca não vale na raia.

**Continua de antes**: `PARAR`, grupos, "Pausar tudo", teto global de velocidade, máscaras de privacidade, assinatura (nome do agente), auditoria.

## 2.1 Conversas paradas sem atendimento

Ferramenta **`list_unattended(min_hours, limit)`**: conversas abertas em que o cliente escreveu há pelo menos *N* horas (padrão: `staleHours` da
raia; 0 na raia = usa 24) e **ninguém da equipe iniciou o atendimento**: sem responsável, nenhuma mensagem enviada por uma pessoa, nada respondido
depois do cliente e sem passagem pendente. A mais antiga primeiro, com a última mensagem do cliente. Só a fila da raia; grupos ficam de fora.
A IA lê o histórico, responde com `send_as_attendant` ou passa para uma pessoa com `request_human`. Em sombra, vira rascunho.

## 2.1b Responder com dados reais e mostrar as fontes

A IA pode tirar dúvidas (nova regra do governo, prazo, exigência) com informação **real e atual**. O ChatHM não navega na internet: quem pesquisa é a
própria IA externa, com as ferramentas de busca dela. O servidor faz a parte que dá para garantir:
- `send_as_attendant` e `check_send` aceitam **`sources`** (até 5 páginas, `title` e `url` https). O servidor valida (https, site público, sem IP/localhost,
  sem repetição) e **acrescenta ao fim da mensagem** "Fontes consultadas: • título: link". Vale também para token sem raia.
- Raia com **`requireSources`** (ligado em `certificado`): texto que fala de lei, decreto, portaria, instrução normativa, regra, Receita Federal, ICP-Brasil,
  prazo legal, "nova regra", "governo" etc. **sem `sources` é recusado** (`fontes`) e a IA é orientada a pesquisar e reenviar, ou a dizer que vai confirmar e usar `request_human`.
- Limite honesto: o servidor confere que as fontes existem como links válidos e que foram citadas, **não** que a IA realmente leu a página nem que ela interpretou
  certo. Em sombra você vê o rascunho com as fontes antes de valer.

## 2.2 Qualquer IA externa (Grok, Claude, ChatGPT, Gemini)

A raia é do servidor, não do agente: o mesmo token do atendente autônomo, preso a uma raia, funciona com **qualquer cliente MCP**. Na criação do token
escolha o provedor (Claude, ChatGPT, Grok, Gemini ou Outro): é o que define a logo e o nome acima da mensagem. Como a IA fica sabendo de trabalho novo:
- `wait_for_replies` (espera até 30 s por uma resposta de cliente, em laço) ou o aviso por webhook, se o agente tiver endereço público;
- `list_unattended` de tempos em tempos (por exemplo a cada 15 min) para as conversas paradas;
- `my_access` no começo mostra a raia, as regras e o modo (sombra/valendo).

## 3. Passar para um humano

Ferramenta MCP **`request_human(conversation_id, reason, summary)`** (só o atendente autônomo). Também acontece sozinha, pelo servidor, quando uma
parede exige ("passada_para_humano"). O que acontece:
- registro em `ai_handoffs` (motivo, resumo, quem pediu: IA ou servidor);
- etiqueta **"Precisa de humano"** na conversa e linha `[IA pediu ajuda · data · motivo] resumo` na observação do contato;
- a conversa vai para a fila de destino da raia (`handoffSegmentId`), se houver, e o adiamento é desfeito;
- a IA **fica em silêncio** naquela conversa até uma pessoa responder; quem responde vira responsável e a IA continua fora.

## 4. Modo sombra

`mode = shadow` (padrão): o envio da IA vira um **rascunho** (`ai_lane_drafts`) e **nada vai ao cliente**; a resposta ao agente é `status: "sombra"`.
As passagens para humano **acontecem** mesmo em sombra. Serve para ver, por uns dias, o que a IA faria antes de a raia valer. `mode = live` envia de verdade.

## 5. API (administrador)

| Rota | O que faz |
|---|---|
| `GET /api/v1/automation/lanes` | lista as raias |
| `PUT /api/v1/automation/lanes/{key}` | grava a configuração de uma raia |
| `GET /api/v1/automation/lane-drafts?lane=&limit=` | rascunhos do modo sombra |
| `GET /api/v1/automation/handoffs?open=1&limit=` | passagens para humano (`answered` = uma pessoa já respondeu) |
| `PUT /api/v1/ai/tokens/{id}/lane` `{"laneKey": "certificado"}` | prende o token (vazio solta). Só token do atendente autônomo |

No MCP: `my_access` mostra a raia do token; `check_send(conversation_id|contact_id, text)` confere as regras da raia sem enviar;
`send_as_attendant` devolve a recusa com `codigo` (`raia_desligada`, `fora_da_raia`, `humano_ativo`, `passada_para_humano`, `aguarde_cliente`,
`limite_contato`, `link`, `credencial`, `so_responde`, `documento`). `list_unattended` lista as conversas paradas.

## 6. Como ligar (pela tela)

1. **Atendimento por IA → Raias**: abra a raia, escolha a fila (se quiser limitar) e para onde vai a conversa quando a IA pedir uma pessoa. Ligue a raia (continua em **Sombra**).
2. **MCP → Tokens de acesso**: crie um token **Atendente autônomo** (escolha o provedor: Claude, ChatGPT, Grok, Gemini ou Outro) e, na linha do token, escolha a **Raia da IA**.
3. Em **Raias**, confira os **Rascunhos em sombra** e os **Pediram ajuda de uma pessoa** por alguns dias; ajuste limites e orientações.
4. Passe a raia para **Valendo** (pede confirmação).

A mesma coisa pela API está na seção 5.

## 7. Próximos passos

1. **Acordar a IA** quando o cliente responde: hoje o aviso automático (webhook) está desligado; sem isso a IA só age quando é disparada.
2. **Aprovar, editar ou descartar** rascunhos da sombra pela tela (hoje é só leitura).
3. **Manual do assunto e exemplos**: `get_lane_guidelines` e trechos de conversas boas já resolvidas por pessoas (mascarados, sem documentos).
4. **Painel de atividade** e **pausa automática** por comportamento anormal (muitos `PARAR`, passagens, textos repetidos).
