package conversations

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// Reações às mensagens (como no WhatsApp). O escritório tem uma conta só no WhatsApp: todas as reações dele (feitas aqui ou no
// celular) são uma só por mensagem, com reactor = "office". Do cliente (ou de cada integrante de um grupo) é o identificador dele.
const officeReactor = "office"

// validReaction: um emoji (ou sequência de emoji: bandeira, tom de pele, família), nunca texto comum.
func validReaction(e string) bool {
	if e == "" || len(e) > 24 || utf8.RuneCountInString(e) > 10 || !utf8.ValidString(e) {
		return false
	}
	nonASCII := false
	for _, r := range e {
		if r >= 0x80 {
			nonASCII = true
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
		if r < 0x80 && !strings.ContainsRune("#*0123456789", r) { // letras e símbolos ASCII não são emoji (dígito e #/* só em "keycap": 1️⃣)
			return false
		}
	}
	return nonASCII
}

// canReact: quem pode responder na conversa pode reagir (a mesma regra da tela: outra pessoa atendendo bloqueia, conversa encerrada também).
func canReact(v Viewer, c Conversation) bool {
	if c.Status == "closed" {
		return false
	}
	if c.Assignee == nil || c.Assignee.ID == v.ID {
		return true
	}
	for _, p := range c.Participants {
		if p.ID == v.ID {
			return true
		}
	}
	return false
}

// React reage a uma mensagem em nome do escritório. emoji vazio remove a reação. Devolve a mensagem atualizada.
func (s *Service) React(ctx context.Context, v Viewer, msgID, emoji string) (Message, error) {
	emoji = strings.TrimSpace(emoji)
	if emoji != "" && !validReaction(emoji) {
		return nil, httpx.FieldErrors{"emoji": "Escolha um emoji."}
	}
	r, c, err := s.loadMessage(ctx, v, msgID)
	if err != nil {
		return nil, err
	}
	if r.deleted {
		return nil, httpx.FieldErrors{"emoji": "Esta mensagem foi apagada."}
	}
	if r.status != nil && (*r.status == "failed" || *r.status == "pending") {
		return nil, httpx.FieldErrors{"emoji": "Aguarde a mensagem ser enviada para reagir."}
	}
	if !canReact(v, c) {
		return nil, ErrForbidden
	}
	if s.sender == nil || !s.sender.Connected() {
		return nil, ErrUnavailable
	}
	var senderJID string
	if r.dir == "in" && strings.HasSuffix(r.chatJID, "@g.us") {
		// Em grupo o WhatsApp precisa saber quem escreveu a mensagem reagida (guardado só para isso).
		_ = s.db.QueryRow(ctx, `SELECT coalesce(payload->>'senderJid', '') FROM messages WHERE id = $1`, msgID).Scan(&senderJID)
		if senderJID == "" {
			return nil, httpx.FieldErrors{"emoji": "Não foi possível reagir a esta mensagem."}
		}
	}
	if err := s.sender.SendReaction(ctx, r.chatJID, r.ext, r.dir == "out", senderJID, emoji); err != nil {
		return nil, httpx.FieldErrors{"emoji": "Não foi possível reagir pelo WhatsApp."}
	}
	if err := s.setReaction(ctx, msgID, officeReactor, v.ID, emoji); err != nil {
		return nil, err
	}
	after := s.publishState(ctx, r.convID, nil)
	s.publishMessage(ctx, EventMessageUpdated, msgID, after)
	return s.message(ctx, msgID)
}

// setReaction grava (ou, com emoji vazio, remove) a reação de uma pessoa a uma mensagem.
func (s *Service) setReaction(ctx context.Context, msgID, reactor, userID, emoji string) error {
	if emoji == "" {
		_, err := s.db.Exec(ctx, `DELETE FROM message_reactions WHERE message_id = $1 AND reactor = $2`, msgID, reactor)
		return err
	}
	var uid *string
	if userID != "" {
		uid = &userID
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO message_reactions (message_id, reactor, emoji, user_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (message_id, reactor) DO UPDATE SET emoji = EXCLUDED.emoji, user_id = EXCLUDED.user_id, updated_at = now()`,
		msgID, reactor, emoji, uid)
	return err
}

// ingestReaction aplica uma reação (ou remoção) vinda do WhatsApp: do cliente, ou do escritório pelo celular.
func (s *Service) ingestReaction(ctx context.Context, m whatsapp.InboundMessage) error {
	if m.Reaction != "" && !validReaction(m.Reaction) {
		return nil
	}
	var msgID, convID string
	err := s.db.QueryRow(ctx, `SELECT id::text, conversation_id::text FROM messages WHERE `+sameMessage("")+` AND deleted_at IS NULL`,
		m.ChatJID, m.ReactTo).Scan(&msgID, &convID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // mensagem desconhecida (anterior ao ChatHM)
	}
	if err != nil {
		return err
	}
	reactor := officeReactor
	if !m.FromMe {
		reactor = m.ContactWAID
		if m.IsGroup && m.SenderJID != "" {
			reactor = m.SenderJID
		}
	}
	if err := s.setReaction(ctx, msgID, reactor, "", m.Reaction); err != nil {
		return err
	}
	if m.Historical {
		return nil
	}
	after := s.publishState(ctx, convID, nil)
	s.publishMessage(ctx, EventMessageUpdated, msgID, after)
	return nil
}
