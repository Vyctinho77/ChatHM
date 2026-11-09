# Plano: envios automáticos pela IA (MCP, etapa 6)

Status: **só plano, nada implementado.** Decisões em aberto na seção 11.

## 1. O que se quer

Hoje a IA nunca envia: `draft_reply` e `send_message` só deixam uma **sugestão** que uma pessoa aprova na conversa (etapa 4).
A evolução é permitir **automações**: a IA dispara mensagens e PDFs (cobrança de honorários, lembrete de guia, envio de DAS, aviso de prazo)
**sem ficar esperando resposta**: a ferramenta devolve na hora um número de lote e o envio acontece em segundo plano.

O que **não** é: disparo em massa para números frios, nem a IA conversando livremente com o cliente. Quando o cliente responde, quem responde é uma pessoa.

## 2. Por que isso é diferente (e mais arriscado) que sugerir

| Risco | Por que importa aqui | Resposta no desenho |
|---|---|---|
| **Instrução escondida em mensagem de cliente** (prompt injection) | A IA lê textos de clientes. Hoje o pior caso é uma sugestão ruim; com envio direto vira mensagem enviada | A IA **não escreve o texto livre**: escolhe um **modelo aprovado por humano** e preenche variáveis simples. Destinatários só de dentro do ChatHM |
| **Número de WhatsApp banido** | O ChatHM usa o WhatsApp pelo protocolo não oficial (whatsmeow). Padrão de spam pode restringir o número do escritório inteiro | Limites de ritmo, só contatos que já falaram com o escritório, opt-out, kill switch (seção 6) |
| **Mensagem errada é irreversível** | Cobrança com valor ou cliente errado, no nome do escritório | Prévia por destinatário, aprovação humana do lote (padrão), cancelamento enquanto não enviado, relatório |
| **Vazamento de documento** | A IA mandaria o PDF do cliente A para o cliente B | A IA nunca envia bytes: aponta um documento por id, e o servidor confere de quem ele é (seção 5) |
| **LGPD e cobrança** (CDC art. 42) | Cobrança não pode expor nem constranger; dados só ao titular | Textos de modelo escritos por humano, horário permitido, opt-out registrado. *Não é parecer jurídico: um advogado deve revisar os modelos de cobrança.* |

**Ponto estratégico:** para automação recorrente e em volume (cobrança mensal de dezenas ou centenas de clientes), o caminho robusto é a **API oficial do WhatsApp Business** (modelos aprovados pela Meta, sem risco de banimento por padrão, custo por conversa). O desenho abaixo funciona nos dois mundos (a fila e os modelos não dependem do protocolo), mas se o volume previsto for alto, vale avaliar a API oficial **antes** de ligar o envio automático no número atual.

## 3. Conceitos novos

- **Modelo**: texto aprovado pelo administrador, com variáveis (`{nome}`, `{valor}`, `{vencimento}`, `{competencia}`), tipo (texto ou texto + PDF), categoria (cobrança, lembrete, aviso, documento) e regras próprias (janela de horário, intervalo mínimo por cliente, máximo por dia). Só o administrador cria e edita. A IA só lê a lista.
- **Lote**: conjunto de envios criado pela IA (ou por uma pessoa): `modelo + lista de destinatários + variáveis de cada um + documento opcional`. Estados: `rascunho → aguardando aprovação → aprovado/agendado → enviando → concluído | parcial | cancelado`.
- **Item do lote**: um envio. Estados: `pendente → enviado | falhou | pulado (com o motivo)`. Cada item tem id próprio (idempotência).
- **Biblioteca de documentos**: PDFs prontos para enviar (subidos por uma pessoa, passam pelo antivírus e pela validação de mídia atuais), cada um com dono opcional (um cliente) ou marcado "compartilhável".
- **Fila de saída (outbox)** persistente no banco + **worker** que envia respeitando os limites. Reinício do servidor no meio de um lote não duplica nem perde itens.

## 4. Como a IA usa (ferramentas MCP novas)

Todas exigem o escopo novo **`automate`** (acima de `send`) e a chave do administrador *"Permitir envios automáticos"* (`ai_settings.allow_automation`, desligada por padrão).

| Ferramenta | O que faz | Observação |
|---|---|---|
| `list_templates` | Lista os modelos e as variáveis de cada um | só leitura |
| `list_documents` | Lista a biblioteca (id, nome, dono, tamanho) | só leitura |
| `preview_batch` | **Simulação (dry-run)**: devolve o texto final por destinatário e quem seria pulado, com o motivo | não cria nada, não envia |
| `create_batch` | Cria o lote e o deixa **aguardando aprovação** (ou rascunho) | devolve o id **na hora**; não envia |
| `get_batch` | Estado do lote e de cada item (enviado, falhou, pulado, respondeu) | a IA consulta depois, sem esperar |
| `cancel_batch` | Cancela o que ainda não foi enviado | |

**Não existe `send_now`.** O envio só acontece pelo worker, depois da aprovação (ou da regra automática, etapa 6c).

## 5. Quem pode receber, e o que pode ir

Regras checadas **no servidor** para cada item, na hora de criar e de novo na hora de enviar (o mundo muda entre uma coisa e outra):

