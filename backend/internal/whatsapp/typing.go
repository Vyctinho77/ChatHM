package whatsapp

import (
	"context"
	"time"
)

func (m *Manager) SetTypingHandler(h func(context.Context, Typing) error) { m.onTyping = h }

func (m *Manager) OnTyping(update Typing) {
	if m.onTyping == nil || !m.Connected() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.onTyping(ctx, update); err != nil {
		m.log.Debug("processando indicador de digitação", "err", err)
	}
}
