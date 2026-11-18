package automation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// HandoffTag é a etiqueta que a equipe vê na conversa quando a IA passou para um humano.
const HandoffTag = "Precisa de humano"

var reReason = regexp.MustCompile(`^[a-z][a-z_]{0,39}$`)

// HandoffResult: a passagem registrada. Existing=true quando a conversa já estava passada (a chamada é repetida sem efeito).
type HandoffResult struct {
	ID       string `json:"id"`
	Existing bool   `json:"existing"`
}

// RequestHuman passa a conversa para um humano: registra a passagem, etiqueta a conversa, deixa uma linha na observação do contato com o
// motivo e o resumo, leva a conversa à fila de destino da raia (se houver) e desfaz o adiamento. A IA fica em silêncio nela até uma pessoa
// responder. `source` é "ai" (a IA pediu) ou "server" (uma regra da raia exigiu).
func (s *Service) RequestHuman(ctx context.Context, a Actor, convID, reason, summary, source string) (HandoffResult, error) {
	if !httpx.ValidUUID(convID) {
		return HandoffResult{}, ErrNoConversation
	}
	reason = strings.ToLower(strings.TrimSpace(reason))
	if !reReason.MatchString(reason) {
		reason = "outro"
	}
	summary = strings.TrimSpace(strings.Join(strings.Fields(summary), " "))
	if utf8.RuneCountInString(summary) > 500 {
		summary = string([]rune(summary)[:500])
	}
	if source != "ai" && source != "server" {
		source = "ai"
	}
	var out HandoffResult
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var status, contactID string
		if err := tx.QueryRow(ctx, `SELECT status, contact_id::text FROM conversations WHERE id = $1 FOR UPDATE`, convID).Scan(&status, &contactID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoConversation
			}
			return err
		}
		if status == "closed" {
			return refuse("encerrada", "A conversa está encerrada.")
		}
		var open string
		err := tx.QueryRow(ctx, `
			SELECT h.id::text FROM ai_handoffs h WHERE h.conversation_id = $1
			   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = h.conversation_id AND `+humanOut+` AND m.sent_at > h.created_at)
			 ORDER BY h.created_at DESC LIMIT 1`, convID).Scan(&open)
		if err == nil {
			out = HandoffResult{ID: open, Existing: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var handoffSeg string
		if a.Lane != "" {
			_ = tx.QueryRow(ctx, `SELECT coalesce(handoff_segment_id::text, '') FROM ai_lanes WHERE key = $1`, a.Lane).Scan(&handoffSeg)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO ai_handoffs (conversation_id, lane_key, token_id, reason, summary, source)
			VALUES ($1, NULLIF($2, ''), NULLIF($3, '')::uuid, $4, $5, $6) RETURNING id::text`,
			convID, a.Lane, a.TokenID, reason, summary, source).Scan(&out.ID); err != nil {
			return err
		}
		if handoffSeg != "" {
			if _, err := tx.Exec(ctx, `UPDATE conversations SET segment_id = $2 WHERE id = $1`, convID, handoffSeg); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET snoozed_until = NULL, snoozed_by = NULL WHERE id = $1 AND snoozed_until IS NOT NULL`, convID); err != nil {
			return err
		}
		var tagID string
		if _, err := tx.Exec(ctx, `INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, HandoffTag); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM tags WHERE name = $1`, HandoffTag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO conversation_tags (conversation_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, convID, tagID); err != nil {
			return err
		}
		line := fmt.Sprintf("[IA pediu ajuda · %s · %s]", s.now().Format("02/01 15:04"), reason)
		if summary != "" {
			line += " " + summary
		}
		if _, err := tx.Exec(ctx, `
			UPDATE contacts SET note = right(CASE WHEN note = '' THEN $2 ELSE note || E'\n' || $2 END, 1900), updated_at = now() WHERE id = $1`, contactID, line); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "ai.handoff", TargetType: "conversation", TargetID: convID,
			Metadata: map[string]any{"reason": reason, "source": source, "lane": a.Lane, "tokenId": a.TokenID}})
	})
	if err != nil {
		return HandoffResult{}, err
	}
	if !out.Existing {
		s.convs.Refresh(context.WithoutCancel(ctx), convID)
	}
	return out, nil
}

// --- modo sombra -----------------------------------------------------------

