package conversations

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

type TypingEvent struct {
	ConversationID string    `json:"conversationId"`
	SenderID       string    `json:"senderId"`
	Active         bool      `json:"active"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func (s *Service) ApplyTyping(ctx context.Context, update whatsapp.Typing) error {
	if s.pub == nil || update.ContactWAID == "" || update.ChatJID == "" || update.SenderID == "" {
		return nil
	}
	var id string
	err := s.db.QueryRow(ctx, `SELECT c.id FROM conversations c JOIN contacts ct ON ct.id=c.contact_id
		WHERE ct.wa_id = ANY($1::text[]) AND c.status <> 'closed'
		ORDER BY c.last_message_at DESC LIMIT 1`, []string{update.ContactWAID, update.ChatJID}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	users, err := s.visibleUsers(ctx, s.db, id)
	if err != nil {
		return err
	}
	s.publishTyping(id, update.SenderID, update.Active, users)
	return nil
}

func (s *Service) publishTyping(id, senderID string, active bool, users userSet) {
	if s.pub == nil || len(users) == 0 {
		return
	}
	if senderID == "" {
		senderID = "contact"
	}
	s.pub.Publish("conversation.typing", TypingEvent{ConversationID: id, SenderID: senderID, Active: active, ExpiresAt: time.Now().Add(10 * time.Second)}, users.filter())
}
