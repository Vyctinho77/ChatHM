# Integração WhatsApp (Fase 2)

## Arquitetura

```
whatsmeow (WhatsApp Web) ──► wameow.Provider ──► whatsapp.Manager ──► conversations.Ingest ──► PostgreSQL
                                   ▲                    │                                         │
                    store cifrado (AES-256-GCM)         └── realtime.Hub (WebSocket) ◄────────────┘
```

- `internal/whatsapp/provider.go`: interface `Provider` — o resto do sistema não conhece o whatsmeow.
- `internal/whatsapp/wameow`: adaptador whatsmeow (sessão espelhada por QR Code).
- `internal/whatsapp/fake`: provedor simulado para testes e desenvolvimento (`WHATSAPP_PROVIDER=fake`).
- `internal/whatsapp/manager.go`: estados, QR temporário, reconexão no boot, auditoria e eventos.
- `internal/conversations`: ingestão idempotente e consultas.

## Estados da conexão

| Estado | Quando |
|---|---|
| `disconnected` | Sem sessão, QR expirado, aparelho desvinculado no celular ou desconexão pelo painel |
| `pending_qr` | Pareamento em andamento; o QR (somente admin) é renovado pelo WhatsApp |
| `connected` | Sessão ativa; número e nome do perfil exibidos |
| `error` | Queda de conexão (o whatsmeow tenta reconectar sozinho), sessão aberta em outro local, bloqueio temporário |

O QR Code nunca é gravado no banco nem enviado a atendentes (API e WebSocket filtram por perfil).
Reiniciar o backend restaura a sessão salva automaticamente.

Após a leitura do QR, `pairingComplete=true` remove o código e mostra a confirmação
em andamento. Quando `Client.IsLoggedIn()` confirma a autenticação no servidor,
a interface mostra conectado com `preparing=true`, sem aguardar o upload das
pre-keys. O evento posterior `Connected` libera mensagens e a solicitação de
histórico. QR lido sozinho não é considerado autenticação. Um limite local de
espera não invalida um pareamento já confirmado.

As pre-keys são cifradas localmente e gravadas em lote dentro de uma transação,
com trava por dispositivo para que pedidos simultâneos reutilizem as mesmas
chaves. Atualizações de conexão são publicadas antes de gravar o status no banco;
sincronizar histórico não bloqueia essa confirmação.

## Importação inicial

O adaptador processa automaticamente os lotes `HistorySync` fornecidos pelo
WhatsApp: nomes de contatos e mensagens de conversas individuais, recebidas e
enviadas pelo celular. A tela mostra mensagens importadas, contatos processados e
falhas pelo evento `whatsapp.sync`. Ao finalizar cada lote, as consultas de
conversas e mensagens são atualizadas.

O histórico não incrementa não lidas, não classifica filas com base em textos
antigos e usa os mesmos identificadores para evitar duplicação. Arquivos antigos
de mídia não são baixados em massa; seus metadados e legendas são preservados.
Status e canais continuam fora do escopo.

A quantidade de histórico depende do que o WhatsApp disponibiliza ao dispositivo
vinculado. Lotes recebidos antes desta implementação não são recuperados apenas
por atualizar a página; pode ser necessário um novo pareamento. Os contadores de
sincronização ficam em memória, enquanto contatos e mensagens ficam no banco.

Os contatos também são copiados do cache persistido do WhatsApp ao conectar ou
restaurar a sessão e após sincronizações de app state. Isso independe da chegada
de um lote de mensagens. A página Clientes consulta `GET /api/v1/contacts`:
administradores veem o diretório importado; atendentes veem apenas contatos de
conversas às quais já têm acesso.

Ao confirmar a conexão, o backend solicita automaticamente até 30 dias de
histórico ao celular, em segundo plano. Isso também ocorre ao restaurar a sessão
após iniciar o servidor, uma vez por dispositivo durante essa execução. Eventos
repetidos de conexão ou alteração do perfil não repetem a solicitação.

Para tentar recuperar mensagens ausentes sem desvincular a conta, um administrador
pode usar “Tentar sincronizar novamente” (`POST /api/v1/whatsapp/history`).
O pedido manual é limitado a um por minuto. Um retorno 202
confirma somente o envio da solicitação, não a entrega das mensagens: o telefone
ou o WhatsApp podem recusar ou limitar a sincronização. As mensagens recebidas
continuam passando pelo importador idempotente de `HistorySync`.

## Segredos da sessão

O whatsmeow guarda tudo em texto puro por padrão. O ChatHM substitui o armazenamento do dispositivo
(`wameow/store.go`, `wameow/secure_store.go`) para cifrar com AES-256-GCM, com a chave `ENCRYPTION_KEY` fora do banco:

| Dado | Onde | Proteção |
|---|---|---|
| Chaves do dispositivo (noise, identidade, signed pre-key, ADV, conta) | `whatsapp_connections.device_enc` | cifrado; `whatsmeow_device` recebe zeros |
| Pre-keys (chaves privadas) | `whatsapp_prekeys` | cifrado |
| Sessões Signal, sender keys, chaves de app state, segredos de mensagem, tokens de privacidade, salt NCT | tabelas `whatsmeow_*` | valor cifrado |
| Buffer de eventos (mensagens já decifradas) | `whatsmeow_event_buffer`, `whatsmeow_retry_buffer` | valor cifrado |
| Chaves públicas de contatos, nomes, mapeamento LID↔telefone, versões de app state | tabelas `whatsmeow_*` | em claro (não são segredos) |

