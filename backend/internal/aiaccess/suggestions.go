package aiaccess

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
)

const (
	suggestionTTL      = 24 * time.Hour
	maxSuggestionRunes = 4000
	maxPendingPerUser  = 10
	EventSuggestion    = "ai.suggestion"
)

// Publisher entrega eventos em tempo real (satisfeito por *realtime.Hub).
type Publisher interface {
	Publish(typ string, data any, filter realtime.Filter)
}

// Attach liga o serviço às conversas (validação e envio) e ao tempo real.
func (s *Service) Attach(convs *conversations.Service, pub Publisher) {
	s.convs, s.pub = convs, pub
}

// Suggestion é um rascunho (draft) ou pedido de envio (send) da IA, à espera de uma pessoa.
type Suggestion struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Intent         string    `json:"intent"` // draft | send
	Text           string    `json:"text"`
	ReplyTo        *string   `json:"replyTo,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

var ErrSuggestionGone = errors.New("aiaccess: sugestão não encontrada, já decidida ou expirada")

// CreateSuggestion grava o rascunho/pedido da IA para a pessoa do token aprovar. Nada é enviado ao cliente.
func (s *Service) CreateSuggestion(ctx context.Context, p Principal, convID, intent, text, replyTo string) (Suggestion, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxSuggestionRunes {
		return Suggestion{}, httpx.FieldErrors{"text": "O texto deve ter de 1 a 4000 caracteres."}
	}
	if intent != "draft" && intent != "send" {
		return Suggestion{}, errors.New("intent inválido")
	}
	v := conversations.Viewer{ID: p.UserID, Name: p.Name, Admin: p.Admin}
	c, err := s.convs.Get(ctx, v, convID)
	if err != nil {
		return Suggestion{}, err
	}
	if c.Status == "closed" {
		return Suggestion{}, conversations.ErrClosed
	}
	if c.SnoozedUntil != nil {
		return Suggestion{}, httpx.FieldErrors{"conversation_id": "A conversa está adiada; retome o atendimento antes."}
	}
	if c.Assignee != nil && c.Assignee.ID != p.UserID {
		return Suggestion{}, &conversations.TakenError{Assignee: c.Assignee.Name}
	}
	var reply any
	if replyTo != "" {
		if !httpx.ValidUUID(replyTo) {
			return Suggestion{}, httpx.FieldErrors{"reply_to": "Mensagem respondida inválida."}
		}
		var ok bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages m JOIN conversations c ON c.id = m.conversation_id
			WHERE m.id = $1 AND m.deleted_at IS NULL AND c.contact_id = (SELECT contact_id FROM conversations WHERE id = $2))`, replyTo, convID).Scan(&ok); err != nil {
			return Suggestion{}, err
		}
		if !ok {
			return Suggestion{}, httpx.FieldErrors{"reply_to": "A mensagem respondida não existe nesta conversa."}
		}
		reply = replyTo
	}
	var out Suggestion
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		// Uma sugestão pendente por conversa e pessoa: a nova substitui a anterior.
		if _, err := tx.Exec(ctx, `UPDATE ai_suggestions SET status = 'discarded', decided_at = now()
			WHERE user_id = $1 AND conversation_id = $2 AND status = 'pending'`, p.UserID, convID); err != nil {
			return err
		}
		var pending int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM ai_suggestions WHERE user_id = $1 AND status = 'pending' AND expires_at > now()`, p.UserID).Scan(&pending); err != nil {
			return err
		}
		if pending >= maxPendingPerUser {
			return httpx.FieldErrors{"conversation_id": "Há 10 sugestões da IA esperando você; aprove ou descarte algumas antes."}
		}
		return tx.QueryRow(ctx, `
			INSERT INTO ai_suggestions (user_id, conversation_id, token_id, intent, text, reply_to, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
			RETURNING id::text, conversation_id::text, intent, text, reply_to::text, created_at, expires_at`,
			p.UserID, convID, p.TokenID, intent, text, reply, "24 hours").
			Scan(&out.ID, &out.ConversationID, &out.Intent, &out.Text, &out.ReplyTo, &out.CreatedAt, &out.ExpiresAt)
	})
	if err != nil {
		return Suggestion{}, err
	}
	s.publishFor(p.UserID, out)
	return out, nil
}

func (s *Service) publishFor(userID string, sg Suggestion) {
	if s.pub == nil {
		return
	}
	s.pub.Publish(EventSuggestion, map[string]string{"conversationId": sg.ConversationID, "id": sg.ID},
		func(c *realtime.Client) bool { return c.UserID == userID })
}

// Suggestions lista as pendentes da pessoa (opcionalmente de uma conversa).
func (s *Service) Suggestions(ctx context.Context, userID, convID string) ([]Suggestion, error) {
	if convID != "" && !httpx.ValidUUID(convID) {
		return []Suggestion{}, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, conversation_id::text, intent, text, reply_to::text, created_at, expires_at FROM ai_suggestions
		 WHERE user_id = $1 AND status = 'pending' AND expires_at > now() AND ($2 = '' OR conversation_id = NULLIF($2, '')::uuid)
		 ORDER BY created_at DESC LIMIT 50`, userID, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		var sg Suggestion
		if err := rows.Scan(&sg.ID, &sg.ConversationID, &sg.Intent, &sg.Text, &sg.ReplyTo, &sg.CreatedAt, &sg.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

// decide muda o estado de uma sugestão pendente da pessoa (de 'pending' para `to`). Devolve a sugestão.
func (s *Service) decide(ctx context.Context, userID, id, from, to string) (Suggestion, error) {
	var sg Suggestion
	if !httpx.ValidUUID(id) {
		return sg, ErrSuggestionGone
	}
	err := s.db.QueryRow(ctx, `
		UPDATE ai_suggestions SET status = $4, decided_at = now()
		 WHERE id = $1 AND user_id = $2 AND status = $3 AND expires_at > now()
		RETURNING id::text, conversation_id::text, intent, text, reply_to::text, created_at, expires_at`, id, userID, from, to).
		Scan(&sg.ID, &sg.ConversationID, &sg.Intent, &sg.Text, &sg.ReplyTo, &sg.CreatedAt, &sg.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sg, ErrSuggestionGone
	}
	return sg, err
}

// Discard descarta a sugestão.
func (s *Service) Discard(ctx context.Context, userID, ip, id string) error {
	sg, err := s.decide(ctx, userID, id, "pending", "discarded")
	if err != nil {
		return err
	}
	return audit.Record(ctx, s.db, audit.Entry{ActorID: userID, Action: "ai.suggestion.discard", TargetType: "conversation", TargetID: sg.ConversationID, IP: ip,
		Metadata: map[string]any{"suggestionId": sg.ID, "intent": sg.Intent}})
}

// Use entrega o texto para a pessoa editar no campo de mensagem (a sugestão sai da fila; nada é enviado).
func (s *Service) Use(ctx context.Context, userID, ip, id string) (Suggestion, error) {
	sg, err := s.decide(ctx, userID, id, "pending", "used")
	if err != nil {
		return sg, err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: userID, Action: "ai.suggestion.use", TargetType: "conversation", TargetID: sg.ConversationID, IP: ip,
		Metadata: map[string]any{"suggestionId": sg.ID, "intent": sg.Intent}})
	return sg, nil
}

// Send aprova e envia: é a PESSOA enviando (o texto pode ter sido editado por ela), pelo caminho normal do app —
// assume o atendimento, respeita fila, conversa adiada/encerrada e a assinatura do atendente.
func (s *Service) Send(ctx context.Context, v conversations.Viewer, ip, id, edited string) (conversations.Message, error) {
	// "sending" impede aprovação dupla (duplo clique, duas abas).
	sg, err := s.decide(ctx, v.ID, id, "pending", "sending")
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(edited)
	wasEdited := text != "" && text != sg.Text
	if text == "" {
		text = sg.Text
	}
	in := conversations.SendInput{ID: uuid.NewString(), Text: text}
	if sg.ReplyTo != nil {
		in.ReplyTo = *sg.ReplyTo
	}
	msg, _, err := s.convs.Send(ctx, v, sg.ConversationID, in)
	if err != nil {
		// Falhou (ex.: outra pessoa assumiu, conversa encerrada): volta para pendente para a pessoa decidir de novo.
		_, _ = s.db.Exec(ctx, `UPDATE ai_suggestions SET status = 'pending', decided_at = NULL WHERE id = $1 AND status = 'sending'`, sg.ID)
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `UPDATE ai_suggestions SET status = 'sent', decided_at = now() WHERE id = $1`, sg.ID)
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: v.ID, Action: "ai.suggestion.send", TargetType: "conversation", TargetID: sg.ConversationID, IP: ip,
		Metadata: map[string]any{"suggestionId": sg.ID, "intent": sg.Intent, "edited": wasEdited}})
	return msg, nil
}
