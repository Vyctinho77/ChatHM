// Package conversations gerencia contatos, conversas e mensagens.
//
// Fase 2: todas as conversas ficam na fila geral (sem segmento), visível a todos
// os usuários autenticados — regra 3 do blueprint. A Fase 3 adiciona segmentos,
// filas por equipe e a atribuição atômica ao primeiro que responder.
package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

var ErrNotFound = errors.New("conversations: não encontrada")

// Publisher é satisfeito por *realtime.Hub.
type Publisher interface {
	Publish(typ string, data any, filter realtime.Filter)
}

type Service struct {
	db  *pgxpool.Pool
	pub Publisher
}

func NewService(db *pgxpool.Pool, pub Publisher) *Service { return &Service{db: db, pub: pub} }

// Eventos em tempo real.
const (
	EventConversationUpdated = "conversation.updated"
	EventMessageCreated      = "message.created"
)

// ---------------------------------------------------------------------------
// Ingestão

// Ingest grava uma mensagem vinda do provedor. É idempotente: o mesmo
// (chat, ID externo) nunca gera duas mensagens nem incrementa não lidas duas vezes.
// Retorna created=false para duplicatas.
func (s *Service) Ingest(ctx context.Context, m whatsapp.InboundMessage) (bool, error) {
	if m.ExternalID == "" || m.ChatJID == "" || m.ContactWAID == "" {
		return false, errors.New("conversations: mensagem sem identificadores")
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	payload := m.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	var convID, msgID string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		// Duplicata? Sai cedo, sem tocar no contato nem na conversa.
		var dup bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM messages WHERE chat_jid = $1 AND external_id = $2)`,
			m.ChatJID, m.ExternalID).Scan(&dup); err != nil {
			return err
		}
		if dup {
			return nil
		}

		var contactID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO contacts (wa_id, phone, name) VALUES ($1, $2, $3)
			ON CONFLICT (wa_id) DO UPDATE SET
			    phone = CASE WHEN contacts.phone = '' THEN EXCLUDED.phone ELSE contacts.phone END,
			    name  = CASE WHEN contacts.name = '' THEN EXCLUDED.name ELSE contacts.name END
			RETURNING id`,
			m.ContactWAID, m.ContactPhone, strings.TrimSpace(m.PushName)).Scan(&contactID); err != nil {
			return err
		}

		// Conversa aberta do contato (no máximo uma — índice único parcial).
		err := tx.QueryRow(ctx, `
			INSERT INTO conversations (contact_id, last_message_at) VALUES ($1, $2)
			ON CONFLICT (contact_id) WHERE status <> 'closed' DO NOTHING
			RETURNING id`, contactID, m.Timestamp).Scan(&convID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx,
				`SELECT id FROM conversations WHERE contact_id = $1 AND status <> 'closed' FOR UPDATE`,
				contactID).Scan(&convID)
		}
		if err != nil {
			return err
		}

		dir, status := "in", (*string)(nil)
		if m.FromMe {
			dir = "out"
			sent := "sent"
			status = &sent
		}
		pj, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, chat_jid, external_id, direction, kind, body, payload, sender_name, status, sent_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (chat_jid, external_id) DO NOTHING
			RETURNING id`,
			convID, m.ChatJID, m.ExternalID, dir, string(m.Kind), m.Body, pj, strings.TrimSpace(m.PushName), status, m.Timestamp,
		).Scan(&msgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // corrida com outra entrega do mesmo evento
		}
		if err != nil {
			return err
		}

		unread := 0
		if !m.FromMe {
			unread = 1
		}
		_, err = tx.Exec(ctx, `
			UPDATE conversations
			   SET last_message_at = greatest(last_message_at, $2),
			       unread_count = unread_count + $3
			 WHERE id = $1`, convID, m.Timestamp, unread)
		return err
	})
	if err != nil || msgID == "" {
		return false, err
	}

	s.publishConversation(ctx, convID)
	if msg, err := s.message(ctx, msgID); err == nil && s.pub != nil {
		s.pub.Publish(EventMessageCreated, msg, nil)
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Consultas

type Contact struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Company string `json:"company,omitempty"`
	Note    string `json:"note"`
}

type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Message é serializada "achatada", no mesmo formato do frontend
// (campos de mídia/localização no nível de cima).
type Message map[string]any

type Conversation struct {
	ID          string    `json:"id"`
	Contact     Contact   `json:"contact"`
	Segment     string    `json:"segment"`
	Status      string    `json:"status"`
	Assignee    *Ref      `json:"assignee,omitempty"`
	UnreadCount int       `json:"unreadCount"`
	Tags        []string  `json:"tags"`
	LastMessage Message   `json:"lastMessage,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const messageCols = `m.id, m.conversation_id, m.direction, m.kind, m.body, m.payload, m.status, m.sent_at, m.sender_name`