Trocar ou perder a `ENCRYPTION_KEY` invalida a sessão: será preciso conectar por QR Code de novo.
Os testes em `wameow/store_test.go` verificam que nenhum segredo aparece em claro no banco.

## Mensagens

- Deduplicação por `(chat_jid, external_id)`, com restrição única no banco: reentregas do mesmo evento não
  duplicam mensagens nem contam como não lidas duas vezes (inclusive com entregas concorrentes).
- Uma conversa aberta por contato (índice único parcial). Após o encerramento, a próxima mensagem abre uma nova
  conversa (que poderá ser reclassificada na Fase 3).
- Mensagens enviadas pelo próprio celular do escritório entram como `out`, sem contar como não lidas.
- A partir da Fase 3 a mensagem do cliente é classificada e a conversa vai para a fila do segmento (ou fica na fila
  geral). Ver [`queues.md`](queues.md).

## Limitações conhecidas

- **Não oficial**: o protocolo do WhatsApp Web pode mudar sem aviso; a conta pode sofrer restrições ou bloqueio,
  sobretudo com volume alto ou envio em massa. Validar estabilidade em piloto antes de depender disso em produção.
- **Um número por instalação** (MVP de um escritório).
- **Mídias**: áudio, imagem e documento chegam com metadados (tamanho, duração, nome do arquivo); o **download do
  arquivo** e a reprodução chegam na Fase 4 (armazenamento S3 privado).
- **Ignorados nesta fase**: status, canais, reações, edições e mensagens apagadas, histórico antigo
  sincronizado ao parear (só mensagens novas entram), vídeos/figurinhas/contatos/enquetes (exibidos como
  "não suportado").
- **Envio**: texto (Fase 3). O ID da mensagem é gerado antes do envio e reaproveitado em tentativas, então o
  WhatsApp descarta duplicatas. Áudio, imagens e documentos chegam na Fase 4. Ver [`queues.md`](queues.md).
- **Recibos**: entrega e leitura chegam como eventos de recibo; contatos que desativam a confirmação de leitura
  permanecem em "entregue".
- **Contatos por LID**: quando o WhatsApp não informa o telefone, o contato é identificado pelo LID até o número
  ser conhecido.

## Desenvolvimento sem celular

`WHATSAPP_PROVIDER=fake` (somente com `APP_ENV=development`) simula o pareamento (conecta sozinho ~5 s após gerar
o QR) e libera, para admin, `POST /api/v1/dev/whatsapp/inbound` e o formulário "Simular mensagem recebida" na
tela Conexão WhatsApp.

## Mídia (Fase 4)

- **Armazenamento** (`internal/storage`): `fs` (disco, cada arquivo cifrado com AES-256-GCM e a chave do objeto como dado associado) ou `s3`
  (MinIO/AWS, bucket privado; habilite a criptografia do lado do servidor no bucket). Nunca há URL pública.
- **Processamento** (`internal/media`): tipo pelo conteúdo, limite de tamanho, extensões executáveis recusadas, nomes saneados, imagens
  decodificadas (corrompida/gigante é recusada), áudio convertido por **ffmpeg** (precisa estar instalado no servidor), antivírus
  **clamd** opcional (`CLAMAV_ADDR`; falha fechada).
- **Recebimento**: a mensagem é gravada na hora; o arquivo é baixado, verificado e guardado em segundo plano (no máx. 4 por vez).
- **HD**: sem HD a imagem vai reduzida (1600 px, como o WhatsApp comum); "Foto em HD (original)" envia o arquivo original.
  Para preservar 100% (ex.: scans) use **Documento**.
- **Limitações**: vídeo, figurinhas, contatos e localização em tempo real aparecem como "não exibido"; o adaptador S3 e o envio/recebimento de
  mídia por um celular real ainda não foram validados em campo.

## Grupos e fotos de perfil

- **Grupos** entram como contatos do tipo `group` (`contacts.kind`): `wa_id` é o JID do grupo (`...@g.us`) e `name`
  é o assunto (acompanha renomeações). Cada grupo tem uma única conversa; as respostas vão para o grupo e cada
  mensagem recebida guarda quem escreveu (`authorName`). Na interface há o filtro **Grupos** na lista de conversas
  e a aba **Grupos** em Clientes. Recibos de leitura de grupos são ignorados.
- **Fotos de perfil** (pessoas e grupos): a miniatura é baixada do WhatsApp em segundo plano quando chega
  mensagem (no máximo 2 consultas simultâneas; recheca em 7 dias com foto, 1 dia sem foto), guardada em
  `contacts.avatar` e servida por `GET /contacts/{id}/avatar` (exige sessão). O campo `avatarUrl` do contato
  traz a URL com `?v=` para o cache do navegador. Sem foto (ou privada), a interface mostra as iniciais (ou o ícone de grupo).

## Renomear contatos

O lápis ao lado do nome (painel **Dados do contato** e lista **Clientes**) troca o nome exibido (`PATCH /contacts/{id}`, até 120 caracteres).
O nome definido pela equipe não é sobrescrito pelo nome do WhatsApp nem pelo assunto do grupo (`contacts.name_edited`).
Apagar o nome volta a seguir o WhatsApp. Quem vê a conversa pode renomear; o administrador também renomeia contatos importados sem conversa.