// Draft: o que a IA enviaria (modo sombra).
type Draft struct {
	ID             string     `json:"id"`
	Lane           string     `json:"lane"`
	ConversationID string     `json:"conversationId"`
	ContactID      string     `json:"contactId"`
	ContactName    string     `json:"contactName"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"createdAt"`
	Outcome        string     `json:"outcome,omitempty"` // vazio = ainda para revisar; approved | edited | discarded
	ReviewedAt     *time.Time `json:"reviewedAt,omitempty"`
	Provider       string     `json:"provider"`
	Agent          string     `json:"agent"`
	FinalBody      string     `json:"finalBody,omitempty"`
}

func (s *Service) saveDraft(ctx context.Context, a Actor, lane, convID, contactID, body string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO ai_lane_drafts (lane_key, conversation_id, contact_id, token_id, body, provider, app, agent)
		VALUES ($1, NULLIF($2, '')::uuid, $3, NULLIF($4, '')::uuid, $5, $6, $7, $8) RETURNING id::text`, lane, convID, contactID, a.TokenID, body, a.Provider, a.App, a.Agent).Scan(&id)
	return id, err
}

// Drafts lista os rascunhos da raia (os mais novos primeiro). pendingOnly: só os que ainda esperam revisão.
func (s *Service) Drafts(ctx context.Context, lane string, pendingOnly bool, limit int) ([]Draft, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT d.id::text, d.lane_key, coalesce(d.conversation_id::text, ''), d.contact_id::text, c.name, d.body, d.created_at, coalesce(d.outcome, ''), d.reviewed_at,
		       d.provider, d.agent, coalesce(d.final_body, '')
		  FROM ai_lane_drafts d JOIN contacts c ON c.id = d.contact_id
		 WHERE ($1 = '' OR d.lane_key = $1) AND (NOT $2 OR d.outcome IS NULL) ORDER BY d.created_at DESC LIMIT $3`, lane, pendingOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Draft{}
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.Lane, &d.ConversationID, &d.ContactID, &d.ContactName, &d.Body, &d.CreatedAt, &d.Outcome, &d.ReviewedAt, &d.Provider, &d.Agent, &d.FinalBody); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ApproveDraft (administrador) envia o rascunho agora, como o agente que o escreveu, com o texto original ou o editado.
// Confere de novo a conversa (uma pessoa pode ter assumido, a conversa pode ter sido passada para a equipe ou encerrada) e os limites globais
// do atendente (pausa, PARAR, grupos, velocidade); não refaz os limites de rodadas nem as regras do texto: quem revisa decide.
func (s *Service) ApproveDraft(ctx context.Context, reviewerID, ip, draftID, editedText string) (AttendResult, error) {
	text := strings.TrimSpace(editedText)
	edited := text != ""
	var d struct {
		lane, convID, contactID, body, provider, app, agent, tokenID, owner string
	}
	// Reserva o rascunho (dois cliques ou duas pessoas não enviam duas vezes).
	err := s.db.QueryRow(ctx, `
		UPDATE ai_lane_drafts d SET outcome = CASE WHEN $2 THEN 'edited' ELSE 'approved' END, reviewed_by = $3::uuid, reviewed_at = now()
		 WHERE d.id = $1 AND d.outcome IS NULL
		RETURNING d.lane_key, coalesce(d.conversation_id::text, ''), d.contact_id::text, d.body, d.provider, d.app, d.agent, coalesce(d.token_id::text, ''),
		          coalesce((SELECT t.user_id::text FROM api_tokens t WHERE t.id = d.token_id), '')`, draftID, edited, reviewerID).
		Scan(&d.lane, &d.convID, &d.contactID, &d.body, &d.provider, &d.app, &d.agent, &d.tokenID, &d.owner)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if qerr := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_lane_drafts WHERE id = $1)`, draftID).Scan(&exists); qerr == nil && exists {
			return AttendResult{}, ErrState // já foi enviado ou descartado
		}
		return AttendResult{}, ErrNotFound
	}
	if err != nil {
		return AttendResult{}, err
	}
	release := func() { // não enviou: o rascunho volta para a revisão
		_, _ = s.db.Exec(context.WithoutCancel(ctx), `UPDATE ai_lane_drafts SET outcome = NULL, reviewed_by = NULL, reviewed_at = NULL WHERE id = $1`, draftID)
	}
	if !edited {
		text = d.body
	}
	owner := d.owner
	if owner == "" {
		owner = reviewerID
	}
	actor := Actor{UserID: owner, TokenID: d.tokenID, Provider: d.provider, App: d.app, Agent: d.agent, Lane: d.lane}
	res, err := s.SendAsAttendant(ctx, actor, AttendInput{ContactID: d.contactID, ConversationID: d.convID, Text: text, reviewed: true})
	if err != nil {
		release()
		return AttendResult{}, err
	}
	if res.Status != "enviado" {
		release()
		return res, refuse("whatsapp", "O WhatsApp não aceitou a mensagem. O rascunho continua aqui para você tentar de novo.")
	}
	_, _ = s.db.Exec(ctx, `UPDATE ai_lane_drafts SET final_body = $2, message_id = $3::uuid WHERE id = $1`, draftID, text, res.MessageID)
	s.record(ctx, "ai.lane.draft.approve", draftID, map[string]any{"lane": d.lane, "edited": edited, "messageId": res.MessageID, "reviewer": reviewerID, "ip": ip})
	return res, nil
}

// DiscardDraft (administrador) descarta o rascunho: nada é enviado.
func (s *Service) DiscardDraft(ctx context.Context, reviewerID, ip, draftID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE ai_lane_drafts SET outcome = 'discarded', reviewed_by = $2::uuid, reviewed_at = now() WHERE id = $1 AND outcome IS NULL`, draftID, reviewerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if qerr := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_lane_drafts WHERE id = $1)`, draftID).Scan(&exists); qerr == nil && exists {
			return ErrState
		}
		return ErrNotFound
	}
	s.record(ctx, "ai.lane.draft.discard", draftID, map[string]any{"reviewer": reviewerID, "ip": ip})
	return nil
}

// applyReviewed: o envio de um rascunho já revisado por uma pessoa. Só as regras da CONVERSA (aberta, sem passagem pendente, sem atendente humano, não adiada)
// e os caracteres invisíveis/credenciais do texto; limites de rodadas, links e fontes não valem (a pessoa decidiu).
func (s *Service) applyReviewed(ctx context.Context, a Actor, in AttendInput, text string) (string, error) {
	lane, err := s.Lane(ctx, a.Lane)
	if errors.Is(err, ErrNoLane) {
		return "", refuse("raia_desligada", "A raia deste rascunho não existe mais.")
	}
	if err != nil {
		return "", err
	}
	relaxed := Lane{Key: lane.Key, Label: lane.Label, Enabled: true, Mode: LaneLive, MaxTurns: 1000, MaxConsecutive: 1000, ContactDaily: 1000, HumanQuietMinutes: lane.HumanQuietMinutes}
	f, err := s.laneFactsFor(ctx, in)
	if errors.Is(err, errNoOpenConversation) {
		f, err = s.laneFactsForContact(ctx, in.ContactID)
	}
	if err != nil {
		return "", err
	}
	f.Inbound = nil // o que o cliente mandou já foi visto por quem revisou
	d := decideLane(relaxed, f, text)
	if d.Refusal != nil {
		return "", d.Refusal
	}
	return d.Text, nil
}

// Handoff: uma passagem para humano, para a tela do administrador.
type Handoff struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Lane           string    `json:"lane"`
	Reason         string    `json:"reason"`
	Summary        string    `json:"summary"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"createdAt"`
	Answered       bool      `json:"answered"` // uma pessoa já respondeu depois
}

// Handoffs lista as passagens para humano (as mais novas primeiro).
func (s *Service) Handoffs(ctx context.Context, openOnly bool, limit int) ([]Handoff, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT h.id::text, h.conversation_id::text, coalesce(h.lane_key, ''), h.reason, h.summary, h.source, h.created_at,
		       EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = h.conversation_id AND `+humanOut+` AND m.sent_at > h.created_at)
		  FROM ai_handoffs h ORDER BY h.created_at DESC LIMIT $1`, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Handoff{}
	for rows.Next() {
		var h Handoff
		if err := rows.Scan(&h.ID, &h.ConversationID, &h.Lane, &h.Reason, &h.Summary, &h.Source, &h.CreatedAt, &h.Answered); err != nil {
			return nil, err
		}
		if openOnly && h.Answered {
			continue
		}
		if len(out) < limit {
			out = append(out, h)
		}
	}
	return out, rows.Err()
}