func scanMessage(row pgx.Row) (Message, error) {
	var (
		id, convID, dir, kind, body, sender string
		payload                             []byte
		status                              *string
		sentAt                              time.Time
	)
	if err := row.Scan(&id, &convID, &dir, &kind, &body, &payload, &status, &sentAt, &sender); err != nil {
		return nil, err
	}
	return buildMessage(id, convID, dir, kind, body, payload, status, sentAt, sender), nil
}

func buildMessage(id, convID, dir, kind, body string, payload []byte, status *string, sentAt time.Time, sender string) Message {
	m := Message{}
	_ = json.Unmarshal(payload, &m)
	m["id"], m["conversationId"], m["direction"], m["kind"] = id, convID, dir, kind
	m["sentAt"] = sentAt.UTC().Format(time.RFC3339Nano)
	if status != nil {
		m["status"] = *status
	}
	if dir == "out" && sender != "" {
		m["authorName"] = sender
	}
	switch kind {
	case "text", "unsupported":
		m["text"] = body
	case "image", "document":
		if body != "" {
			m["caption"] = body
		}
	}
	return m
}

func displayName(name, phone, waID string) string {
	if name != "" {
		return name
	}
	if phone != "" {
		return FormatPhone(phone)
	}
	return waID
}

const listQuery = `
	SELECT c.id, c.status, c.unread_count, c.last_message_at,
	       ct.id, ct.name, ct.phone, ct.wa_id, ct.company, ct.note,
	       u.id, u.name,
	       lm.id, lm.direction, lm.kind, lm.body, lm.payload, lm.status, lm.sent_at, lm.sender_name
	  FROM conversations c
	  JOIN contacts ct ON ct.id = c.contact_id
	  LEFT JOIN users u ON u.id = c.assignee_id
	  LEFT JOIN LATERAL (
	       SELECT * FROM messages m WHERE m.conversation_id = c.id
	        ORDER BY m.sent_at DESC, m.created_at DESC LIMIT 1) lm ON true`

func scanConversation(row pgx.Row) (Conversation, error) {
	var (
		c                                         Conversation
		name, phone, waID                         string
		uID, uName                                *string
		mID, mDir, mKind, mBody, mStatus, mSender *string
		mPayload                                  []byte
		mSent                                     *time.Time
	)
	err := row.Scan(&c.ID, &c.Status, &c.UnreadCount, &c.UpdatedAt,
		&c.Contact.ID, &name, &phone, &waID, &c.Contact.Company, &c.Contact.Note,
		&uID, &uName,
		&mID, &mDir, &mKind, &mBody, &mPayload, &mStatus, &mSent, &mSender)
	if err != nil {
		return c, err
	}
	c.Contact.Name = displayName(name, phone, waID)
	c.Contact.Phone = FormatPhone(phone)
	if c.Contact.Phone == "" {
		c.Contact.Phone = waID
	}
	c.Segment = "geral"
	c.Tags = []string{}
	if uID != nil {
		c.Assignee = &Ref{ID: *uID, Name: *uName}
	}
	if mID != nil {
		c.LastMessage = buildMessage(*mID, c.ID, *mDir, *mKind, *mBody, mPayload, mStatus, *mSent, *mSender)
	}
	return c, nil
}

