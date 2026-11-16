package conversations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/database"
)

// Estado individual: cada pessoa tem o seu próprio "lido até" e pode adiar a conversa só para si.
// Sem linha em conversation_user_state a pessoa enxerga o estado geral (conversations.unread_count / snoozed_until).

const (
	SnoozeAll = "all" // adia para toda a equipe (a conversa sai da lista de todos)
	SnoozeMe  = "me"  // adia só para quem pediu
)

// userState é o que uma pessoa tem de próprio numa conversa.
type userState struct {
	unread  int
	snoozed *time.Time
}

// overlay aplica o estado individual sobre a conversa. O adiamento geral vale para todos e tem prioridade.
func (c *Conversation) overlay(st userState) {
	c.UnreadCount = st.unread
	if c.SnoozedUntil == nil && st.snoozed != nil {
		c.SnoozedUntil = st.snoozed
		c.SnoozedFor = SnoozeMe
	}
}

// userStates lê o estado individual de várias conversas para uma pessoa (ou, com userID vazio, de todas as pessoas).
func (s *Service) userStates(ctx context.Context, db database.DBTX, userID string, convIDs []string) (map[string]map[string]userState, error) {
	out := map[string]map[string]userState{} // conversa -> pessoa -> estado
	if len(convIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `
		SELECT us.conversation_id::text, us.user_id::text, us.snoozed_until,
		       (SELECT count(*) FROM messages m
		         WHERE m.conversation_id = us.conversation_id AND m.direction = 'in'
		           AND m.deleted_at IS NULL AND m.sent_at > us.read_through)
		  FROM conversation_user_state us
		 WHERE us.conversation_id = ANY($1::uuid[]) AND ($2 = '' OR us.user_id = NULLIF($2, '')::uuid)`, convIDs, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var conv, user string
		var st userState
		if err := rows.Scan(&conv, &user, &st.snoozed, &st.unread); err != nil {
			return nil, err
		}
		if out[conv] == nil {
			out[conv] = map[string]userState{}
		}
		out[conv][user] = st
	}
	return out, rows.Err()
}

// personalize põe o estado individual de uma pessoa nas conversas da lista.
func (s *Service) personalize(ctx context.Context, userID string, list []Conversation) error {
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].ID
	}
	states, err := s.userStates(ctx, s.db, userID, ids)
	if err != nil {
		return err
	}
	for i := range list {
		if st, ok := states[list[i].ID][userID]; ok {
			list[i].overlay(st)
		}
	}
	return nil
}

// publishUser manda a conversa, com o estado individual, só para essa pessoa.
func (s *Service) publishUser(ctx context.Context, convID, userID string) {
	if s.pub == nil {
		return
	}
	c, err := s.fetch(ctx, s.db, convID)
	if err != nil || c.Status == "closed" {
		return
	}
	one := []Conversation{c}
	if err := s.personalize(ctx, userID, one); err != nil {
		return
	}
	s.pub.Publish(EventConversationUpdated, one[0], userSet{userID: {}}.filter())
}

// publishViewers manda a conversa a quem a vê, cada pessoa com o seu estado: quem não tem estado próprio
// recebe uma mesma cópia; quem tem recebe a sua.
func (s *Service) publishViewers(ctx context.Context, c Conversation, to userSet) {
	states, err := s.userStates(ctx, s.db, "", []string{c.ID})
	if err != nil {
		s.pub.Publish(EventConversationUpdated, c, to.filter())
		return
	}
	plain := userSet{}
	for id := range to {
		st, ok := states[c.ID][id]
		if !ok {
			plain[id] = struct{}{}
			continue
		}
		mine := c
		mine.overlay(st)
		s.pub.Publish(EventConversationUpdated, mine, userSet{id: {}}.filter())
	}
	if len(plain) > 0 {
		s.pub.Publish(EventConversationUpdated, c, plain.filter())
	}
}

// readThrough: "agora", ou a última mensagem do cliente se o relógio do provedor estiver à frente do servidor.
const readThrough = `greatest(now(), coalesce((SELECT max(m.sent_at) FROM messages m WHERE m.conversation_id = $1 AND m.direction = 'in'), now()))`

// markReadFor registra que esta pessoa leu a conversa até agora. Só cria o estado individual quando há algo
// não lido para ela. Devolve se algo mudou.
func (s *Service) markReadFor(ctx context.Context, convID, userID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO conversation_user_state (conversation_id, user_id, read_through)
		SELECT c.id, $2, `+readThrough+` FROM conversations c
		 WHERE c.id = $1 AND (c.unread_count > 0 OR EXISTS (
		       SELECT 1 FROM conversation_user_state x WHERE x.conversation_id = c.id AND x.user_id = $2))
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET read_through = `+readThrough+`
		 WHERE EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = $1 AND m.direction = 'in'
		                  AND m.deleted_at IS NULL AND m.sent_at > conversation_user_state.read_through)`, convID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// clearPersonal tira os adiamentos individuais (conversa respondida pelo cliente ou encerrada).
func clearPersonal(ctx context.Context, tx pgx.Tx, convID string) error {
	_, err := tx.Exec(ctx, `UPDATE conversation_user_state SET snoozed_until = NULL WHERE conversation_id = $1 AND snoozed_until IS NOT NULL`, convID)
	return err
}
