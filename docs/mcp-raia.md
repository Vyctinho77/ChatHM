# Raia da IA (atendente autônomo com paredes)

Status: **servidor implementado (migração 43), tudo desligado.** Falta a tela (aba "Raias" em *Atendimento por IA*, ligação do token à raia, rascunhos e
banner de passagem) e o aviso que acorda a IA quando o cliente responde. Veja "Próximos passos".

## 1. A ideia

O atendente autônomo livre (`send_as_attendant`, escopo `attend`) manda o que quiser, a qualquer hora. A raia troca a "gordura" de regras
(assuntos, níveis, janela de horário, aprovação) por **poucas paredes que o servidor aplica a cada envio**. Dentro da raia a IA é livre.

> A IA não é a barreira de segurança. O servidor é.

Cada **raia** é uma configuração (tabela `ai_lanes`). Um **token** do atendente autônomo pode ser **preso** a uma raia (`api_tokens.lane_key`):
preso, só a raia vale para ele. Token sem raia continua exatamente como era (nada muda para quem não ligar).

Duas raias nascem prontas, **desligadas e em sombra**:

| Raia | Para quê | Nasce assim |
|---|---|---|
| `certificado` | Atendimento inicial de certificado digital: orientar o cliente como um atendente faria no começo | só responde (não inicia), sem números, sem links |
| `cobranca` | Lembrete de cobrança de honorários | inicia, aceita valores e o link do Asaas (`asaas.com`), 1 mensagem seguida, 2 por cliente por dia |

## 2. As paredes (valem a cada envio)

**Da conversa (nesta ordem; a primeira que falhar decide)**
1. Raia ligada. 2. Conversa aberta. 3. Da fila da raia (se a raia tem fila). 4. Não foi passada para um humano. 5. Sem atendente humano
(responsável, ou resposta de uma pessoa nos últimos *N* minutos, padrão 60). 6. Não está adiada.

**Do que o cliente mandou (desde a última mensagem enviada) → passa para um humano**
Foto ou documento (`blockInboundMedia`), CPF/CNPJ/números longos, senha/código/cartão/PIX, pedido de atendente ou reclamação
("quero falar com uma pessoa", "procon", "advogado", "cancelar"...), outros tipos (localização, contato) e rajada (8+ mensagens em 2 min).

**Orçamento**
- `maxTurns` rodadas da IA por conversa (desde a última mensagem de uma pessoa) → passa para um humano.
- `maxConsecutive` mensagens seguidas sem resposta do cliente → espera (recusa `aguarde_cliente`).
- `contactDaily` mensagens da IA por cliente em 24 h → recusa `limite_contato`.

**Do texto que a IA quer mandar**
Sem e-mail, sem pedir/citar senha, código de verificação, cartão ou chave PIX, sem CPF/CNPJ e identificadores longos. Sem valores (`R$`) e
números de 7+ dígitos, **salvo** em raia com `allowDigits` (cobrança). Datas e horas passam. Link só dos domínios da lista da raia
(`allowedDomains`, vazio = nenhum). Termos de pressão (protesto, negativação, SPC/Serasa...) barram (`blockPressure`). Documento da biblioteca
não vale na raia. Na primeira mensagem da IA na conversa o servidor acrescenta "Sou o assistente virtual do escritório." (`discloseAi`).

**O que continua de antes:** `PARAR`, grupos, "Pausar tudo", teto global de velocidade, máscaras de privacidade, assinatura, auditoria.

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
`limite_contato`, `digitos`, `documento`, `link`, `pressao`, `credencial`, `email`, `so_responde`, `documento`).

## 6. Como ligar (quando a tela existir; hoje só pela API)

1. Crie (ou use) a fila "Certificado digital" com palavras-chave e a equipe; aponte `segmentId` da raia `certificado` para ela.
2. Em MCP, crie um token **Atendente autônomo** e prenda-o à raia. Deixe a raia em **sombra** e ligada.
3. Revise os rascunhos por alguns dias; ajuste as regras e o manual (`guidelines`).
4. Passe a raia para **valendo**.

## 7. Próximos passos

1. **Tela**: aba *Raias* (editar, ligar, sombra/valendo), escolher a raia ao criar/editar o token, lista de rascunhos com aprovar/editar/descartar,
   banner "IA pediu ajuda" na conversa.
2. **Acordar a IA** quando o cliente responde: hoje o aviso automático (webhook) está desligado; sem isso a IA só age quando é disparada.
3. **Manual do assunto e exemplos**: `get_lane_guidelines` e trechos de conversas boas já resolvidas por pessoas (mascarados, sem documentos).
4. **Painel de atividade** e **pausa automática** por comportamento anormal (muitos `PARAR`, passagens, textos repetidos).