// --- envio dentro da raia --------------------------------------------------

// applyLane aplica a raia ao envio do atendente. Devolve o texto final a enviar, ou um resultado pronto (modo sombra), ou uma recusa.
func (s *Service) applyLane(ctx context.Context, a Actor, in AttendInput, text string) (final string, shadow *AttendResult, err error) {
	lane, err := s.Lane(ctx, a.Lane)
	if errors.Is(err, ErrNoLane) {
		return "", nil, refuse("raia_desligada", "A raia deste token não existe mais. Peça ao administrador para conferir.")
	}
	if err != nil {
		return "", nil, err
	}
	if in.DocumentID != "" {
		return "", nil, refuse("documento", "Nesta raia a IA não anexa documentos.")
	}
	f, err := s.laneFactsFor(ctx, in)
	if errors.Is(err, errNoOpenConversation) {
		if !lane.CanInitiate {
			return "", nil, refuse("so_responde", "Esta raia só responde a conversas que já existem. Informe o conversation_id de uma conversa da raia.")
		}
		f, err = s.laneFactsForContact(ctx, in.ContactID)
	}
	if err != nil {
		return "", nil, err
	}
	f.Sources = len(in.Sources)
	d := decideLane(lane, f, text)
	d.Text = withSources(d.Text, in.Sources)
	switch {
	case d.Refusal != nil:
		return "", nil, d.Refusal
	case d.Handoff != nil:
		if f.ConvID != "" {
			if _, herr := s.RequestHuman(ctx, a, f.ConvID, d.Handoff.Reason, d.Handoff.Summary, "server"); herr != nil {
				return "", nil, herr
			}
		}
		return "", nil, refuse("passada_para_humano", "Passei esta conversa para um atendente (%s). Não escreva mais aqui; a equipe continua.", d.Handoff.Reason)
	}
	if lane.Mode == LaneShadow {
		id, derr := s.saveDraft(ctx, a, lane.Key, f.ConvID, f.ContactID, d.Text)
		if derr != nil {
			return "", nil, derr
		}
		return "", &AttendResult{MessageID: id, Status: "sombra"}, nil
	}
	return d.Text, nil, nil
}

