package aiaccess

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

// laneWake: o que acorda uma IA presa a uma raia. Parte das respostas de clientes (as mesmas de list_pending_replies) e:
//   - só deixa o que é da raia (a fila dela) e ainda não tem pessoa: conversa com responsável ou passada para um humano não acorda a IA;
//   - acrescenta as conversas PARADAS (reason "parada"): o cliente escreveu e ninguém da equipe iniciou o atendimento há staleHours.
//     O instante do aviso é o em que a conversa ficou parada (última mensagem do cliente + staleHours), então o cursor "since" de quem
//     escuta (scripts/chathm-listen.mjs) funciona sem mudança: o que já foi avisado não volta;
//   - raia desligada não acorda ninguém.
func (m *MCP) laneWake(ctx context.Context, p Principal, key string, byID map[string]conversations.Conversation, rows []pendingRow, since time.Time) ([]pendingRow, error) {
	lane, err := m.auto.Lane(ctx, p.Lane)
	if err != nil || !lane.Enabled {
		return nil, nil
	}
	segKey := ""
	if lane.SegmentID != "" {
		if segKey, err = m.svc.segmentKey(ctx, lane.SegmentID); err != nil || segKey == "" {
			return nil, nil
		}
	}
	inLane := func(c conversations.Conversation) bool {
		return c.Assignee == nil && (segKey == "" || c.Segment == segKey)
	}
	candidates := make([]string, 0, len(rows))
	for _, r := range rows {
		if c, ok := byID[r.ConversationID]; ok && inLane(c) {
			candidates = append(candidates, r.ConversationID)
		}
	}
	var stale []pendingStale
	if lane.StaleHours > 0 {
		us, uerr := m.auto.UnattendedList(ctx, lane.SegmentID, lane.StaleHours, 60)
		if uerr != nil {
			return nil, uerr
		}
		for _, u := range us {
			if c, ok := byID[u.ConversationID]; ok && inLane(c) {
				stale = append(stale, pendingStale{id: u.ConversationID, waiting: u.WaitingSince})
			}
		}
	}
	all := append([]string{}, candidates...)
	for _, st := range stale {
		all = append(all, st.id)
	}
	handoff, err := m.auto.OpenHandoffSet(ctx, candidates)
	if err != nil {
		return nil, err
	}
	tracked, err := m.svc.trackedUntil(ctx, key, all)
	if err != nil {
		return nil, err
	}
	out := make([]pendingRow, 0, len(rows)+len(stale))
	have := map[string]bool{}
	for _, r := range rows {
		c, ok := byID[r.ConversationID]
		if !ok || !inLane(c) || handoff[r.ConversationID] {
			continue
		}
		r.Reason = "resposta"
		out = append(out, r)
		have[r.ConversationID] = true
	}
	for _, st := range stale {
		if have[st.id] {
			continue
		}
		at := st.waiting.Add(time.Duration(lane.StaleHours) * time.Hour) // quando ficou parada
		if !at.After(since) {
			continue
		}
		if t, ok := tracked[st.id]; ok && !t.Before(st.waiting) {
			continue // este agente já tratou (mark_replies) a última mensagem do cliente
		}
		out = append(out, pendingRow{ConversationID: st.id, Count: 1, FirstAt: st.waiting, LastAt: at, Reason: "parada"})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastAt.After(out[j].LastAt) })
	return out, nil
}

type pendingStale struct {
	id      string
	waiting time.Time
}

// segmentKey: a chave da fila (as conversas trazem a chave, não o id).
func (s *Service) segmentKey(ctx context.Context, id string) (string, error) {
	var k string
	err := s.db.QueryRow(ctx, `SELECT key FROM segments WHERE id = $1`, id).Scan(&k)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return k, err
}

// trackedUntil: até quando este agente já tratou cada conversa (mark_replies).
func (s *Service) trackedUntil(ctx context.Context, key string, convIDs []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if len(convIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `SELECT conversation_id::text, seen_until FROM ai_reply_tracking WHERE agent_key = $1 AND conversation_id = ANY($2::uuid[])`, key, convIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var t time.Time
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}
