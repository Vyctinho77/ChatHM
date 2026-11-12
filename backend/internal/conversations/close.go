package conversations

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// ErrNotInvolved: quem não está no atendimento (nem como responsável nem como participante) não tem "parte" para encerrar.
var ErrNotInvolved = errors.New("conversations: pessoa fora do atendimento")

// Escopos do encerramento.
const (
	CloseAll  = "all" // encerra o atendimento para todas as pessoas (padrão)
	CloseMine = "me"  // encerra só a parte de quem pede
)

// Avisos em tempo real para quem está no atendimento (ver EventConversationNotice).
const (
	NoticeClosedForAll = "closed_for_all" // outra pessoa encerrou o atendimento para todos
	NoticeHandedOver   = "handed_over"    // o responsável saiu e passou o atendimento para você
)

// EventConversationNotice: aviso, só para as pessoas envolvidas, de que o atendimento mudou por ação de outra pessoa.
const EventConversationNotice = "conversation.notice"

type CloseOptions struct {
	Scope string // CloseAll (padrão) ou CloseMine

	// Só para CloseMine do responsável quando outras pessoas participam: quem continua (padrão: a participante mais antiga)
	// e se o lembrete pessoal vai junto.
	PassTo        string
	ShareReminder bool
}

// Close encerra o atendimento, para todos ou só a parte de quem pede.
//
//   - CloseAll: o responsável, um gestor (nas conversas que enxerga) ou um administrador encerram para todos. Quem encerra a
//     conversa em que outras pessoas estão é registrado na auditoria com quem foi afetado, e essas pessoas recebem um aviso.
//   - CloseMine: o responsável ou uma participante saem do atendimento, que continua para quem ficou. Se a pessoa é o
//     responsável e há participantes, o atendimento passa a uma delas (como numa transferência, inclusive o lembrete); se é a
//     única pessoa, o atendimento se encerra. Quem não está no atendimento recebe ErrNotInvolved: só pode encerrar para todos.
//     A participante que sai nunca encerra a conversa: ela volta a valer só pelas regras normais de visibilidade.
func (s *Service) Close(ctx context.Context, v Viewer, convID string, opts CloseOptions) error {
	if !httpx.ValidUUID(convID) {
		return ErrNotFound
	}
	scope := opts.Scope
	if scope == "" {
		scope = CloseAll
	}
	if scope != CloseAll && scope != CloseMine {
		return httpx.FieldErrors{"scope": "Escolha encerrar só a sua parte ou para todos."}
	}
	if opts.PassTo != "" && !httpx.ValidUUID(opts.PassTo) {
		return httpx.FieldErrors{"passTo": "Escolha quem continua o atendimento."}
	}

	var (
		before  userSet
		notices []notice
		changed bool
	)
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			status        string
			assignee      *string
			segID, teamID *string
			ct            Contact
		)
		err := tx.QueryRow(ctx, `
			SELECT c.status, c.assignee_id::text, c.segment_id::text, s.team_id::text,
			       ct.name, ct.phone, (ct.name_edited OR ct.whatsapp_saved), ct.kind
			  FROM conversations c
			  JOIN contacts ct ON ct.id = c.contact_id
			  LEFT JOIN segments s ON s.id = c.segment_id
			 WHERE c.id = $3 AND `+forViewer+` FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&status, &assignee, &segID, &teamID, &ct.Name, &ct.Phone, &ct.Saved, &ct.Kind)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return nil
		}
		isAssignee := assignee != nil && *assignee == v.ID
		contact := contactTitle(ct)

		// Participantes, da que entrou primeiro à última; só as ativas podem continuar o atendimento.
		rows, err := tx.Query(ctx, `
			SELECT cp.user_id::text, u.is_active FROM conversation_participants cp JOIN users u ON u.id = cp.user_id
			 WHERE cp.conversation_id = $1 ORDER BY cp.created_at, cp.user_id`, convID)
		if err != nil {
			return err
		}
		var participants, active []string
		for rows.Next() {
			var id string
			var ok bool
			if err := rows.Scan(&id, &ok); err != nil {
				rows.Close()
				return err
			}
			participants = append(participants, id)
			if ok {
				active = append(active, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		isParticipant := slices.Contains(participants, v.ID)
		// Todas as pessoas no atendimento além de quem pede.
		others := slices.DeleteFunc(slices.Clone(participants), func(id string) bool { return id == v.ID })
		if assignee != nil && !isAssignee {
			others = append([]string{*assignee}, others...)
		}

		if scope == CloseAll {
			if !v.CanSupervise() && !isAssignee {
				return ErrForbidden
			}
			changed = true
			for _, id := range others {
				notices = append(notices, notice{to: id, kind: NoticeClosedForAll, contact: contact})
			}
			return s.closeTx(ctx, tx, v, convID, map[string]any{"scope": CloseAll, "affected": others})
		}

		// CloseMine
		if !isAssignee && !isParticipant {
			return ErrNotInvolved
		}
		if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
			return err
		}
		if !isAssignee { // participante: só sai
			if _, err := tx.Exec(ctx, `DELETE FROM conversation_participants WHERE conversation_id = $1 AND user_id = $2`, convID, v.ID); err != nil {
				return err
			}
			changed = true
			return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationLeave, TargetType: "conversation", TargetID: convID,
				Metadata: map[string]any{"role": "participant"}})
		}
		if len(active) == 0 { // a única pessoa no atendimento: encerra o atendimento
			changed = true
			return s.closeTx(ctx, tx, v, convID, map[string]any{"scope": CloseMine, "last": true})
		}
		next := opts.PassTo
		if next == "" {
			next = active[0]
		}
		if !slices.Contains(active, next) {
			return httpx.FieldErrors{"passTo": "Escolha uma das pessoas que continuam no atendimento."}
		}
		var note, noteColor *string
		if opts.ShareReminder {
			if note, noteColor, err = readReminder(ctx, tx, v.ID, convID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM conversation_participants WHERE conversation_id = $1 AND user_id = $2`, convID, next); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE conversation_assignments SET ended_at = now(), ended_reason = 'transferred' WHERE conversation_id = $1 AND ended_at IS NULL`, convID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET assignee_id = $2, status = 'in_progress' WHERE id = $1`, convID, next); err != nil {
			return err
		}
		var handoverBy *string
		if note != nil {
			handoverBy = &v.ID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_assignments (conversation_id, user_id, team_id, segment_id, handover_note, handover_color, handover_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, convID, next, teamID, segID, note, noteColor, handoverBy); err != nil {
			return err
		}
		if err := copyReminder(ctx, tx, next, convID, note, noteColor); err != nil {
			return err
		}
		changed = true
		notices = append(notices, notice{to: next, kind: NoticeHandedOver, contact: contact})
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationLeave, TargetType: "conversation", TargetID: convID,
			Metadata: map[string]any{"role": "assignee", "handoverTo": next, "reminderShared": note != nil}})
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	s.publishState(ctx, convID, before) // quem deixou de ver a conversa a perde da lista; quem ficou a recebe atualizada
	for _, n := range notices {
		s.publishNotice(convID, v, n)
	}
	return nil
}

