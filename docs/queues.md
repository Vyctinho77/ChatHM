# Classificação, filas e atendimento (Fase 3)

## Classificação automática

Código: `backend/internal/segments` (`normalize.go`, `classify.go`). Sem IA: o resultado é sempre explicável.

**Quando classifica.** Na chegada de cada mensagem do cliente com texto (ou legenda), e **somente** se a conversa
ainda não tem fila e ninguém assumiu. Conversa em andamento nunca muda de fila por palavras isoladas (regra 5).
Depois que o atendimento é encerrado, uma nova mensagem abre uma nova conversa, classificada de novo.

**Como compara.** Texto e palavras-chave passam pela mesma normalização:

1. minúsculas e sem acentos (`Férias` → `ferias`; `13º` → `13o`);
2. pontuação separa palavras;
3. palavras de ligação são ignoradas (`de`, `da`, `uma`, `para`, `meu`…), então `emitir nota` combina com
   "quero emitir **uma** nota";
4. plural simples vira singular (`notas`→`nota`, `fiscais`→`fiscal`, `emissões`→`emissao`, `valores`→`valor`).

A comparação é por **palavra inteira**: `folha` não combina com "folheto" nem "folhagem". Uma expressão
(`nota fiscal`) combina quando as palavras aparecem **juntas e na ordem**.

**Curinga `*`.** Uma palavra-chave pode terminar em `*` para pegar variações: `emit*` combina com emitir, emita,
emitiu e emitido (o prefixo vale como foi escrito, sem acento nem plural). O `*` só vale no fim de uma palavra e com ao
menos 3 letras antes dele (`a*`, `*nota` e `no*ta` são recusados). `emit* nota` combina com "emita a nota" e não com
"nota emitir" (as palavras seguem juntas e em ordem).

**Como decide.** Cada palavra-chave tem uma prioridade (0–100, padrão 0). A pontuação de um segmento é a maior
prioridade entre as suas palavras que apareceram.

| Situação | Resultado |
|---|---|
| Exatamente um segmento com a maior pontuação | vai para a fila desse segmento (`classification = keyword`) |
| Empate entre segmentos | **fila geral** (`conflict`) |
| Nenhuma palavra encontrada | **fila geral** (`none`) |

Exemplos (todos cobertos por testes): "quero emitir uma nota" → Emissão de notas; "preciso calcular a folha" e
"preciso de férias de funcionário" → Folha de pagamento; "quero orçamento" e "quero contratar" → Clientes novos;
"bom dia" → geral; "quero orçamento da folha de pagamento" → conflito → geral. O administrador resolve um conflito
conhecido dando prioridade maior a uma das palavras.

Uma conversa que ficou na fila geral (sem correspondência ou conflito) e ainda não foi assumida pode ser classificada
por uma mensagem posterior do cliente. Segmentos desativados não classificam.

**Configuração (admin, tela Filas / Segmentos).** Segmentos, palavras-chave, prioridades, equipe de cada fila e um
campo "Testar classificação". Os três segmentos iniciais e suas palavras são criados uma única vez; apagar um
segmento depois não o recria. Excluir um segmento devolve suas conversas à fila geral.

### Palavras-chave pelas equipes

- **Quem edita.** O administrador, em todos os segmentos; os integrantes de uma equipe, nas palavras dos segmentos
  **ligados à própria equipe** (tela Filas, seção *Palavras-chave*, ou `/filas/palavras/<fila>`). Quem não é da equipe
  nem vê as palavras. A prioridade, o vínculo de equipe e a estrutura continuam só do administrador: palavras
  cadastradas pela equipe entram sempre com prioridade **50**.
- **Como cadastrar.** Uma por vez ou várias de uma vez (uma por linha, ou separadas por vírgula), com até 300
  palavras por segmento. Repetidas e inválidas são puladas com o motivo, sem impedir as outras. Se a mesma palavra
  (com a mesma prioridade) já existe em outra fila ativa, o sistema avisa que as mensagens com ela vão empatar e cair na
  fila geral.
- **Sugestões.** Cada segmento recebe sugestões conforme o nome (Folha/DP/Pessoal, Notas/Fiscal, Impostos, Certificado,
  Clientes novos), sem repetir o que já tem. Termos técnicos e expressões (`nota fiscal`, `holerite`, `fgts`) entram com
  prioridade 60; palavras soltas e genéricas (`nota`, `folha`) com 40, para que o termo técnico decida quando duas filas
  combinam na mesma mensagem. Vêm marcadas as de prioridade 60. Código: `segments/suggestions.go`.
- **Teste.** *Testar uma mensagem* mostra para qual fila ela iria e por qual palavra (qualquer pessoa pode usar).
- **Auditoria.** Cada palavra adicionada ou removida é registrada com quem fez.

### Conversa em andamento: chamar a equipe do assunto

