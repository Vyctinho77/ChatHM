# Indicador de digitação

Quando o WhatsApp informa que o cliente está escrevendo, a prévia da conversa e o
subtítulo no cabeçalho mostram **Digitando…**. O texto tem um gradiente neutro em
movimento e os três pontos entram e recolhem em sequência, sem alterar a largura.
Com redução de movimento ativada no sistema, o indicador é estático.

O adaptador usa `events.ChatPresence` do whatsmeow. Ao conectar, envia
`PresenceAvailable`, necessário para receber esses eventos; a conta do escritório
passa a se apresentar como online. A disponibilidade do sinal depende do WhatsApp.
Eventos do próprio escritório, status e canais são ignorados. Gravação de áudio
não é apresentada como digitação de texto. Contatos por LID são resolvidos para
telefone quando o vínculo está disponível, assim como na importação de mensagens.

O evento `conversation.typing` é enviado apenas a quem pode ver a conversa.
Não cria contatos, mensagens, atendimentos, notificações ou registros no banco,
nem muda a fila, a atribuição, os contadores ou o adiamento da conversa.

O estado existe apenas na memória do navegador. Some ao receber a pausa ou a
mensagem do participante, ao perder acesso à conversa, ao desconectar ou após
10 segundos sem atualização. Em grupos, cada participante tem seu próprio prazo;
a pausa de uma pessoa não esconde o indicador de outra que ainda está escrevendo.

No provedor simulado, use **Conectar → Testar uma conversa → Testar digitando**.
Informe o telefone de um contato com conversa existente (ou envie primeiro uma
mensagem de teste). A rota `POST /api/v1/dev/whatsapp/typing` recebe `{ "phone": "..." }`,
exige administrador e CSRF, e só existe com o simulador habilitado.

Referência: https://pkg.go.dev/go.mau.fi/whatsmeow/types/events#ChatPresence
