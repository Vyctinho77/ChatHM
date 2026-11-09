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

// Snooze preserves the assignment, queue and open conversation. A nil deadline resumes it.
func (s *Service) Snooze(ctx context.Context, v Viewer, id string, until *time.Time) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
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
		s.publishState(ctx, id, nil)
	}
	return err
}

// ResumeDue processes a bounded batch. Row locks prevent duplicate returns across API instances.
func (s *Service) ResumeDue(ctx context.Context) (int, error) {
	var ids []string
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
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		s.publishState(ctx, id, nil)
	}
	return len(ids), nil
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
