// Package fake implementa um Provider em memória para testes e desenvolvimento
// (WHATSAPP_PROVIDER=fake), permitindo simular pareamento e mensagens sem celular.
package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

type Provider struct {
	h whatsapp.Handler

	// AutoPairAfter: se > 0, conclui o pareamento sozinho após esse tempo (modo dev).
	AutoPairAfter time.Duration
	Account       whatsapp.Account

	mu      sync.Mutex
	session bool
	events  chan whatsapp.PairEvent
	sent    []Sent
	sendErr error
	// connected espelha o estado do "celular": só envia depois de conectado.
	connected bool
}

// Sent é uma mensagem de texto enviada pelo sistema (para asserções em testes).
type Sent struct {
	ChatJID, MessageID, Text string
	Media                    *whatsapp.MediaSend
	Location                 *whatsapp.Location
}

func New(h whatsapp.Handler) *Provider {
	return &Provider{h: h, Account: whatsapp.Account{Phone: "+5511900000000", Name: "Escritório (simulado)", DeviceJID: "fake@s.whatsapp.net"}}
}

// Factory devolve uma whatsapp.Factory; o callback permite ao teste capturar a instância.
func Factory(autoPair time.Duration, created func(*Provider)) whatsapp.Factory {
	return func(h whatsapp.Handler) whatsapp.Provider {
		p := New(h)
		p.AutoPairAfter = autoPair
		if created != nil {
			created(p)
		}
		return p
	}
}

func (p *Provider) HasSession(context.Context) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session, nil
}

func (p *Provider) Restore(context.Context) error {
	p.mu.Lock()
	ok := p.session
	p.mu.Unlock()
	if !ok {
		return whatsapp.ErrNoSession
	}
	p.mu.Lock()
	p.connected = true
	p.mu.Unlock()
	go p.h.OnConnected(p.Account)
	return nil
}

func (p *Provider) Pair(ctx context.Context) (<-chan whatsapp.PairEvent, error) {
	ch := make(chan whatsapp.PairEvent, 8)
	p.mu.Lock()
	p.events = ch
	p.mu.Unlock()
	ch <- whatsapp.PairEvent{QR: "chathm-fake-qr-" + time.Now().Format("150405"), Expires: 60 * time.Second}
	if p.AutoPairAfter > 0 {
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(p.AutoPairAfter):
				p.CompletePairing()
			}
		}()
	}
	return ch, nil
}

// CompletePairing simula a leitura do QR Code no celular.
func (p *Provider) CompletePairing() {
	p.mu.Lock()
	p.session = true
	p.connected = true
	ch := p.events
	p.events = nil
	p.mu.Unlock()
	p.h.OnConnected(p.Account)
	if ch != nil {
		ch <- whatsapp.PairEvent{Success: true}
		close(ch)
	}
}

// CompletePairingSilently marca a sessão como salva sem emitir eventos
// (estado de um servidor que reinicia com sessão válida).
func (p *Provider) CompletePairingSilently() {
	p.mu.Lock()
	p.session = true
	p.mu.Unlock()
}

// ExpirePairing simula o esgotamento dos QR Codes.
func (p *Provider) ExpirePairing() {
	p.mu.Lock()
	ch := p.events
	p.events = nil
	p.mu.Unlock()
	if ch != nil {
		ch <- whatsapp.PairEvent{Timeout: true}
		close(ch)
	}
}

// Drop simula queda de conexão (loggedOut=true: desvinculado pelo celular).
func (p *Provider) Drop(loggedOut bool) {
	p.mu.Lock()
	p.connected = false
	p.mu.Unlock()
	if loggedOut {
		p.mu.Lock()
		p.session = false
		p.mu.Unlock()
	}
	p.h.OnDisconnected("simulado", loggedOut)
}

// Receive entrega uma mensagem como se viesse do WhatsApp.
func (p *Provider) Receive(m whatsapp.InboundMessage) { p.h.OnMessage(m) }

func (p *Provider) Logout(context.Context) error {
	p.mu.Lock()
	p.session = false
	p.connected = false
	p.mu.Unlock()
	return nil
}

func (p *Provider) Close() {}

func (p *Provider) NewMessageID() string {
	// Development messages survive process restarts in PostgreSQL.
	return "FAKE-" + uuid.NewString()
}

func (p *Provider) SendText(_ context.Context, chatJID, messageID, text string) error {
	return p.record(Sent{ChatJID: chatJID, MessageID: messageID, Text: text})
}

func (p *Provider) SendMedia(_ context.Context, chatJID, messageID string, m whatsapp.MediaSend) error {
	return p.record(Sent{ChatJID: chatJID, MessageID: messageID, Text: m.Caption, Media: &m})
}

func (p *Provider) SendLocation(_ context.Context, chatJID, messageID string, l whatsapp.Location) error {
	return p.record(Sent{ChatJID: chatJID, MessageID: messageID, Location: &l})
}

func (p *Provider) record(s Sent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.connected {
		return whatsapp.ErrNotConnected
	}
	if p.sendErr != nil {
		return p.sendErr
	}
	// Reenvio com o mesmo ID não duplica (como no WhatsApp).
	for _, x := range p.sent {
		if x.MessageID == s.MessageID {
			return nil
		}
	}
	p.sent = append(p.sent, s)
	return nil
}

// Sent devolve uma cópia das mensagens enviadas.
func (p *Provider) Sent() []Sent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Sent(nil), p.sent...)
}

// FailSends faz os próximos envios falharem (nil restaura o normal).
func (p *Provider) FailSends(err error) {
	p.mu.Lock()
	p.sendErr = err
	p.mu.Unlock()
}

// SimulateFailure é um erro de envio genérico para testes.
var SimulateFailure = errors.New("falha simulada no envio")

// Receipt simula um recibo de entrega/leitura vindo do WhatsApp.
func (p *Provider) Receipt(status whatsapp.ReceiptStatus, ids ...string) {
	p.h.OnReceipt(whatsapp.Receipt{MessageIDs: ids, Status: status, Timestamp: time.Now()})
}

// ProfilePicture: o provedor simulado não tem fotos.
func (p *Provider) ProfilePicture(context.Context, string, bool) ([]byte, error) { return nil, nil }
