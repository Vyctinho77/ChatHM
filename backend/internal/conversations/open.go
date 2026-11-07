package conversations

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// jidOf: destino do WhatsApp a partir do identificador do contato (número ou JID completo, como LID).
func jidOf(waID string) string {
	waID = strings.TrimSpace(waID)
	switch {
	case waID == "":
		return ""
	case strings.Contains(waID, "@"):
		return waID
	default:
		return waID + "@s.whatsapp.net"
	}
}

// NormalizePhone devolve só os dígitos em formato internacional. Sem "+", números de 10 ou 11 dígitos
// (DDD + número) ganham o 55 do Brasil.
func NormalizePhone(in string) (string, error) {
	in = strings.TrimSpace(in)
	international := strings.HasPrefix(in, "+")
	var b strings.Builder
	for _, r := range in {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := strings.TrimLeft(b.String(), "0")
	if !international && (len(d) == 10 || len(d) == 11) {
		d = "55" + d
	}
	if len(d) < 11 || len(d) > 15 {
		return "", httpx.FieldErrors{"phone": "Informe o telefone com DDD (ex.: 92 99999-9999) ou com o código do país (+55…)."}
	}
	return d, nil
}

// OpenForContact abre (ou devolve) o atendimento de um contato já cadastrado: quem clica passa a ser o
// responsável, e a conversa fica privada para a pessoa (e administradores) até ela mandar a primeira mensagem
// ou encerrar. Se o cliente já está sendo atendido por outra pessoa, responde TakenError.
func (s *Service) OpenForContact(ctx context.Context, v Viewer, contactID string) (Conversation, error) {
	if !httpx.ValidUUID(contactID) {
		return Conversation{}, ErrNotFound
	}
	var convID string
	var created bool
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contacts WHERE id = $1)`, contactID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		// Atendimento aberto deste contato (no máximo um: índice único parcial).
		var assigneeName string
		var visible bool
		err := tx.QueryRow(ctx, `
			SELECT c.id, coalesce(u.name, ''),
			       (`+forViewerSQL+`)
			  FROM conversations c
			  LEFT JOIN segments s ON s.id = c.segment_id
			  LEFT JOIN users u ON u.id = c.assignee_id
			 WHERE c.contact_id = $3 AND c.status <> 'closed'
			 FOR UPDATE OF c`, v.ID, v.Admin, contactID).Scan(&convID, &assigneeName, &visible)
		switch {
		case err == nil:
			if !visible {
				return &TakenError{Assignee: assigneeName}
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO conversations (contact_id, status, assignee_id) VALUES ($1, 'in_progress', $2)
			ON CONFLICT (contact_id) WHERE status <> 'closed' DO NOTHING
			RETURNING id`, contactID, v.ID).Scan(&convID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) { // corrida: alguém abriu primeiro
				return &TakenError{}
			}
			return err
		}
		created = true
		if _, err := tx.Exec(ctx, `INSERT INTO conversation_assignments (conversation_id, user_id) VALUES ($1, $2)`, convID, v.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: "conversation.open", TargetType: "conversation", TargetID: convID,
			Metadata: map[string]any{"contactId": contactID}})
	})
	if err != nil {
		return Conversation{}, err
	}
	if created {
		s.publishState(ctx, convID, nil)
	}
	return s.Get(ctx, v, convID)
}

// OpenByPhone cria (se preciso) o contato a partir do telefone e abre o atendimento.
func (s *Service) OpenByPhone(ctx context.Context, v Viewer, phone, name string) (Conversation, error) {
	digits, err := NormalizePhone(phone)
	if err != nil {
		return Conversation{}, err
	}
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n > 120 {
		return Conversation{}, httpx.FieldErrors{"name": "O nome pode ter até 120 caracteres."}
	}
	var contactID string
	if err := s.db.QueryRow(ctx, `
		INSERT INTO contacts (wa_id, phone, name) VALUES ($1, $2, $3)
		ON CONFLICT (wa_id) DO UPDATE SET
		    phone = CASE WHEN contacts.phone = '' THEN EXCLUDED.phone ELSE contacts.phone END,
		    name  = CASE WHEN contacts.name = '' THEN EXCLUDED.name ELSE contacts.name END
		RETURNING id`, digits, "+"+digits, name).Scan(&contactID); err != nil {
		return Conversation{}, err
	}
	return s.OpenForContact(ctx, v, contactID)
}

// forViewerSQL é a mesma regra de visibilidade, como expressão booleana avaliável no SELECT.
var forViewerSQL = forViewer

// ---------------------------------------------------------------------------
// Busca

// MessageHit é uma mensagem encontrada pela busca.
type MessageHit struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	ContactID      string `json:"contactId"`
	// OpenConversationID: atendimento aberto do contato que a pessoa pode ver (para abrir o resultado de um atendimento encerrado).
	OpenConversationID *string   `json:"openConversationId,omitempty"`
	ContactName        string    `json:"contactName"`
	Direction          string    `json:"direction"`
	Kind               string    `json:"kind"`
	Snippet            string    `json:"snippet"`
	SentAt             time.Time `json:"sentAt"`
	Closed             bool      `json:"closed"`
}

