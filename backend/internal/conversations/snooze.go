package conversations

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Snooze adia (ou, com prazo nulo, retoma) a conversa para toda a equipe.
func (s *Service) Snooze(ctx context.Context, v Viewer, id string, until *time.Time) error {
	return s.SnoozeFor(ctx, v, id, until, SnoozeAll)
}

// SnoozeFor preserves the assignment, queue and open conversation. A nil deadline resumes it.
// scope "all": para toda a equipe (quem atende ou gestor); "me": só para quem pediu (qualquer pessoa que vê a conversa).
func (s *Service) SnoozeFor(ctx context.Context, v Viewer, id string, until *time.Time, scope string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	if scope == "" {
		scope = SnoozeAll
	}
	if scope != SnoozeAll && scope != SnoozeMe {
		return httpx.FieldErrors{"scope": "Escolha adiar para você ou para todos."}
	}
	if until != nil && (!until.After(time.Now()) || until.After(time.Now().AddDate(1, 0, 0))) {
		return httpx.FieldErrors{"until": "Escolha um horário futuro, dentro de até um ano."}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var status string
		var assignee *string
		var previous *time.Time
		err := tx.QueryRow(ctx, `SELECT c.status, c.assignee_id, c.snoozed_until
			FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
			WHERE c.id = $3 AND `+forViewer+` FOR UPDATE OF c`, v.ID, v.Admin, id).Scan(&status, &assignee, &previous)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return ErrClosed
		}
		if scope == SnoozeMe {
			return s.snoozePersonal(ctx, tx, v, id, until)
		}
		if !v.CanSupervise() && (assignee == nil || *assignee != v.ID) {
			return ErrForbidden
		}
		if until == nil && previous == nil {
			return nil
		}
		actor := ""
		action := "conversation.resume"
		if until != nil {
			actor, action = v.ID, "conversation.snooze"
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET snoozed_until = $2, snoozed_by = NULLIF($3, '')::uuid WHERE id = $1`, id, until, actor); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: action, TargetType: "conversation", TargetID: id, Metadata: map[string]any{"until": until}})
	})
	if err == nil {
		if scope == SnoozeMe {
			s.publishUser(ctx, id, v.ID)
		} else {
			s.publishState(ctx, id, nil)
		}
	}
	return err
}

// snoozePersonal guarda o adiamento só desta pessoa; a conversa continua igual para as demais.
func (s *Service) snoozePersonal(ctx context.Context, tx pgx.Tx, v Viewer, id string, until *time.Time) error {
	action := "conversation.resume"
	if until != nil {
		action = "conversation.snooze"
		if _, err := tx.Exec(ctx, `INSERT INTO conversation_user_state (conversation_id, user_id, snoozed_until) VALUES ($1, $2, $3)
			ON CONFLICT (conversation_id, user_id) DO UPDATE SET snoozed_until = $3`, id, v.ID, until); err != nil {
			return err
		}
	} else {
		tag, err := tx.Exec(ctx, `UPDATE conversation_user_state SET snoozed_until = NULL WHERE conversation_id = $1 AND user_id = $2 AND snoozed_until IS NOT NULL`, id, v.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
	}
	return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: action, TargetType: "conversation", TargetID: id, Metadata: map[string]any{"until": until, "scope": SnoozeMe}})
}

// ResumeDue processes a bounded batch. Row locks prevent duplicate returns across API instances.
func (s *Service) ResumeDue(ctx context.Context) (int, error) {
	var ids []string
	type personal struct{ conv, user string }
	var mine []personal
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `WITH due AS (
			SELECT id FROM conversations WHERE snoozed_until <= now() AND status <> 'closed'
			ORDER BY snoozed_until LIMIT 100 FOR UPDATE SKIP LOCKED
		) UPDATE conversations c SET snoozed_until = NULL, snoozed_by = NULL
		FROM due WHERE c.id = due.id RETURNING c.id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if err := audit.Record(ctx, tx, audit.Entry{Action: "conversation.resume", TargetType: "conversation", TargetID: id, Metadata: map[string]any{"reason": "scheduled"}}); err != nil {
				return err
			}
		}
		// Adiamentos individuais vencidos: voltam só para quem adiou.
		prows, err := tx.Query(ctx, `WITH due AS (
			SELECT conversation_id, user_id FROM conversation_user_state WHERE snoozed_until <= now()
			ORDER BY snoozed_until LIMIT 100 FOR UPDATE SKIP LOCKED
		) UPDATE conversation_user_state us SET snoozed_until = NULL
		FROM due WHERE us.conversation_id = due.conversation_id AND us.user_id = due.user_id
		RETURNING us.conversation_id::text, us.user_id::text`)
		if err != nil {
			return err
		}
		defer prows.Close()
		for prows.Next() {
			var p personal
			if err := prows.Scan(&p.conv, &p.user); err != nil {
				return err
			}
			mine = append(mine, p)
		}
		return prows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		s.publishState(ctx, id, nil)
	}
	for _, p := range mine {
		s.publishUser(ctx, p.conv, p.user)
	}
	return len(ids) + len(mine), nil
}

// RunSnoozeWorker also catches deadlines missed while the server was offline.
func (s *Service) RunSnoozeWorker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		count, err := s.ResumeDue(batchCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "retomando atendimentos adiados", "err", err)
		}
		if count == 100 && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
