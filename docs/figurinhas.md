# Figurinhas no chat

Mensagens `StickerMessage` do WhatsApp são importadas como `kind: "sticker"`.
O arquivo WebP passa pelo processamento de mídia e antivírus configurado e fica
no armazenamento privado, com a mesma autorização dos demais anexos.
O original é preservado para manter transparência e animação.

A interface mostra a figurinha sem fundo nem cauda, com hora e recibos em uma
cápsula abaixo. O menu de contexto permite responder, baixar e encaminhar.
Na lista de conversas e nas respostas citadas, a descrição é “Figurinha”.
Falhas de download, bloqueios e arquivos indisponíveis têm estados próprios.

A migração `00021_stickers.sql` libera o novo tipo. Reinicie a API local para
aplicá-la e carregar o conversor atualizado. Figurinhas antigas gravadas como
`unsupported` não têm arquivo salvo e não podem ser recuperadas por esta mudança.
O histórico continua seguindo a regra atual do app: importa metadados de mídia,
sem baixar arquivos antigos. Não foi adicionado um catálogo de figurinhas.

Os testes usam WebP sintéticos, transparente e animado, em
`backend/internal/media/testdata`. O teste HTTP cobre recebimento, armazenamento,
download autenticado e encaminhamento sem alterar o arquivo.
