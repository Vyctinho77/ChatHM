# Tipografia

O ChatHM usa a mesma família do WhatsApp Web: **Roboto Variable** (pacote `@fontsource-variable/roboto`, licença SIL OFL 1.1), com 14,2 px no texto principal.

Origem: o painel "Font info" do DevTools no WhatsApp Web mostrou `"Roboto Variable", Roboto, "Helvetica Neue", Helvetica, sans-serif`, com 14,2 px (mensagens), 12 px, 13,33 px (padrão de campos), 14 px e 11 px.
Antes o app usava Inter 13,5 px com espaçamento negativo (`-0,003em`), `cv11` e a suavização forçada (`antialiased`). Num teste lado a lado na tela do dono (Windows, zoom 100%), a Roboto 14,2 px com linha de 19 px
ficou mais nítida que a Inter (com ou sem tamanho inteiro) e que a fonte do sistema.

## O que vale hoje (src/index.css)
- `--font-sans`: Roboto Variable, Roboto, Helvetica Neue, Helvetica, depois a fonte do sistema (só se a Roboto não carregar).
- `body`: 14,2 px, linha 1,34 (≈19 px). Textos de mensagem e campo de escrever: `text-[14.2px] leading-[19px]`.
- Sem espaçamento entre letras, sem `font-feature-settings` e sem forçar a suavização: a nitidez fica por conta da fonte e do sistema.
- Os outros tamanhos (13, 12,5, 12, 11,5, 11 px...) continuam como estavam.
- Só o pedaço "latin" da fonte (43 KB) é baixado para textos em português; os outros (cirílico, grego etc.) só se aparecerem caracteres deles.
- Os emojis usam a fonte própria (docs/emojis.md), não a Roboto.

## Se quiser ajustar
Mudar o tamanho principal: `body` e as classes `text-[14.2px]` (um `sed` troca todas). Voltar à Inter: `pnpm add @fontsource-variable/inter` e trocar o `@import` e o `--font-sans`.
