// Package whatsapp define a integração com o WhatsApp desacoplada do provedor.
//
// O sistema conversa apenas com a interface Provider. O primeiro adaptador
// (pacote wameow) usa whatsmeow — sessão espelhada por QR Code, não oficial e
// sujeita a instabilidade/restrições. Um adaptador para a WhatsApp Business
// Platform oficial pode ser adicionado sem mudar o restante do sistema; o fluxo
// de conexão dela é diferente (cadastro do número na Meta, sem QR Code).
package whatsapp

import (
	"context"
	"errors"
	"time"
)

type State string

const (
	StateDisconnected State = "disconnected"
	StatePendingQR    State = "pending_qr"
	StateConnected    State = "connected"
	StateError        State = "error"
)

// Account identifica o número conectado.
type Account struct {
	Phone     string // E.164, ex.: +5592999990000
	Name      string // nome do perfil no WhatsApp
	DeviceJID string
}

type Kind string

const (
	KindText        Kind = "text"
	KindAudio       Kind = "audio"
	KindImage       Kind = "image"
	KindDocument    Kind = "document"
	KindLocation    Kind = "location"
	KindUnsupported Kind = "unsupported"
)

// InboundMessage é uma mensagem recebida (ou enviada pelo próprio celular) já
// normalizada, independente do provedor.
type InboundMessage struct {
	ExternalID   string // ID da mensagem no provedor (deduplicação)
	ChatJID      string // conversa 1:1 no provedor
	ContactWAID  string // identificador estável do contato (número ou LID)
	ContactPhone string // E.164 quando conhecido
	PushName     string
	FromMe       bool // enviada pelo celular do escritório
	Historical   bool // importada do histórico; não incrementa não lidas
	Timestamp    time.Time
	Kind         Kind
	Body         string         // texto ou legenda
	Payload      map[string]any // metadados (mídia, localização); nunca o binário
	// Download baixa o arquivo da mídia (nil quando a mensagem não tem arquivo).
	Download func(ctx context.Context) ([]byte, error) `json:"-"`
}

// MediaSend é um anexo a enviar (já validado e normalizado).
type MediaSend struct {
	Kind        Kind // image | audio | document
	Data        []byte
	MimeType    string
	FileName    string
	Caption     string
	DurationSec int
	Waveform    []byte // 0–100 por barra (opcional)
	Width       int
	Height      int
}

// Location é uma localização estática.
type Location struct {
	Lat, Lng      float64
	Name, Address string
}

// ReceiptStatus é o avanço de uma mensagem enviada, informado pelo WhatsApp.
type ReceiptStatus string

const (
	ReceiptDelivered ReceiptStatus = "delivered"
	ReceiptRead      ReceiptStatus = "read"
)

// Receipt é um recibo de entrega ou leitura de mensagens que o escritório enviou.
type Receipt struct {
	MessageIDs []string
	Status     ReceiptStatus
	Timestamp  time.Time
}

// Handler recebe os eventos do provedor. Implementado pelo Manager.
type Handler interface {
	OnConnected(Account)
	OnDisconnected(reason string, loggedOut bool)
	OnMessage(InboundMessage)
	OnReceipt(Receipt)
}

// PairEvent é emitido durante o pareamento por QR Code.
type PairEvent struct {
	QR      string        // novo QR Code a exibir
	Expires time.Duration // validade do QR atual
	Success bool
	Timeout bool // QR Codes esgotados sem leitura
	Err     error
}

var (
	ErrNoSession    = errors.New("whatsapp: nenhuma sessão salva")
	ErrNotConnected = errors.New("whatsapp: não conectado")
)

type Provider interface {
	// HasSession informa se há uma sessão salva para reconectar.
	HasSession(ctx context.Context) (bool, error)
	// Restore reconecta usando a sessão salva.
	Restore(ctx context.Context) error
	// Pair inicia uma nova sessão e transmite os QR Codes até concluir.
	Pair(ctx context.Context) (<-chan PairEvent, error)
	// Logout desvincula o aparelho e apaga a sessão local.
	Logout(ctx context.Context) error
	// Close encerra a conexão sem apagar a sessão.
	Close()
	// NewMessageID gera o ID de uma mensagem a enviar. Reenviar com o mesmo ID é seguro:
	// o WhatsApp descarta a duplicata.
	NewMessageID() string
	// SendText envia texto para uma conversa 1:1 usando o ID informado.
	SendText(ctx context.Context, chatJID, messageID, text string) error
	// SendMedia envia áudio (mensagem de voz), imagem ou documento.
	SendMedia(ctx context.Context, chatJID, messageID string, m MediaSend) error
	// SendLocation envia uma localização estática.
	SendLocation(ctx context.Context, chatJID, messageID string, l Location) error
}

// Factory cria o provedor ligado ao handler de eventos.
type Factory func(h Handler) Provider

type HistoryContact struct {
	WAID, Phone, Name string
}

type HistoryProgress struct {
	State    string `json:"state"`
	Messages int    `json:"messages"`
	Contacts int    `json:"contacts"`
	Failed   int    `json:"failed"`
}

// HistoryHandler imports the chunks supplied by WhatsApp independently from
// the live connection event handler.
type HistoryHandler interface {
	OnHistoryMessage(InboundMessage) (bool, error)
	OnHistoryContact(HistoryContact) error
	OnHistoryProgress(HistoryProgress)
}