func (s *Service) List(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.Query(ctx, listQuery+` ORDER BY c.last_message_at DESC LIMIT 300`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (Conversation, error) {
	if !httpx.ValidUUID(id) {
		return Conversation{}, ErrNotFound
	}
	c, err := scanConversation(s.db.QueryRow(ctx, listQuery+` WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

// Messages devolve as mensagens mais recentes (até 300) em ordem cronológica.
func (s *Service) Messages(ctx context.Context, conversationID string) ([]Message, error) {
	if _, err := s.Get(ctx, conversationID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT * FROM (
		    SELECT `+messageCols+`, m.created_at FROM messages m
		     WHERE m.conversation_id = $1
		     ORDER BY m.sent_at DESC, m.created_at DESC LIMIT 300) x
		 ORDER BY sent_at, created_at`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var (
			id, convID, dir, kind, body, sender string
			payload                             []byte
			status                              *string
			sentAt, createdAt                   time.Time
		)
		if err := rows.Scan(&id, &convID, &dir, &kind, &body, &payload, &status, &sentAt, &sender, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, buildMessage(id, convID, dir, kind, body, payload, status, sentAt, sender))
	}
	return out, rows.Err()
}

func (s *Service) message(ctx context.Context, id string) (Message, error) {
	return scanMessage(s.db.QueryRow(ctx, `SELECT `+messageCols+` FROM messages m WHERE m.id = $1`, id))
}

func (s *Service) MarkRead(ctx context.Context, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	tag, err := s.db.Exec(ctx, `UPDATE conversations SET unread_count = 0 WHERE id = $1 AND unread_count > 0`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.publishConversation(ctx, id)
	}
	return nil
}

type ContactInput struct {
	Name    *string `json:"name"`
	Company *string `json:"company"`
	Note    *string `json:"note"`
}

func (s *Service) UpdateContact(ctx context.Context, id string, in ContactInput) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	f := httpx.FieldErrors{}
	trim := func(p *string) {
		if p != nil {
			*p = strings.TrimSpace(*p)
		}
	}
	trim(in.Name)
	trim(in.Company)
	if in.Name != nil && utf8.RuneCountInString(*in.Name) > 120 {
		f["name"] = "Nome deve ter até 120 caracteres."
	}
	if in.Company != nil && utf8.RuneCountInString(*in.Company) > 120 {
		f["company"] = "Empresa deve ter até 120 caracteres."
	}
	if in.Note != nil && utf8.RuneCountInString(*in.Note) > 2000 {
		f["note"] = "Observação deve ter até 2000 caracteres."
	}
	if err := f.Err(); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE contacts SET name = coalesce($2, name), company = coalesce($3, company), note = coalesce($4, note)
		 WHERE id = $1`, id, in.Name, in.Company, in.Note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	var convID string
	if err := s.db.QueryRow(ctx,
		`SELECT id FROM conversations WHERE contact_id = $1 ORDER BY last_message_at DESC LIMIT 1`, id).Scan(&convID); err == nil {
		s.publishConversation(ctx, convID)
	}
	return nil
}

func (s *Service) publishConversation(ctx context.Context, id string) {
	if s.pub == nil {
		return
	}
	if c, err := s.Get(ctx, id); err == nil {
		s.pub.Publish(EventConversationUpdated, c, nil)
	}
}

// FormatPhone formata números brasileiros E.164 para exibição (+55 92 99811-2040).
func FormatPhone(e164 string) string {
	d := strings.TrimPrefix(e164, "+")
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		ddd, rest := d[2:4], d[4:]
		cut := len(rest) - 4
		return "+55 " + ddd + " " + rest[:cut] + "-" + rest[cut:]
	}
	return e164
}
