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
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
			if n%12 == 0 { // a cada minuto vê se algum gateway está na hora de sincronizar
				s.SyncDue(ctx)
			}
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
	var contactID, text, category, docID, provider, app, tokenID, chargeID, creator string
	var manual bool
	var snapAmount *int64
	var snapDue *time.Time
	var attempts int
	err := s.db.QueryRow(ctx, `
		UPDATE automation_sends SET status = 'sending', attempts = attempts + 1, updated_at = now() WHERE id = $1 AND status = 'approved'
		RETURNING contact_id::text, text, category, coalesce(document_id::text, ''), provider, app_name, coalesce(token_id::text, ''), approved_by IS NOT NULL,
		          coalesce(charge_id::text, ''), charge_amount_cents, charge_due_date, attempts, coalesce(created_by::text, '')`, id).
		Scan(&contactID, &text, &category, &docID, &provider, &app, &tokenID, &manual, &chargeID, &snapAmount, &snapDue, &attempts, &creator)
	if err != nil {
		return outcomeSkipped // outro worker pegou, ou foi cancelado
	}
	var result outcome
	var ready bool
	if st, result, ready = s.dispatchPolicy(ctx, id, category, text, tokenID, manual, chargeID != ""); !ready {
		return result
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
	cat, err := s.category(ctx, category)
	if err != nil {
		s.mark(ctx, id, StatusApproved, "")
		return outcomeWait
	}
	if ref, err := s.clientLimits(ctx, s.db, contactID, cat, st, id); err != nil {
		s.mark(ctx, id, StatusApproved, "")
		return outcomeWait
	} else if ref != nil {
		s.mark(ctx, id, StatusSkipped, ref.Message)
		return outcomeSkipped
	}
	if _, ref, _ := s.guardTextNow(text, st, chargeID != ""); ref != nil {
		s.mark(ctx, id, StatusSkipped, ref.Message)
		s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
		return outcomeSkipped
	}
	// Cobrança: confere de novo, AGORA, no gateway. Quem pagou entre a aprovação e o envio nunca recebe a cobrança.
	var cv *ChargeVars
	if chargeID != "" {
		ch, rerr := s.refreshCharge(ctx, chargeID)
		if rerr != nil {
			if attempts >= 6 {
				s.mark(ctx, id, StatusFailed, "Não foi possível conferir a cobrança no gateway. Tente de novo mais tarde.")
				s.record(ctx, "automation.failed", id, map[string]any{"contactId": contactID, "why": "gateway"})
				return outcomeFailed
			}
			// Gateway fora do ar: volta para a fila e tenta de novo em 10 minutos, sem travar os demais.
			_, _ = s.db.Exec(ctx, `UPDATE automation_sends SET status = 'approved', scheduled_for = $2, updated_at = now() WHERE id = $1`, id, s.now().Add(10*time.Minute))
			return outcomeSkipped
		}
		if _, ref, gerr := s.guardCharge(ctx, ch.ID, contactID, st); gerr != nil {
			s.mark(ctx, id, StatusApproved, "")
			return outcomeWait
		} else if ref != nil {
			s.mark(ctx, id, StatusSkipped, ref.Message)
			s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
			return outcomeSkipped
		}
		if snapAmount != nil && snapDue != nil && (ch.AmountCents != *snapAmount || !ch.DueDate.Equal(*snapDue)) {
			s.mark(ctx, id, StatusSkipped, "O valor ou o vencimento da cobrança mudou depois da aprovação. Peça um novo envio para o administrador aprovar com os dados atuais.")
			s.record(ctx, "automation.skipped", id, map[string]any{"code": "charge_changed"})
			return outcomeSkipped
		}
		cv = ch.vars()
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
	// A leitura/validação do PDF pode levar tempo: revalida pausa, autonomia e permissões antes de sair.
	if st, result, ready = s.dispatchPolicy(ctx, id, category, text, tokenID, manual, chargeID != ""); !ready {
		return result
	}
	msgID := uuid.NewString()
	msg, err := s.convs.SendAutomated(ctx, conversations.AutomatedSend{
		ID: msgID, ContactID: contactID, Author: Author(st.Signature), Text: RenderCharge(text, info.Name, st.OfficeFields, cv, false),
		Att: att, Provider: provider, App: app, SendID: id, ActorID: creator,
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
	s.record(ctx, "automation.sent", id, map[string]any{"contactId": contactID, "category": category, "provider": provider, "hasDocument": docID != "", "chargeId": chargeID})
	return outcomeSent
}

// dispatchPolicy também vale para pedidos autônomos persistidos antes de um reinício.
func (s *Service) dispatchPolicy(ctx context.Context, id, category, text, tokenID string, manual, hasCharge bool) (Settings, outcome, bool) {
	wait := func(st Settings) (Settings, outcome, bool) {
		s.mark(ctx, id, StatusApproved, "")
		return st, outcomeWait, false
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return wait(st)
	}
	on, err := s.masterOn(ctx)
	if err != nil || !on || st.Paused {
		return wait(st)
	}
	cat, err := s.category(ctx, category)
	if err != nil || !cat.Enabled || !s.inWindow(cat, st, s.now()) {
		return wait(st)
	}
	flags, ref := ValidateTextCharge(text, st.AllowedDomains, st.OfficeFields, hasCharge)
	if ref != nil {
		s.mark(ctx, id, StatusSkipped, ref.Message)
		s.record(ctx, "automation.skipped", id, map[string]any{"code": ref.Code})
		return st, outcomeSkipped, false
	}
	if !manual && (cat.Level != LevelAutomatic || len(flags) > 0) {
		_, _ = s.db.Exec(ctx, `UPDATE automation_sends SET status='pending_approval', scheduled_for=NULL, flags=$2,
			status_detail='Revisão necessária pelas regras atuais do assunto.', updated_at=now() WHERE id=$1 AND status='sending'`, id, nonNil(flags))
		s.record(ctx, "automation.review_required", id, map[string]any{"level": cat.Level, "flags": flags})
		return st, outcomeSkipped, false
	}
	if tokenID != "" || !manual {
		var valid bool
		err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM api_tokens t JOIN users u ON u.id=t.user_id
			WHERE t.id::text=$1 AND u.is_active AND 'automate'=ANY(t.scopes) AND (
			(t.kind='personal' AND t.revoked_at IS NULL AND t.expires_at>now()) OR
			(t.kind='oauth' AND EXISTS (SELECT 1 FROM ai_settings WHERE id AND allow_oauth)
			 AND EXISTS (SELECT 1 FROM oauth_grants g WHERE g.id=t.grant_id AND g.revoked_at IS NULL AND 'automate'=ANY(g.scopes))
			 AND EXISTS (SELECT 1 FROM oauth_refresh_tokens r WHERE r.grant_id=t.grant_id AND r.used_at IS NULL AND r.expires_at>now()))
			))`, tokenID).Scan(&valid)
		if err != nil {
			return wait(st)
		}
		if !valid {
			s.mark(ctx, id, StatusSkipped, "O token de automação foi revogado, expirou ou perdeu permissão.")
			return st, outcomeSkipped, false
		}
	}
	return st, outcomeSent, true
}

// guardTextNow: revalida o texto na hora de enviar (as regras do administrador podem ter mudado desde a aprovação).
func (s *Service) guardTextNow(text string, st Settings, hasCharge bool) ([]string, *Refusal, error) {
	flags, ref := ValidateTextCharge(text, st.AllowedDomains, st.OfficeFields, hasCharge)
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
