package fake

import (
	"context"
	"sync"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// Mídia simulada para testes: a referência (MediaRef.Proto) é o ID externo da mensagem. PutMedia põe o arquivo "no servidor do WhatsApp";
// PhoneHasMedia faz o "celular" reenviá-lo quando pedirem; sem nenhum dos dois, o arquivo "expirou".
type mediaSim struct {
	mu       sync.Mutex
	server   map[string][]byte
	phone    map[string][]byte
	requests []whatsapp.MediaMessage
	retryErr error
	handler  whatsapp.MediaRetryHandler
}

func (p *Provider) media() *mediaSim {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ms == nil {
		p.ms = &mediaSim{server: map[string][]byte{}, phone: map[string][]byte{}}
	}
	return p.ms
}

// PutMedia põe o arquivo no "servidor do WhatsApp".
func (p *Provider) PutMedia(externalID string, data []byte) {
	m := p.media()
	m.mu.Lock()
	m.server[externalID] = data
	m.mu.Unlock()
}

// PhoneHasMedia: o "celular" ainda tem o arquivo e o reenvia quando pedirem.
func (p *Provider) PhoneHasMedia(externalID string, data []byte) {
	m := p.media()
	m.mu.Lock()
	m.phone[externalID] = data
	m.mu.Unlock()
}

// FailMediaRetry faz os pedidos de reenvio falharem (celular sem conexão).
func (p *Provider) FailMediaRetry(err error) {
	m := p.media()
	m.mu.Lock()
	m.retryErr = err
	m.mu.Unlock()
}

// MediaRetryRequests devolve os pedidos de reenvio feitos ao "celular".
func (p *Provider) MediaRetryRequests() []whatsapp.MediaMessage {
	m := p.media()
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]whatsapp.MediaMessage(nil), m.requests...)
}

// RespondMediaRetry simula a resposta do celular a um pedido: reenvia o arquivo se o tiver.
func (p *Provider) RespondMediaRetry(externalID, chatJID string) {
	m := p.media()
	m.mu.Lock()
	h := m.handler
	m.mu.Unlock()
	if h == nil {
		return
	}
	h(whatsapp.MediaRetryEvent{MessageID: externalID, ChatJID: chatJID, Complete: func(_ context.Context, ref whatsapp.MediaRef) ([]byte, error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if data, ok := m.phone[string(ref.Proto)]; ok {
			return data, nil
		}
		return nil, whatsapp.ErrMediaNotOnPhone
	}})
}

func (p *Provider) FetchMedia(_ context.Context, ref whatsapp.MediaRef) ([]byte, error) {
	m := p.media()
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.server[string(ref.Proto)]; ok {
		return data, nil
	}
	return nil, whatsapp.ErrMediaGone
}

func (p *Provider) RequestMediaRetry(_ context.Context, _ whatsapp.MediaRef, msg whatsapp.MediaMessage) error {
	m := p.media()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.retryErr != nil {
		return m.retryErr
	}
	m.requests = append(m.requests, msg)
	return nil
}

func (p *Provider) SetMediaRetryHandler(h whatsapp.MediaRetryHandler) {
	m := p.media()
	m.mu.Lock()
	m.handler = h
	m.mu.Unlock()
}
