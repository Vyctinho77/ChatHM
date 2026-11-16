package whatsapp

import (
	"context"
	"errors"
)

// Referência de mídia: o que o WhatsApp entrega junto da mensagem para baixar o arquivo (endereço, chave e hashes). O histórico traz a
// referência mesmo quando o arquivo não é baixado na hora; guardada (cifrada), ela permite baixar mais tarde e, se o WhatsApp já apagou
// o arquivo dos servidores, pedir ao celular que o envie de novo.

// MediaRef é a referência de uma mídia (imagem, áudio, documento ou figurinha).
type MediaRef struct {
	Kind  Kind   // image | audio | document | sticker
	Proto []byte // conteúdo opaco do provedor (no whatsmeow, a mensagem de mídia serializada, sem a miniatura)
}

// ErrMediaGone: o arquivo não está mais nos servidores do WhatsApp (expirou). Dá para pedir ao celular que o envie de novo.
var ErrMediaGone = errors.New("whatsapp: o arquivo não está mais no servidor do WhatsApp")

// ErrMediaNotOnPhone: o celular também não tem mais o arquivo. Não adianta tentar de novo.
var ErrMediaNotOnPhone = errors.New("whatsapp: o celular não tem mais o arquivo")

// MediaMessage identifica a mensagem de uma mídia, para pedir o reenvio ao celular.
type MediaMessage struct {
	ExternalID string
	ChatJID    string
	SenderJID  string // grupos: quem escreveu
	FromMe     bool
	IsGroup    bool
}

// MediaFetcher é o que um provedor oferece para (re)baixar mídia a partir da referência guardada. Opcional.
type MediaFetcher interface {
	// FetchMedia baixa o arquivo. Devolve ErrMediaGone se ele já não está nos servidores.
	FetchMedia(ctx context.Context, ref MediaRef) ([]byte, error)
	// RequestMediaRetry pede ao celular que envie o arquivo de novo; a resposta chega depois, como MediaRetryEvent.
	RequestMediaRetry(ctx context.Context, ref MediaRef, msg MediaMessage) error
}

// MediaRetryEvent é a resposta do celular a um pedido de reenvio. Complete termina o trabalho com a referência guardada: devolve o arquivo,
// ErrMediaNotOnPhone (o celular não tem) ou ErrMediaGone (o reenvio falhou).
type MediaRetryEvent struct {
	MessageID string // ID externo da mensagem
	ChatJID   string
	Complete  func(ctx context.Context, ref MediaRef) ([]byte, error)
}

// MediaRetryHandler recebe as respostas do celular.
type MediaRetryHandler func(MediaRetryEvent)

// MediaRetryNotifier: provedores que avisam as respostas de reenvio.
type MediaRetryNotifier interface{ SetMediaRetryHandler(MediaRetryHandler) }