Regra 5 continua valendo: a conversa **não troca de dono, de fila nem de status** por palavras soltas. Mas quando o
cliente, numa conversa **já em atendimento** (ou já numa fila), passa a falar do assunto de **outra** equipe ("emite a
nota fiscal" numa conversa atendida pela Folha), a **equipe do assunto entra como participante** e a conversa aparece em
**Minhas** de cada integrante, sem tirar nada de quem atende. Detalhes:

- só mensagem do cliente em conversa 1:1 aberta, e só quando **uma** equipe vence a classificação (empate não chama
  ninguém);
- o segmento precisa estar ativo, ter equipe e estar com *Chamar a equipe em conversas em andamento* ligado (padrão:
  ligado; o administrador desliga por segmento);
- **uma vez por assunto em cada conversa**: citar de novo não repete, e quem tirar a equipe à mão não a vê voltar;
- o responsável atual nunca vira participante, e quem já participa não é incluído de novo;
- fica na auditoria como `conversation.team_called` (assunto, palavra que combinou e quantas pessoas);
- quem entra como participante passa a ver também o histórico da conversa em que participou (como ocorre com
  participantes incluídos à mão), mas não o dos atendimentos de que não participou.

## Quem vê cada conversa

| Situação da conversa | Quem enxerga |
|---|---|
| Sem responsável, fila geral ou segmento **sem equipe** | todos os usuários |
| Sem responsável, segmento com equipe | integrantes da equipe + admin |
| Assumida, segmento com equipe | integrantes da equipe + admin |
| Assumida, fila geral | **somente o responsável** + admin |

A regra vale em todas as consultas (lista, detalhe, mensagens, leitura, contato, etiquetas, envio, encerrar) e nos
eventos do WebSocket. Quem não pode ver recebe `404`. Quando a visão muda (outra pessoa assumiu uma conversa da
fila geral, ou a conversa foi classificada para outra equipe), quem perdeu o acesso recebe `conversation.removed`
e a conversa some da lista sem recarregar.

Atenção: um segmento **sem equipe vinculada** é visível a todos. Por isso excluir uma equipe vinculada a segmentos é
recusado (`409 team_in_use`): desvincule os segmentos antes. Mudanças de equipe de um usuário valem na próxima
consulta (a lista é recarregada ao abrir a tela de Filas ou a página).

## Assumir o atendimento (atômico)

Responder (`POST /conversations/{id}/messages`) é o ato de assumir. Numa transação:

1. `SELECT ... FOR UPDATE` na conversa serializa respostas simultâneas;
2. se já há responsável diferente → `409 conversation_taken` (nada é gravado nem enviado);
3. se não há → grava o responsável, muda para "em andamento" e registra em `conversation_assignments`;
4. grava a mensagem como `pending`.

Segunda barreira no banco: índice único parcial `conversation_assignments(conversation_id) WHERE ended_at IS NULL`.
O teste `TestConcurrentClaimOnlyOneWins` dispara dois atendentes ao mesmo tempo em 12 conversas e exige exatamente um
vencedor por conversa, uma atribuição ativa e um envio ao WhatsApp.

Sem WhatsApp conectado a resposta é recusada (`503`) **antes** de assumir: a tentativa não prende a conversa.

## Envio sem duplicar

O `id` da mensagem é gerado no navegador e enviado ao servidor. Repetir a requisição (duplo clique, "tentar de
novo") devolve a mesma mensagem e não reenvia. O envio ao WhatsApp acontece depois do commit; se falhar, a mensagem
fica `failed` e continua na conversa (nada é apagado). "Tentar de novo" reenvia com o mesmo ID, e o WhatsApp
descarta duplicatas. Se o servidor recusar (ex.: outra pessoa assumiu), o texto volta ao campo de mensagem.

## Recibos

Entrega e leitura chegam do WhatsApp e atualizam `status`, `deliveredAt`, `readAt` (o status só avança: enviada →
entregue → lida). Contatos que desativam a confirmação de leitura ficam em "entregue".

## Encerrar

Quem pode: o responsável e administradores. Encerra a atribuição, mantém o histórico e libera uma nova conversa
para a próxima mensagem do cliente. Ação idempotente.

## Etiquetas

Catálogo editável só pelo admin (padrão: PENDENTE, URGENTE, PGDAS, CERTIFICADO, ACOMPANHAMENTO). Qualquer usuário
que veja a conversa aplica e remove. Etiquetas nunca alteram o segmento.

## Auditoria

`conversation.claim`, `conversation.close`, `segment.*`, `tag.*` (sem conteúdo de mensagens).

## Limitações conhecidas

- Responder só envia **texto**; áudio, imagens e documentos chegam na Fase 4.
- Conversas atribuídas a um usuário desativado continuam com ele; um administrador pode encerrá-las.
- Não há transferência manual entre equipes (por decisão do blueprint: sem triagem manual).
- Visualização "Não lidas" é global por conversa, não por usuário.

## Transferência e encerramento

- **Transferir** (clique direito na conversa ou botão no chat): lista todo o escritório (`GET /people`). Pode transferir o
  responsável, um administrador ou qualquer pessoa se a conversa ainda não tem responsável. A conversa passa a ser só do
  destinatário (e de administradores), como ao assumir.
- **Encerrar**: o atendimento sai da lista de todos. Se o cliente voltar, abre-se um novo atendimento, classificado de novo:
  assuntos da minha equipe caem na fila da equipe (qualquer integrante assume ao responder); outros assuntos aparecem só
  para as outras equipes. O histórico anterior só é visível a quem podia ver aquele atendimento.
- Filtros da lista: **Minhas** (atribuídas a mim), **Não atribuídos** (sem equipe) e **Todas**.

## Começando do zero

Uma instalação nova **não traz segmentos, palavras-chave nem etiquetas**: tudo é definido pelo gestor. Enquanto não existir segmento,
toda conversa fica em Não atribuídos. O caminho é: criar **equipes** (Equipe → Equipes → Nova equipe), criar **segmentos** com palavras-chave
e vincular cada um a uma equipe (Filas → Configurar filas), convidar as pessoas e conectar o WhatsApp — o painel "Primeiros passos"
na tela de Conversas guia essas etapas. Para adiantar, "Importar exemplos" (`POST /segments/examples`, só admin) acrescenta Clientes novos,
Folha de pagamento e Emissão de notas, com palavras-chave e etiquetas comuns, sem sobrescrever segmentos de mesmo nome ou chave e
sem vincular equipes. `SEED_EXAMPLE_DATA=true` faz o mesmo na primeira subida (para desenvolvimento).
