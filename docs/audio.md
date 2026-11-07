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
