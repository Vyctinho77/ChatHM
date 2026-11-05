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
- Fase 2: todas as conversas ficam na **fila geral**, visível a todos os usuários autenticados (regra 3).

## Limitações conhecidas

- **Não oficial**: o protocolo do WhatsApp Web pode mudar sem aviso; a conta pode sofrer restrições ou bloqueio,
  sobretudo com volume alto ou envio em massa. Validar estabilidade em piloto antes de depender disso em produção.
- **Um número por instalação** (MVP de um escritório).
- **Mídias**: áudio, imagem e documento chegam com metadados (tamanho, duração, nome do arquivo); o **download do
  arquivo** e a reprodução chegam na Fase 4 (armazenamento S3 privado).
- **Ignorados nesta fase**: grupos, status, canais, reações, edições e mensagens apagadas, histórico antigo
  sincronizado ao parear (só mensagens novas entram), vídeos/figurinhas/contatos/enquetes (exibidos como
  "não suportado").
- **Envio** pelo ChatHM chega na Fase 3, junto com a atribuição atômica ao primeiro atendente que responder.
- **Contatos por LID**: quando o WhatsApp não informa o telefone, o contato é identificado pelo LID até o número
  ser conhecido.

## Desenvolvimento sem celular

`WHATSAPP_PROVIDER=fake` (somente com `APP_ENV=development`) simula o pareamento (conecta sozinho ~5 s após gerar
o QR) e libera, para admin, `POST /api/v1/dev/whatsapp/inbound` e o formulário "Simular mensagem recebida" na
tela Conexão WhatsApp.
