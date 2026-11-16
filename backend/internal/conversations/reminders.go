package conversations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Reminder struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
	Color          string `json:"color"`
}

// Personal reminders are deliberately separate from the shared conversation payload.
func (s *Service) ListReminders(ctx context.Context, v Viewer) ([]Reminder, error) {
	rows, err := s.db.Query(ctx, `SELECT r.conversation_id, r.text, r.color
		FROM conversation_reminders r JOIN conversations c ON c.id = r.conversation_id
		WHERE r.user_id = $1 AND c.assignee_id = $1 AND c.status <> 'closed'`, v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Reminder{}
	for rows.Next() {
		var r Reminder
		if err := rows.Scan(&r.ConversationID, &r.Text, &r.Color); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Service) SaveReminder(ctx context.Context, v Viewer, id, text, color string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > 240 {
		return httpx.FieldErrors{"text": "Use até 240 caracteres."}
	}
	if text != "" {
		switch color {
		case "olive", "amber", "sky", "rose", "violet", "neutral":
		default:
			return httpx.FieldErrors{"color": "Escolha uma das cores disponíveis."}
		}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var status string
		// Even administrators can only edit their own reminders on their own assignments.
		err := tx.QueryRow(ctx, `SELECT status FROM conversations WHERE id=$1 AND assignee_id=$2 FOR UPDATE`, id, v.ID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return ErrClosed
		}
		action := "conversation.reminder.update"
		if text == "" {
			if _, err := tx.Exec(ctx, `DELETE FROM conversation_reminders WHERE user_id=$1 AND conversation_id=$2`, v.ID, id); err != nil {
				return err
			}
			action = "conversation.reminder.delete"
		} else {
			if _, err := tx.Exec(ctx, `INSERT INTO conversation_reminders (user_id, conversation_id, text, color)
				VALUES ($1,$2,$3,$4) ON CONFLICT (user_id, conversation_id)
				DO UPDATE SET text=EXCLUDED.text, color=EXCLUDED.color, updated_at=now()`, v.ID, id, text, color); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: action, TargetType: "conversation", TargetID: id})
	})
	if err == nil && s.pub != nil {
		s.pub.Publish("conversation.reminder.updated", map[string]string{"id": id}, userSet{v.ID: struct{}{}}.filter())
	}
	return err
}

func (h *Handler) reminders(w http.ResponseWriter, r *http.Request) {
	reminders, err := h.svc.ListReminders(r.Context(), viewer(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"reminders": reminders})
}

func (h *Handler) saveReminder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text  string `json:"text"`
		Color string `json:"color"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.SaveReminder(r.Context(), viewer(r), chi.URLParam(r, "id"), in.Text, in.Color); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) deleteReminder(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.SaveReminder(r.Context(), viewer(r), chi.URLParam(r, "id"), "", ""); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// readReminder lê o lembrete pessoal de quem entrega a conversa (nil, nil quando não há).
func readReminder(ctx context.Context, tx pgx.Tx, userID, convID string) (note, color *string, err error) {
	err = tx.QueryRow(ctx, `SELECT text, color FROM conversation_reminders WHERE user_id = $1 AND conversation_id = $2`, userID, convID).Scan(&note, &color)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	return note, color, err
}

// copyReminder dá a quem recebe a conversa uma cópia do lembrete (sem nada a copiar, não faz nada). Substitui qualquer
// lembrete antigo dela nesta conversa; o original de quem entregou é mantido.
func copyReminder(ctx context.Context, tx pgx.Tx, toUser, convID string, note, color *string) error {
	if note == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO conversation_reminders (user_id, conversation_id, text, color)
		VALUES ($1, $2, $3, $4) ON CONFLICT (user_id, conversation_id)
		DO UPDATE SET text = EXCLUDED.text, color = EXCLUDED.color, updated_at = now()`, toUser, convID, note, color)
	return err
}