var errNoOpenConversation = errors.New("automation: sem conversa aberta")

// laneFactsFor: os fatos da conversa indicada (ou da conversa aberta do contato).
func (s *Service) laneFactsFor(ctx context.Context, in AttendInput) (laneFacts, error) {
	convID := in.ConversationID
	if convID == "" {
		err := s.db.QueryRow(ctx, `SELECT id::text FROM conversations WHERE contact_id = $1 AND status <> 'closed'`, in.ContactID).Scan(&convID)
		if errors.Is(err, pgx.ErrNoRows) {
			return laneFacts{}, errNoOpenConversation
		}
		if err != nil {
			return laneFacts{}, err
		}
	}
	f, err := s.laneFacts(ctx, convID)
	if errors.Is(err, ErrNoConversation) {
		return f, refuse("conversa", "Conversa não encontrada.")
	}
	if err == nil && f.ContactID != in.ContactID {
		return f, refuse("conversa", "Esta conversa não é deste contato.")
	}
	return f, err
}

// laneFactsForContact: um contato sem conversa aberta (a IA vai escrever primeiro). Só vale em raia que pode iniciar.
func (s *Service) laneFactsForContact(ctx context.Context, contactID string) (laneFacts, error) {
	f := laneFacts{ContactID: contactID, Now: s.now(), Status: "new"}
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id
		 WHERE c.contact_id = $1 AND `+aiOut+` AND m.sent_at > $2::timestamptz - interval '24 hours'`, contactID, f.Now).Scan(&f.ContactToday)
	return f, err
}

// LaneCheckResult: a resposta de check_send para um token preso a uma raia.
type LaneCheckResult struct {
	Lane      string    `json:"lane"`
	Mode      string    `json:"mode"`
	WouldSend bool      `json:"wouldSend"` // passaria agora (em sombra, vira rascunho: nada vai ao cliente)
	Blockers  []Refusal `json:"blockers"`
	Handoff   string    `json:"handoff,omitempty"` // a conversa seria passada para um humano, por este motivo
	Notes     []string  `json:"notes"`
}

// LaneCheck responde, sem efeitos, se o envio passaria nas regras da raia.
func (s *Service) LaneCheck(ctx context.Context, a Actor, in AttendInput) (LaneCheckResult, error) {
	res := LaneCheckResult{Lane: a.Lane, Blockers: []Refusal{}, Notes: []string{}}
	lane, err := s.Lane(ctx, a.Lane)
	if err != nil {
		res.Blockers = append(res.Blockers, *refuse("raia_desligada", "A raia deste token não existe mais."))
		return res, nil
	}
	res.Mode = lane.Mode
	f, err := s.laneFactsFor(ctx, in)
	if errors.Is(err, errNoOpenConversation) {
		if !lane.CanInitiate {
			res.Blockers = append(res.Blockers, *refuse("so_responde", "Esta raia só responde a conversas que já existem."))
			return res, nil
		}
		f, err = s.laneFactsForContact(ctx, in.ContactID)
	}
	var ref *Refusal
	if errors.As(err, &ref) {
		res.Blockers = append(res.Blockers, *ref)
		return res, nil
	}
	if err != nil {
		return res, err
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		text = "ok" // sem texto, confere só as regras da conversa
		res.Notes = append(res.Notes, "Sem text: só as regras da conversa foram conferidas, não as do conteúdo.")
	}
	f.Sources = len(in.Sources)
	d := decideLane(lane, f, text)
	switch {
	case d.Refusal != nil:
		res.Blockers = append(res.Blockers, *d.Refusal)
	case d.Handoff != nil:
		res.Handoff = d.Handoff.Reason
		res.Blockers = append(res.Blockers, *refuse("passada_para_humano", "Esta conversa seria passada para um atendente (%s).", d.Handoff.Reason))
	default:
		res.WouldSend = true
		if lane.Mode == LaneShadow {
			res.Notes = append(res.Notes, "A raia está em sombra: o envio vira rascunho e nada vai ao cliente.")
		}
	}
	return res, nil
}

// Unattended: uma conversa esperando o primeiro atendimento.
type Unattended struct {
	ConversationID string
	WaitingSince   time.Time // a última mensagem do cliente, ainda sem resposta
}

// UnattendedList: conversas abertas em que o cliente escreveu, passou ao menos minHours e NINGUÉM da equipe iniciou o atendimento
// (nenhuma mensagem enviada por uma pessoa, sem responsável, ninguém respondeu depois do cliente, sem passagem pendente). Se a raia
// tem fila, só as dela. Grupos ficam de fora. A mais antiga primeiro.
func (s *Service) UnattendedList(ctx context.Context, segmentID string, minHours, limit int) ([]Unattended, error) {
	if minHours < 1 {
		minHours = 1
	}
	rows, err := s.db.Query(ctx, `
		SELECT c.id::text, li.at
		  FROM conversations c
		  JOIN contacts ct ON ct.id = c.contact_id
		  CROSS JOIN LATERAL (SELECT max(m.sent_at) AS at FROM messages m
		                       WHERE m.conversation_id = c.id AND m.direction = 'in' AND m.deleted_at IS NULL
		                         AND coalesce(m.payload->>'historical', '') <> 'true') li
		 WHERE c.status <> 'closed' AND c.assignee_id IS NULL AND ct.kind <> 'group'
		   AND NOT (c.snoozed_until IS NOT NULL AND c.snoozed_until > now())
		   AND ($1 = '' OR c.segment_id::text = $1)
		   AND li.at IS NOT NULL AND li.at < now() - make_interval(hours => $2)
		   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND `+humanOut+`)
		   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.direction = 'out' AND m.deleted_at IS NULL AND m.sent_at > li.at)
		   AND NOT EXISTS (SELECT 1 FROM ai_handoffs h WHERE h.conversation_id = c.id)
		 ORDER BY li.at ASC LIMIT $3`, segmentID, minHours, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Unattended
	for rows.Next() {
		var u Unattended
		if err := rows.Scan(&u.ConversationID, &u.WaitingSince); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// OpenHandoffSet: das conversas dadas, as que estão passadas para uma pessoa e ainda sem resposta dela (a IA fica em silêncio nelas).
func (s *Service) OpenHandoffSet(ctx context.Context, convIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(convIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT h.conversation_id::text FROM ai_handoffs h
		 WHERE h.conversation_id = ANY($1::uuid[])
		   AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = h.conversation_id AND `+humanOut+` AND m.sent_at > h.created_at)`, convIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
