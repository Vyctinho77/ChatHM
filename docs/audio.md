# Gravação de voz

O navegador captura em mono, preferindo 48 kHz, com cancelamento de eco e
supressão de ruído. O ganho automático fica desativado quando o dispositivo permite,
para não amplificar o chiado ambiente nas pausas. São preferências (`ideal`), não
requisitos que impeçam o uso de outros microfones.

Antes do MediaRecorder, dois filtros leves de Web Audio atenuam frequências abaixo
de 80 Hz e acima de 12 kHz. O fluxo não é reproduzido nos alto-falantes. Se Web Audio
estiver indisponível, a gravação mantém o processamento nativo do navegador.

A codificação continua em Opus a 64 kbps quando suportada. O servidor mantém o caminho
existente de remux de Opus mono para Ogg, sem outra compressão. Áudios recebidos não
passam por esses novos filtros de captura.

Os sons do app param quando o microfone é aberto; novas notificações sonoras ficam
silenciadas durante a gravação. Ao enviar, descartar ou sair da conversa, os recursos
do microfone e do contexto de áudio são liberados e os sons voltam ao normal.
Antes de abrir o microfone, o app aguarda o efeito curto do botão terminar. O silêncio
sobrando no arquivo original não é incluído na cópia do efeito usada pelo app.

O controle de início evita duas capturas simultâneas em cliques repetidos. Falha de
permissão, dispositivo indisponível e desconexão do microfone têm mensagens próprias.
Isso reduz fontes de interferência do app; não corrige saturação ou ruído produzido
pelo hardware, e a supressão nativa varia conforme navegador e dispositivo.

Referências: [restrições de captura](https://www.w3.org/TR/mediacapture-streams/)
e [filtros Web Audio](https://www.w3.org/TR/webaudio/#BiquadFilterNode).

## Transcrição das mensagens de voz recebidas (11/2026)

Cada mensagem de voz que o cliente manda é transcrita **na própria máquina**, em segundo plano, com o [whisper.cpp](https://github.com/ggml-org/whisper.cpp)
(código aberto; sem custo e sem enviar o áudio a nenhum serviço). **A transcrição é só para os agentes de IA:** ela não aparece na tela da equipe (nem por tempo real, nem na lista
ou na busca) e só chega ao agente pelo MCP, em `read_messages` e `list_pending_replies` (`media.transcript`; `media.transcriptPending` enquanto não termina;
`media.transcriptUnavailable` = `failed`, `toolong` ou `empty` quando não houve texto). O texto passa pelo mesmo filtro de privacidade das mensagens.

- **Estados** (`transcriptState`, guardado na mensagem e nunca enviado à tela): `pending`, `done`, `empty` (sem fala), `failed` (o áudio continua tocável, só sem texto) e `toolong` (acima de 10 minutos).
- **Como funciona:** o ffmpeg converte para WAV 16 kHz mono e o `whisper-cli` lê. Um áudio por vez (usa metade dos núcleos), em prioridade baixa (`nice`), fila em memória.
  Ao iniciar, o servidor transcreve também os áudios dos últimos 7 dias que ainda não têm texto. Código: `internal/transcribe`, `conversations/transcript.go`.
- **Memória:** o modelo `small` quantizado deve usar entre 0,5 e 1 GB enquanto transcreve (estimativa; **medir na VPS depois de publicar**, olhando o container do app durante um áudio). Na VPS de
  4 GB, o app e o Caddy usavam cerca de 40 MB juntos antes disso. Proteção: antes de cada áudio o servidor confere a memória disponível (`MemAvailable`) e **espera** se houver menos de
  1000 MB livres (`WHISPER_MIN_FREE_MB`; `-1` desliga), desistindo depois de 10 minutos (o áudio fica sem texto, `failed`). A transcrição dos áudios antigos começa 2 minutos depois do servidor subir.
- **Imagem Docker:** o `Dockerfile` compila o whisper.cpp (tag fixa) e embute o modelo `ggml-small-q5_1.bin` (~190 MB, checksum conferido), com `WHISPER_BIN`, `WHISPER_MODEL`
  e `WHISPER_LANG=pt`. A compilação usa `-march=native`: construa a imagem na máquina em que ela vai rodar (na VPS, é o caso). Sem as variáveis, o recurso fica desligado e
  nada mais muda. Ajustes: `WHISPER_THREADS`, `WHISPER_MAX_SECONDS` (padrão 600).
- **Qualidade:** o modelo "small" entende bem o português falado, mas erra nomes próprios, números e ruído forte; trate o texto como apoio, não como registro exato. Para mais
  precisão, troque para o modelo `medium` (mais lento e mais pesado) em `WHISPER_MODEL`/`WHISPER_MODEL_SHA256` no build.
- **Não faz:** áudios enviados pela equipe não são transcritos; a busca de mensagens não procura dentro da transcrição.