type SearchResult struct {
	Contacts []ContactRecord `json:"contacts"`
	Messages []MessageHit    `json:"messages"`
}

var foldTable = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

// fold: minúsculas sem acentos, preservando o número de caracteres (para localizar o trecho).
func fold(s string) []rune {
	return []rune(foldTable.Replace(strings.Map(unicode.ToLower, s)))
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// snippet recorta o texto em volta da primeira ocorrência da busca.
func snippet(body, q string) string {
	runes := []rune(strings.Join(strings.Fields(body), " "))
	fq := fold(q)
	hay := fold(string(runes))
	if len(hay) != len(runes) { // folding inesperado: devolve o começo
		if len(runes) > 160 {
			return string(runes[:160]) + "…"
		}
		return string(runes)
	}
	idx := -1
	for i := 0; i+len(fq) <= len(hay); i++ {
		if string(hay[i:i+len(fq)]) == string(fq) {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	start, end := max(0, idx-50), min(len(runes), idx+len(fq)+110)
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// Search procura contatos (nome, empresa, telefone) e o conteúdo de TODAS as mensagens que o viewer pode ver
// (texto, legendas e nomes de arquivo), ignorando acentos e maiúsculas.
func (s *Service) Search(ctx context.Context, v Viewer, q string) (SearchResult, error) {
	q = strings.TrimSpace(q)
	if n := len([]rune(q)); n < 2 {
		return SearchResult{Contacts: []ContactRecord{}, Messages: []MessageHit{}}, nil
	} else if n > 100 {
		q = string([]rune(q)[:100])
	}
	pattern := "%" + likeEscape(string(fold(q))) + "%"
	digits := ""
	for _, r := range q {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	digitsPattern := "%%"
	if len(digits) >= 3 {
		digitsPattern = "%" + digits + "%"
	}
	res := SearchResult{Contacts: []ContactRecord{}, Messages: []MessageHit{}}

	crows, err := s.db.Query(ctx, `SELECT ct.id, ct.name, ct.phone, ct.wa_id, ct.company, ct.note, ct.kind, ct.avatar_version, lc.id::text
		FROM contacts ct LEFT JOIN LATERAL (
		  SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
		   WHERE c.contact_id = ct.id AND c.status <> 'closed' AND `+forViewer+`
		   ORDER BY c.last_message_at DESC LIMIT 1) lc ON true
		LEFT JOIN LATERAL (
		  SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
		   WHERE c.contact_id = ct.id AND `+forViewer+` LIMIT 1) la ON true
		WHERE ($2::boolean OR la.id IS NOT NULL)
		  AND (unaccent(lower(ct.name || ' ' || ct.company)) LIKE $3 ESCAPE '\'
		       OR ($4 <> '%%' AND regexp_replace(ct.phone || ct.wa_id, '\D', '', 'g') LIKE $4))
		ORDER BY lower(coalesce(nullif(ct.name, ''), nullif(ct.phone, ''), ct.wa_id)) LIMIT 20`,
		v.ID, v.Admin, pattern, digitsPattern)
	if err != nil {
		return res, err
	}
	defer crows.Close()
	for crows.Next() {
		var c ContactRecord
		var name, phone, waID, kind string
		var ver int64
		if err := crows.Scan(&c.ID, &name, &phone, &waID, &c.Company, &c.Note, &kind, &ver, &c.ConversationID); err != nil {
			return res, err
		}
		c.finish(name, phone, waID, kind, ver)
		res.Contacts = append(res.Contacts, c)
	}
	if err := crows.Err(); err != nil {
		return res, err
	}

	mrows, err := s.db.Query(ctx, `
		SELECT m.id, m.conversation_id, ct.id::text, oc.id::text, ct.name, ct.phone, ct.wa_id, m.direction, m.kind,
		       coalesce(nullif(m.body, ''), m.payload->>'fileName', ''), m.sent_at, c.status = 'closed'
		  FROM messages m
		  JOIN conversations c ON c.id = m.conversation_id
		  JOIN contacts ct ON ct.id = c.contact_id
		  LEFT JOIN segments s ON s.id = c.segment_id
		  LEFT JOIN LATERAL (
		       SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
		        WHERE c.contact_id = ct.id AND c.status <> 'closed' AND `+forViewer+`
		        LIMIT 1) oc ON true
		 WHERE `+forViewer+` AND m.kind <> 'location'
		   AND unaccent(lower(m.body || ' ' || coalesce(m.payload->>'fileName', ''))) LIKE $3 ESCAPE '\'
		 ORDER BY m.sent_at DESC LIMIT 40`, v.ID, v.Admin, pattern)
	if err != nil {
		return res, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var h MessageHit
		var name, phone, waID, text string
		if err := mrows.Scan(&h.ID, &h.ConversationID, &h.ContactID, &h.OpenConversationID, &name, &phone, &waID, &h.Direction, &h.Kind, &text, &h.SentAt, &h.Closed); err != nil {
			return res, err
		}
		h.ContactName, h.Snippet = displayName(name, phone, waID), snippet(text, q)
		res.Messages = append(res.Messages, h)
	}
	return res, mrows.Err()
}
