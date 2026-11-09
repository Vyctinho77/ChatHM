package automation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

// Run: o worker da fila. Um envio por vez, no ritmo do escritório (intervalo aleatório, tetos por hora e por dia, janela de horário).
func (s *Service) Run(ctx context.Context) {
	s.recoverStuck(ctx)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// recoverStuck: um envio que estava "enviando" quando o servidor caiu pode ou não ter saído. Nunca reenvia sozinho
// (duplicaria uma cobrança): marca como falho e pede conferência na conversa.
func (s *Service) recoverStuck(ctx context.Context) {
	tag, err := s.db.Exec(ctx, `UPDATE automation_sends SET status = 'failed', updated_at = now(),
		status_detail = 'Interrompido no meio do envio (reinício do servidor). Confira a conversa do cliente antes de tentar de novo.' WHERE status = 'sending'`)
	if err == nil && tag.RowsAffected() > 0 {
		s.log.Warn("envios automáticos interrompidos marcados como falhos", "n", tag.RowsAffected())
	}
}

// Tick faz uma volta da fila e devolve quantas mensagens saíram (0 ou 1).
func (s *Service) Tick(ctx context.Context) int {
	if on, err := s.masterOn(ctx); err != nil || !on {
		return 0
	}
	st, err := s.Settings(ctx)
	if err != nil || st.Paused || !s.convs.CanSend() {
		return 0
	}
	now := s.now()
	var next time.Time
	if err := s.db.QueryRow(ctx, `SELECT next_allowed_at FROM automation_state WHERE id`).Scan(&next); err != nil || now.Before(next) {
		return 0
	}
	var hour, day int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sent_at > $1::timestamptz - interval '1 hour'), count(*) FILTER (WHERE sent_at > $1::timestamptz - interval '24 hours')
		  FROM automation_sends WHERE status = 'sent'`, now).Scan(&hour, &day); err != nil {
		return 0
	}
	if hour >= st.HourlyCap || day >= st.DailyCap {
		return 0
	}
	cats, err := s.Categories(ctx)
	if err != nil {
		return 0
	}
	byKey := map[string]Category{}
	for _, c := range cats {
		byKey[c.Key] = c
	}
	rows, err := s.db.Query(ctx, `SELECT id::text, category FROM automation_sends WHERE status = 'approved' AND scheduled_for <= $1 ORDER BY scheduled_for, created_at LIMIT 25`, now)
	if err != nil {
		return 0
	}
	type cand struct{ id, cat string }
	var list []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.cat); err == nil {
			list = append(list, c)
		}
	}
	rows.Close()
	for _, c := range list {
		cat, ok := byKey[c.cat]
		if !ok || !cat.Enabled || !s.inWindow(cat, st, now) {
			continue // fora da janela ou categoria desligada: espera na fila
		}
		switch s.process(ctx, c.id, st) {
		case outcomeSent, outcomeFailed:
			gap := st.GapMinSeconds
			if span := st.GapMaxSeconds - st.GapMinSeconds; span > 0 {
				gap += s.rnd(span + 1)
			}
			_, _ = s.db.Exec(ctx, `UPDATE automation_state SET next_allowed_at = $1 WHERE id`, s.now().Add(time.Duration(gap)*time.Second))
			return 1
		case outcomeWait:
			return 0
		}
		// outcomeSkipped: nada saiu; segue para o próximo candidato
	}
	return 0
}

type outcome int

const (
	outcomeSkipped outcome = iota // recusado pelas regras (o cliente não recebe)
	outcomeSent
	outcomeFailed
	outcomeWait // o WhatsApp não está disponível: volta para a fila e espera
)

func (s *Service) mark(ctx context.Context, id, status, detail string) {
	_, _ = s.db.Exec(ctx, `UPDATE automation_sends SET status = $2, status_detail = $3, updated_at = now() WHERE id = $1`, id, status, detail)
}

func (s *Service) record(ctx context.Context, action, id string, meta map[string]any) {
	_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.Entry{Action: action, TargetType: "automation_send", TargetID: id, Metadata: meta})
	})
}

// process reivindica e envia um item. As regras do servidor rodam DE NOVO aqui: o mundo mudou desde que a IA enfileirou.
func (s *Service) process(ctx context.Context, id string, st Settings) outcome {
	var contactID, text, category, docID, provider, app string
	err := s.db.QueryRow(ctx, `
		UPDATE automation_sends SET status = 'sending', attempts = attempts + 1, updated_at = now() WHERE id = $1 AND status = 'approved'
		RETURNING contact_id::text, text, category, coalesce(document_id::text, ''), provider, app_name`, id).Scan(&contactID, &text, &category, &docID, &provider, &app)
	if err != nil {
		return outcomeSkipped // outro worker pegou, ou foi cancelado
	}
	info, ref, err := s.guardContact(ctx, contactID, st)
	if err != nil {
		s.mark(ctx, id, StatusApproved, "") // erro de banco: volta para a fila
		return outcomeWait
	}
	if ref != nil {
		s.mark(ctx, id, StatusSkipped, ref.Message)
		s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
		return outcomeSkipped
	}
	if _, ref, _ := s.guardTextNow(text, st); ref != nil {
		s.mark(ctx, id, StatusSkipped, ref.Message)
		s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
		return outcomeSkipped
	}
	var att *media.Processed
	if docID != "" {
		d, ref, derr := s.guardDocument(ctx, docID, contactID)
		if derr != nil {
			s.mark(ctx, id, StatusApproved, "")
			return outcomeWait
		}
		if ref != nil {
			s.mark(ctx, id, StatusSkipped, ref.Message)
			s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
			return outcomeSkipped
		}
		var key string
		if err := s.db.QueryRow(ctx, `SELECT storage_key FROM automation_documents WHERE id = $1`, docID).Scan(&key); err != nil {
			s.mark(ctx, id, StatusFailed, "O documento não está mais na biblioteca.")
			return outcomeFailed
		}
		data, err := s.store.Get(ctx, key)
		if err != nil {
			s.mark(ctx, id, StatusFailed, "Não foi possível ler o documento da biblioteca.")
			return outcomeFailed
		}
		p, err := s.media.ProcessInbound(ctx, media.Document, data, d.Name, "application/pdf") // valida e varre de novo
		if err != nil {
			s.mark(ctx, id, StatusFailed, "O documento foi recusado pela validação de segurança.")
			return outcomeFailed
		}
		att = &p
	}
	msgID := uuid.NewString()
	msg, err := s.convs.SendAutomated(ctx, conversations.AutomatedSend{
		ID: msgID, ContactID: contactID, Author: Author(st.Signature), Text: Render(text, info.Name, st.OfficeFields),
		Att: att, Provider: provider, App: app, SendID: id,
	})
	var blocked *conversations.ErrAutomatedBlocked
	switch {
	case errors.Is(err, conversations.ErrUnavailable):
		s.mark(ctx, id, StatusApproved, "") // WhatsApp fora do ar: espera
		return outcomeWait
	case errors.As(err, &blocked):
		s.mark(ctx, id, StatusSkipped, "Bloqueado: "+blocked.Reason+".")
		s.record(ctx, "automation.skipped", id, map[string]any{"reason": blocked.Reason})
		return outcomeSkipped
	case err != nil:
		_, _ = s.db.Exec(ctx, `UPDATE automation_sends SET status = 'failed', status_detail = $2, message_id = NULLIF($3, '')::uuid, updated_at = now() WHERE id = $1`,
			id, "Não foi possível enviar pelo WhatsApp.", func() string {
				if msg != nil {
					return msgID
				}
				return ""
			}())
		s.record(ctx, "automation.failed", id, map[string]any{"contactId": contactID})
		s.checkFailureStreak(ctx)
		return outcomeFailed
	}
	_, _ = s.db.Exec(ctx, `UPDATE automation_sends SET status = 'sent', sent_at = $2, message_id = $3::uuid, status_detail = '', updated_at = now() WHERE id = $1`, id, s.now(), msgID)
	s.record(ctx, "automation.sent", id, map[string]any{"contactId": contactID, "category": category, "provider": provider, "hasDocument": docID != ""})
	return outcomeSent
}

// guardTextNow: revalida o texto na hora de enviar (as regras do administrador podem ter mudado desde a aprovação).
func (s *Service) guardTextNow(text string, st Settings) ([]string, *Refusal, error) {
	flags, ref := ValidateText(text, st.AllowedDomains, st.OfficeFields)
	return flags, ref, nil
}

// checkFailureStreak: cinco falhas seguidas pausam tudo (WhatsApp com problema, número restrito...). Quem decide religar é o administrador.
func (s *Service) checkFailureStreak(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT status FROM automation_sends WHERE status IN ('sent', 'failed') ORDER BY updated_at DESC LIMIT 5`)
	if err != nil {
		return
	}
	defer rows.Close()
	n, failed := 0, 0
	for rows.Next() {
		var st string
		if rows.Scan(&st) == nil {
			n++
			if st == StatusFailed {
				failed++
			}
		}
	}
	if n == 5 && failed == 5 {
		s.autoPause(ctx, "Cinco envios seguidos falharam. Confira a conexão do WhatsApp antes de religar.")
	}
}