1. Destinatário é um **contato existente** do ChatHM com conversa: nunca um número informado pela IA. Grupos ficam de fora.
2. **Sem opt-out.** Se o cliente responde `PARAR` (ou o administrador marca), o contato fica "não perturbe" e a IA não consegue mais incluí-lo.
3. **Não atropelar atendimento.** Pula se houver conversa aberta com mensagem de uma pessoa nas últimas X horas (configurável) ou se estiver adiada por alguém.
4. **Variáveis validadas por tipo** (valor é número, vencimento é data, nome tem tamanho máximo), sem quebra de linha nem caracteres de formatação do WhatsApp. A IA não consegue colocar texto livre.
5. **Documento**: precisa ser da biblioteca e ser **do próprio destinatário** ou marcado compartilhável. Tentar mandar o PDF de um cliente a outro recusa o item (e é auditado).
6. **Dados financeiros sensíveis**: a máscara atual (chave PIX, boleto, conta) continua valendo para tudo que a IA lê. PIX e boleto vão **no texto fixo do modelo** ou no PDF da biblioteca, escritos por pessoa, nunca montados pela IA.

## 6. Limites e freios (anti-banimento e anti-erro)

- **Ritmo**: intervalo entre envios com variação aleatória (padrão 8 a 25 s) e teto por hora.
- **Teto diário** global, começando baixo (padrão 100/dia) e subindo aos poucos pelo administrador.
- **Por cliente**: no máximo 1 mensagem automática por dia e N por semana.
- **Janela de horário** por modelo (padrão seg a sex, 8h às 18h, fuso do escritório; cobrança sem domingos e feriados).
- **Parada automática** se mais de X% dos itens falharem, se o WhatsApp desconectar ou se muitos clientes responderem `PARAR`/reclamarem. O lote fica pausado e o administrador é avisado na tela.
- **Botão "Pausar tudo"** do administrador, com efeito imediato (o worker confere a cada item).
- **Modo teste**: o lote vai só para o número do próprio administrador, para ver o texto final antes de usar de verdade.

## 7. Execução

- Novo caminho `conversations.SendAutomated` (não reaproveita o `Send` de pessoa): **não assume a conversa**, não muda o responsável, não marca "em atendimento", grava `payload.automated = true` e o lote/item de origem. Reaproveita a validação de mídia, o envio e os recibos (entregue/lido).
- Assinatura configurável (padrão: nome do escritório, não o nome de uma pessoa). No app a mensagem tem selo **"Automática"**.
- Idempotência por item; reenvio limitado (2 tentativas) e só em falha de rede, nunca em recusa do WhatsApp.
- **Resposta do cliente**: entra normalmente na fila do ChatHM (responsável anterior ou fila geral), com a etiqueta opcional "respondeu envio automático". A IA não responde sozinha.

## 8. Telas (administrador e gestor)

Nova área **Envios automáticos**:
1. **Modelos** (administrador): criar, prévia com dados de exemplo, regras.
2. **Lotes para aprovar**: lista por destinatário com o texto final, checkbox para tirar itens, botões *Aprovar e enviar*, *Agendar*, *Rejeitar*. Quem aprova é humano logado (com MFA para administrador). Gestor aprova só lotes cujos destinatários são das equipes dele.
3. **Histórico e relatório**: enviados, falhas, pulados (com motivo), respostas. Exportável.
4. **Controles**: limites, janela de horário, pausar tudo, modo teste.

## 9. Auditoria e privacidade

Cada item registra: quem criou (token e pessoa dona), quem aprovou, modelo, destinatário (id), variáveis, resultado e horário. Aprovar, cancelar, pausar e mudar limites entram em `audit_logs`. As respostas MCP continuam mascaradas; clientes OAuth (ChatGPT, claude.ai) seguem sem telefone.

## 10. Fases (cada uma publicável sozinha)

- **6a. Base sem IA** (a maior): modelos, lotes, fila com limites, biblioteca, telas, opt-out, relatório. Já é útil sozinha: uma pessoa monta um lote (ex.: “lembrete de vencimento” para uma etiqueta) e aprova. Menor risco.
- **6b. IA propõe lotes**: as ferramentas da seção 4, com aprovação humana sempre. A IA só prepara; o humano revisa e libera.
- **6c. Aprovação automática por regra** (opcional, só depois de meses de uso limpo): modelos escolhidos pelo administrador (ex.: lembrete de guia) podem sair sem aprovação, dentro dos limites. Cobrança fica sempre com aprovação.
- **6d. Pós-envio**: etiquetas e filas automáticas para quem responde, relatórios por período.

Testes obrigatórios em cada fase: idempotência e reinício no meio do lote; limites, janela e teto; opt-out; documento de outro cliente recusado; humano atendendo no mesmo momento; kill switch durante o envio; texto de cliente que "pede" mudança do lote (injeção) sem efeito; permissões por perfil (atendente, gestor, administrador); homologação com um número de WhatsApp de teste antes do número do escritório.

## 11. Decisões que dependem de você

1. **Volume**: quantas mensagens automáticas por dia/mês? Acima de algumas centenas por mês, recomendo avaliar a API oficial antes.
2. **Primeiro caso de uso**: cobrança de honorários, lembrete de guias/prazos ou envio de DAS/PDFs? Define os primeiros modelos.
3. **De onde vêm os dados** (nome, valor, vencimento, PDF)? O ChatHM **não tem financeiro**: precisa de uma fonte (planilha, sistema contábil, ou a própria IA lendo outro conector). Isso decide o desenho da 6b.
4. **Consentimento**: os clientes já aceitaram receber mensagens automáticas (contrato ou cadastro)? Define se há opt-in prévio ou só opt-out.
5. **Quem aprova**: só administrador, ou gestor da equipe também?
6. **Horário e feriados**: janela padrão e calendário de feriados do escritório.
7. **Assinatura**: nome do escritório, nome de quem criou o lote ou "Assistente do escritório"?
