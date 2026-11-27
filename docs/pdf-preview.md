# Prévia dos PDFs no balão

Como no WhatsApp: PDF recebido ou enviado mostra, em cima do cartão, o **alto da primeira página** e, embaixo, "N páginas • PDF • tamanho". Os arquivos em geral
(Word, Excel, certificado...) ganharam a mesma borda fina do balão de foto (3 px) e o cartão ocupa o balão quase de ponta a ponta (largura fixa de 18 rem).

## Como funciona
- **Prévia:** `GET /api/v1/attachments/{id}?size=preview` (a mesma rota da prévia das fotos). Para PDF, o servidor desenha a primeira página com o **poppler** (`pdftoppm`), 640 px de largura
  e até 360 px de altura (só o alto da página), em JPEG (~10 KB). Na primeira vez que alguém pede, fica guardada ao lado do original (`<chave>.preview`); as próximas leituras não rodam o poppler.
  A tela mostra uma faixa de 116 px (`object-cover object-top`) e a esconde se o servidor responder 404.
- **Sem prévia (404):** PDF com senha, quebrado, maior que 40 MB, ou imagem sem o poppler instalado. O servidor lembra a falha por 1 hora (só na memória) para não tentar a cada rolagem. O arquivo continua abrindo e baixando.
- **Páginas:** `payload.pages`. Vem do WhatsApp (`DocumentMessage.PageCount`) quando ele informa; senão o servidor conta com `pdfinfo` ao guardar o arquivo (recebido ou enviado) e, nos PDFs antigos,
  `BackfillPDFPages` conta devagar ao iniciar (um por vez, 150 ms entre eles). PDF ilegível fica sem número.
- **Limites de segurança** (o arquivo é do cliente): tamanho máximo, prazo de 15 s, no máximo 2 execuções ao mesmo tempo, pasta temporária própria apagada no fim, ambiente vazio, processo sem privilégios
  (o contêiner roda como usuário `app`). Não há verificação de vírus nesse caminho (igual ao resto dos anexos hoje).
- **Docker:** `poppler-utils` entra no `apk add` do `Dockerfile` (cerca de 20 MB). Sem ele o app funciona normalmente, só sem prévia nem contagem de páginas.

## Código
`backend/internal/media/pdf.go` (poppler), `backend/internal/conversations/pdfpreview.go` (rota, cache, páginas, backfill), `apps/web/src/features/conversations/MessageBubble.tsx` (`DocumentCard`, `PdfPreview`).
