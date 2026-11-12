package aiaccess

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

// Respostas pendentes: respostas de clientes a mensagens que uma IA enviou, ainda não tratadas por este agente.
// O agente lê, avisa a pessoa e marca o que já foi resolvido, SEM responder ao cliente. O que ele já tratou fica guardado por
// agente (pessoa dona do token + provedor): trocar de token não perde o histórico, e agentes diferentes não se atrapalham.

const (
	replyWindow    = 30 * 24 * time.Hour // só conversas em que uma IA falou nos últimos 30 dias
	maxMarkItems   = 50
	maxReplyNote   = 300
	pendingDefault = 20
	waitDefault    = 25 * time.Second
	waitMax        = 30 * time.Second
	waitPoll       = 3 * time.Second
	pendingMax     = 50
)

// agentKey identifica o agente de forma estável (não muda quando o token é renovado).
func agentKey(p Principal) string {
	prov := p.Provider
	if prov == "" {
		prov = "other"
	}
	return p.UserID + ":" + prov
}

type pendingRow struct {
	ConversationID  string
	Count           int
	FirstAt, LastAt time.Time
}

// pendingReplies devolve, entre as conversas visíveis, as que têm resposta de cliente depois da mensagem de uma IA e depois do que o agente já tratou.
func (s *Service) pendingReplies(ctx context.Context, key string, convIDs []string, since time.Time) ([]pendingRow, error) {
	if len(convIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT m.conversation_id::text, count(*), min(m.sent_at), max(m.sent_at)
		  FROM messages m
		  JOIN (SELECT conversation_id, min(sent_at) AS first_ai FROM messages
		         WHERE direction = 'out' AND payload->>'automated' = 'true' AND sent_at > now() - $4::interval
		           AND conversation_id = ANY($1::uuid[]) GROUP BY conversation_id) ai ON ai.conversation_id = m.conversation_id
		  LEFT JOIN ai_reply_tracking t ON t.agent_key = $2 AND t.conversation_id = m.conversation_id
		 WHERE m.direction = 'in' AND coalesce(m.payload->>'historical', '') <> 'true'
		   AND m.sent_at > greatest(ai.first_ai, coalesce(t.seen_until, '-infinity'::timestamptz), $3)
		 GROUP BY m.conversation_id
		 ORDER BY max(m.sent_at) DESC`, convIDs, key, since, replyWindow.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.ConversationID, &r.Count, &r.FirstAt, &r.LastAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type handledRow struct {
	ConversationID, Status, Note string
	UpdatedAt                    time.Time
}

func (s *Service) handledReplies(ctx context.Context, key string, convIDs []string) ([]handledRow, error) {
	if len(convIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT t.conversation_id::text, t.status, t.note, t.updated_at FROM ai_reply_tracking t
		 WHERE t.agent_key = $1 AND t.conversation_id = ANY($2::uuid[])
		   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = t.conversation_id AND m.direction = 'in'
		                      AND coalesce(m.payload->>'historical', '') <> 'true' AND m.sent_at > t.seen_until)
		 ORDER BY t.updated_at DESC LIMIT 100`, key, convIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []handledRow
	for rows.Next() {
		var r handledRow
		if err := rows.Scan(&r.ConversationID, &r.Status, &r.Note, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// markReplies grava que o agente já tratou as respostas do cliente nesta conversa, até a mais recente de agora.
func (s *Service) markReplies(ctx context.Context, key, convID, status, note string) error {
	var seen time.Time
	if err := s.db.QueryRow(ctx, `SELECT coalesce(max(sent_at), now()) FROM messages WHERE conversation_id = $1 AND direction = 'in'`, convID).Scan(&seen); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO ai_reply_tracking (agent_key, conversation_id, status, note, seen_until) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (agent_key, conversation_id) DO UPDATE SET status = $3, note = $4, seen_until = $5, updated_at = now()`, key, convID, status, note, seen)
	return err
}

func (m *MCP) registerReplies(s *mcp.Server, p Principal, v conversations.Viewer) {
	type listIn struct {
		IncludeHandled bool   `json:"include_handled,omitempty" jsonschema:"também devolve, em handled, as conversas que você já marcou como avisadas ou resolvidas e em que o cliente não escreveu de novo"`
		Since          string `json:"since,omitempty" jsonschema:"só respostas depois deste instante (ISO 8601), para consultar de tempos em tempos"`
		Limit          int    `json:"limit,omitempty" jsonschema:"máximo de conversas (padrão 20, até 50)"`
	}
	listFn := func(ctx context.Context, in listIn) (any, error) {
		var since time.Time
		if in.Since != "" {
			t, err := time.Parse(time.RFC3339, in.Since)
			if err != nil {
				return nil, errors.New("since deve estar em ISO 8601, ex.: 2026-11-07T12:00:00Z")
			}
			since = t
		}
		all, err := m.convs.List(ctx, v)
		if err != nil {
			return nil, err
		}
		byID := map[string]conversations.Conversation{}
		ids := make([]string, 0, len(all))
		for _, c := range all {
			if c.Status == "closed" || c.Contact.Kind == "group" {
				continue
			}
			byID[c.ID] = c
			ids = append(ids, c.ID)
		}
		key := agentKey(p)
		rows, err := m.svc.pendingReplies(ctx, key, ids, since)
		if err != nil {
			return nil, err
		}
		limit := clampLimit(in.Limit, pendingDefault, pendingMax)
		items := []any{}
		for i, r := range rows {
			if i >= limit {
				break
			}
			c := byID[r.ConversationID]
			item := map[string]any{
				"conversationId": c.ID,
				"contact":        contactView(c.Contact, nil),
				"queue":          map[string]any{"key": c.Segment, "name": c.SegmentName},
				"status":         statusPT[c.Status],
				"pending":        map[string]any{"count": r.Count, "firstAt": r.FirstAt, "lastAt": r.LastAt},
			}
			if c.Assignee != nil {
				item["assignee"] = c.Assignee.Name
			}
			if msgs, merr := m.convs.Messages(ctx, v, c.ID, 200); merr == nil {
				var lastIn conversations.Message
				for _, mm := range msgs {
					if str(mm["direction"]) == "in" {
						lastIn = mm
					}
				}
				if lastIn != nil {
					item["lastReply"] = msgView(lastIn, p, c.Contact.ID)
					lastAt, _ := time.Parse(time.RFC3339Nano, str(lastIn["sentAt"]))
					for _, mm := range msgs {
						at, _ := time.Parse(time.RFC3339Nano, str(mm["sentAt"]))
						if str(mm["direction"]) == "out" && at.After(lastAt) {
							item["answeredAfter"] = map[string]any{"at": str(mm["sentAt"]), "from": authorOf(mm, p, c.Contact.ID)}
							break
						}
					}
				}
			}
			items = append(items, item)
		}
		out := map[string]any{"total": len(rows), "returned": len(items), "conversations": items,
			"howToUse": "Avise a pessoa e depois chame mark_replies (status notified ou resolved). Quando o cliente escrever de novo, a conversa volta para esta lista."}
		if in.IncludeHandled {
			hs, err := m.svc.handledReplies(ctx, key, ids)
			if err != nil {
				return nil, err
			}
			handled := make([]any, 0, len(hs))
			for _, h := range hs {
				c := byID[h.ConversationID]
				handled = append(handled, map[string]any{"conversationId": h.ConversationID, "contact": contactView(c.Contact, nil), "status": h.Status, "note": h.Note, "updatedAt": h.UpdatedAt})
			}
			out["handled"] = handled
		}
		return out, nil
	}
	addTool(m, s, p, "list_pending_replies",
		"Respostas de clientes a mensagens enviadas por IA que você ainda não tratou, por conversa, da mais recente para a mais antiga: quem respondeu, em qual fila, quantas respostas, a última resposta (com quem a escreveu em from) e se alguém já respondeu depois (answeredAfter). "+
			"Fluxo: leia, avise a pessoa e use mark_replies para marcar o que já foi avisado ou resolvido. Esta ferramenta NÃO responde ao cliente.",
		listFn)

	type waitIn struct {
		IncludeHandled bool   `json:"include_handled,omitempty" jsonschema:"também devolve, em handled, as conversas já marcadas como avisadas ou resolvidas"`
		Since          string `json:"since,omitempty" jsonschema:"só respostas depois deste instante (ISO 8601)"`
		Limit          int    `json:"limit,omitempty" jsonschema:"máximo de conversas (padrão 20, até 50)"`
		TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"quanto esperar por uma resposta nova (padrão 25, de 1 a 30 segundos)"`
	}
	addTool(m, s, p, "wait_for_replies",
		"Fica esperando (até 30 s) e responde assim que houver resposta de cliente ainda não tratada; se nada chegar, volta com timedOut=true e a lista vazia. "+
			"Serve a quem não tem endereço público para o webhook (app no computador): chame de novo em seguida, em laço. Devolve o mesmo que list_pending_replies. Use mark_replies para dar baixa no que já tratou, senão a conversa volta a cada chamada. NÃO responde ao cliente.",
		func(ctx context.Context, in waitIn) (any, error) {
			wait := time.Duration(in.TimeoutSeconds) * time.Second
			if wait <= 0 {
				wait = waitDefault
			}
			if wait > waitMax {
				wait = waitMax
			}
			deadline := time.Now().Add(wait)
			tick := time.NewTicker(waitPoll)
			defer tick.Stop()
			for {
				res, err := listFn(ctx, listIn{IncludeHandled: in.IncludeHandled, Since: in.Since, Limit: in.Limit})
				if err != nil {
					return nil, err
				}
				if out, ok := res.(map[string]any); ok {
					if n, _ := out["total"].(int); n > 0 {
						out["timedOut"] = false
						return out, nil
					}
					if !time.Now().Before(deadline) {
						out["timedOut"] = true
						return out, nil
					}
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-tick.C:
				}
			}
		})

	type markItem struct {
		ConversationID string `json:"conversation_id" jsonschema:"id da conversa (list_pending_replies)"`
		Status         string `json:"status" jsonschema:"notified (você avisou a pessoa) ou resolved (já está resolvido)"`
		Note           string `json:"note,omitempty" jsonschema:"anotação sua, até 300 caracteres (ex.: o que foi resolvido)"`
	}
	type markIn struct {
		Items []markItem `json:"items" jsonschema:"de 1 a 50 conversas"`
	}
	addWrite(m, s, p, "mark_replies",
		"Marca conversas de list_pending_replies como avisadas (notified) ou resolvidas (resolved) por você. É só o SEU controle do que já foi tratado: não responde ao cliente, não muda a conversa e não precisa de escopo de escrita. Se o cliente escrever de novo, a conversa volta para a lista.",
		true, func(ctx context.Context, in markIn) (any, map[string]any, error) {
			if len(in.Items) == 0 || len(in.Items) > maxMarkItems {
				return nil, nil, errors.New("items deve ter de 1 a 50 conversas")
			}
			key := agentKey(p)
			results := make([]any, 0, len(in.Items))
			okN := 0
			for _, it := range in.Items {
				res := map[string]any{"conversationId": it.ConversationID}
				note := strings.TrimSpace(it.Note)
				switch {
				case it.Status != "notified" && it.Status != "resolved":
					res["ok"], res["error"] = false, "status deve ser notified ou resolved"
				case utf8.RuneCountInString(note) > maxReplyNote || strings.ContainsAny(note, "\x00"):
					res["ok"], res["error"] = false, "a anotação pode ter até 300 caracteres"
				default:
					if _, perr := uuid.Parse(it.ConversationID); perr != nil {
						res["ok"], res["error"] = false, "id de conversa inválido"
					} else if c, gerr := m.convs.Get(ctx, v, it.ConversationID); gerr != nil || c.Status == "closed" {
						res["ok"], res["error"] = false, "conversa não encontrada (ou você não tem acesso a ela)"
					} else if merr := m.svc.markReplies(ctx, key, it.ConversationID, it.Status, note); merr != nil && !errors.Is(merr, pgx.ErrNoRows) {
						return nil, nil, errors.New("não foi possível registrar agora; tente de novo")
					} else {
						res["ok"] = true
						okN++
					}
				}
				results = append(results, res)
			}
			return map[string]any{"ok": okN == len(in.Items), "marked": okN, "results": results}, map[string]any{"count": len(in.Items), "marked": okN}, nil
		})
}
