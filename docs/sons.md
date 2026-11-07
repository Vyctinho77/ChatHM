# Efeitos sonoros

Os arquivos originais estão em `sonoros/`. As cópias usadas no build ficam em
`apps/web/src/assets/sounds/`, com nomes sem espaços e com cache por hash do Vite.

| Arquivo original | Uso |
|---|---|
| `Enviar mensagem novo.mp3` | Envio confirmado pelo servidor |
| `receber mensagem novo.mp3` | Nova mensagem na conversa aberta, com a aba em foco |
| `efeito para botão de audio.mp3` | Clique no botão de gravar áudio (respeita sons da conversa e volume) |
| `padrão.mp3` | Notificação padrão para mensagens fora da conversa aberta |
| `águas.mp3` | Tom de notificação Águas |
| `Noites.mp3` | Tom de notificação Noites |
| `Hipinotico.mp3` | Tom de notificação Hipnótico |

Em **Configurações → Sons e notificações**, é possível ligar ou desligar os sons
da conversa e de notificação separadamente, escolher o tom, ouvir uma prévia e
ajustar o volume. As preferências são salvas por conta neste navegador.

O navegador libera o áudio após uma interação com o app. O `AudioContext` é
reutilizado e os arquivos são baixados e decodificados apenas quando necessários.
Falhas de áudio não interrompem mensagens nem geram erros na conversa.

Histórico, carregamento inicial, recibos e eventos duplicados não disparam sons.
Envios com falha ou pendentes também não. Rajadas têm um intervalo mínimo e os
IDs recentes evitam repetição; com Web Locks e armazenamento disponíveis, abas do
mesmo navegador compartilham a deduplicação. O som não é uma notificação do sistema
operacional e não funciona com o app totalmente fechado.

Referências: [autoplay](https://developer.mozilla.org/en-US/docs/Web/Media/Guides/Autoplay)
e [AudioContext.resume](https://developer.mozilla.org/en-US/docs/Web/API/AudioContext/resume).