type notice struct{ to, kind, contact string }

func (s *Service) publishNotice(convID string, by Viewer, n notice) {
	if s.pub == nil {
		return
	}
	s.pub.Publish(EventConversationNotice, map[string]any{"id": convID, "kind": n.kind, "by": by.Name, "contact": n.contact}, userSet{n.to: {}}.filter())
}

// closeTx encerra o atendimento para todos dentro da transação.
func (s *Service) closeTx(ctx context.Context, tx pgx.Tx, v Viewer, convID string, meta map[string]any) error {
	if _, err := tx.Exec(ctx, `UPDATE conversations SET status = 'closed', closed_at = now(), snoozed_until = NULL, snoozed_by = NULL WHERE id = $1`, convID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE conversation_assignments SET ended_at = now(), ended_reason = 'closed' WHERE conversation_id = $1 AND ended_at IS NULL`, convID); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationClose, TargetType: "conversation", TargetID: convID, Metadata: meta})
}

// contactTitle: como o cliente aparece nos avisos, igual ao cabeçalho da conversa: grupos e contatos salvos pelo nome, os demais
// pelo telefone formatado (o nome de perfil do WhatsApp é escolhido pelo próprio cliente).
func contactTitle(c Contact) string {
	if c.Kind == "group" || c.Saved || c.Phone == "" {
		return c.Name
	}
	return FormatPhone(c.Phone)
}
