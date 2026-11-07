package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

const (
	// WhatsApp só deixa editar por 20 minutos e apagar para todos por cerca de dois dias.
	editWindow   = 20 * time.Minute
	revokeWindow = 48 * time.Hour
	quoteRunes   = 160
	maxForwards  = 5
)

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// quoteSnapshot é a prévia da mensagem citada guardada junto da resposta (continua certa mesmo se a original mudar).
func quoteSnapshot(id, ext, dir, kind, body, sender, senderJID string, payload []byte) map[string]any {
	text := body
	if text == "" {
		var p map[string]any
		_ = json.Unmarshal(payload, &p)
		if fn, _ := p["fileName"].(string); fn != "" {
			text = fn
		}
	}
	q := map[string]any{"externalId": ext, "fromMe": dir == "out", "kind": kind, "text": clip(text, quoteRunes), "author": sender}
	if id != "" {
		q["id"] = id
	}
	if senderJID != "" {
		q["senderJid"] = senderJID
	}
	return q
}

// incomingQuote monta a citação de uma mensagem recebida, preferindo a mensagem guardada.
func (s *Service) incomingQuote(ctx context.Context, tx pgx.Tx, m whatsapp.InboundMessage) map[string]any {
	var id, dir, kind, body, sender string
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT id::text, direction, kind, body, sender_name, payload FROM messages
		WHERE chat_jid = $1 AND external_id = $2 AND deleted_at IS NULL`, m.ChatJID, m.Quote.ExternalID).
		Scan(&id, &dir, &kind, &body, &sender, &payload)
	if err == nil {
		return quoteSnapshot(id, m.Quote.ExternalID, dir, kind, body, sender, "", payload)
	}
	dir = "in"
	if m.Quote.FromMe {
		dir = "out"
	}
	return quoteSnapshot("", m.Quote.ExternalID, dir, "text", m.Quote.Text, "", m.Quote.SenderJID, nil)
}

// quoteFromPayload reconstrói a citação para o WhatsApp a partir da prévia guardada (usado também no reenvio).
func quoteFromPayload(payload map[string]any) *whatsapp.Quote {
	q, _ := payload["quoted"].(map[string]any)
	ext, _ := q["externalId"].(string)
	if ext == "" {
		return nil
	}
	fromMe, _ := q["fromMe"].(bool)
	jid, _ := q["senderJid"].(string)
	text, _ := q["text"].(string)
	return &whatsapp.Quote{ExternalID: ext, FromMe: fromMe, SenderJID: jid, Text: text}
}

// resolveReply valida a mensagem respondida (mesmo contato, não apagada) e devolve a prévia.
func (s *Service) resolveReply(ctx context.Context, tx pgx.Tx, convID, replyTo string) (map[string]any, error) {
	if !httpx.ValidUUID(replyTo) {
		return nil, httpx.FieldErrors{"replyTo": "Mensagem respondida inválida."}
	}
	var ext, dir, kind, body, sender, senderJID string
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT m.external_id, m.direction, m.kind, m.body, m.sender_name, coalesce(m.payload->>'senderJid', ''), m.payload
		  FROM messages m JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.id = $1 AND m.deleted_at IS NULL
		   AND c.contact_id = (SELECT contact_id FROM conversations WHERE id = $2)`, replyTo, convID).
		Scan(&ext, &dir, &kind, &body, &sender, &senderJID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.FieldErrors{"replyTo": "A mensagem respondida não existe mais."}
	}
	if err != nil {
		return nil, err
	}
	return quoteSnapshot(replyTo, ext, dir, kind, body, sender, senderJID, payload), nil
}

// ingestChange aplica uma edição ou um "apagar para todos" vindo do WhatsApp.
func (s *Service) ingestChange(ctx context.Context, m whatsapp.InboundMessage) error {
	var msgID, convID string
	if m.EditOf != "" {
		// O eco de uma edição do WhatsApp traz a assinatura que só o cliente vê.
		var author string
		err := s.db.QueryRow(ctx, `SELECT sender_name FROM messages
			WHERE chat_jid = $1 AND external_id = $2 AND direction = 'out' AND sender_user_id IS NOT NULL`,
			m.ChatJID, m.EditOf).Scan(&author)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if author != "" {
			m.Body = strings.TrimPrefix(m.Body, signedText(author, ""))
		}
		err = s.db.QueryRow(ctx, `
			UPDATE messages SET body = $3, edited_at = now()
			 WHERE chat_jid = $1 AND external_id = $2 AND deleted_at IS NULL AND kind IN ('text', 'image', 'document')
			   AND body <> $3
			RETURNING id::text, conversation_id::text`, m.ChatJID, m.EditOf, m.Body).Scan(&msgID, &convID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // mensagem desconhecida ou texto igual
		}
		if err != nil {
			return err
		}
	} else {
		var keys []string
		err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `
				UPDATE messages SET deleted_at = now(), body = '',
				       payload = payload - 'quoted' - 'thumb' - 'lat' - 'lng' - 'name' - 'address' - 'fileName' - 'waveform' - 'label'
				 WHERE chat_jid = $1 AND external_id = $2 AND deleted_at IS NULL
				RETURNING id::text, conversation_id::text`, m.ChatJID, m.RevokeOf).Scan(&msgID, &convID)
			if err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `DELETE FROM attachments WHERE message_id = $1 RETURNING storage_key`, msgID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var k string
				if err := rows.Scan(&k); err != nil {
					return err
				}
				keys = append(keys, k)
			}
			return rows.Err()
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, k := range keys {
			if s.store != nil {
				_ = s.store.Delete(context.WithoutCancel(ctx), k)
			}
		}
	}
	after := s.publishState(ctx, convID, nil)
	s.publishMessage(ctx, EventMessageUpdated, msgID, after)
	return nil
}

// ---------------------------------------------------------------------------
// Editar / apagar / encaminhar

type ownMsg struct {
	id, convID, chatJID, ext, dir, kind, body string
	senderName                                string
	status                                    *string
	sentAt                                    time.Time
	senderUser                                *string
	deleted                                   bool
}

// loadMessage lê a mensagem e confere que quem pede enxerga a conversa dela.
func (s *Service) loadMessage(ctx context.Context, v Viewer, msgID string) (ownMsg, Conversation, error) {
	var r ownMsg
	if !httpx.ValidUUID(msgID) {
		return r, Conversation{}, ErrNotFound
	}
	err := s.db.QueryRow(ctx, `
		SELECT m.id::text, m.conversation_id::text, m.chat_jid, m.external_id, m.direction, m.kind, m.body, m.status,
		       m.sent_at, m.sender_user_id::text, m.deleted_at IS NOT NULL, m.sender_name
		  FROM messages m WHERE m.id = $1`, msgID).
		Scan(&r.id, &r.convID, &r.chatJID, &r.ext, &r.dir, &r.kind, &r.body, &r.status, &r.sentAt, &r.senderUser, &r.deleted, &r.senderName)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, Conversation{}, ErrNotFound
	}
	if err != nil {
		return r, Conversation{}, err
	}
	c, err := s.Get(ctx, v, r.convID)
	return r, c, err
}

// canManage: quem escreveu a mensagem, o administrador ou, para mensagens enviadas pelo celular, o responsável.
func canManage(v Viewer, r ownMsg, c Conversation) bool {
	if v.Admin || (r.senderUser != nil && *r.senderUser == v.ID) {
		return true
	}
	return r.senderUser == nil && c.Assignee != nil && c.Assignee.ID == v.ID
}

// EditMessage edita o texto de uma mensagem enviada pelo escritório (até 20 minutos depois do envio).
func (s *Service) EditMessage(ctx context.Context, v Viewer, msgID, text string) (Message, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxTextRunes {
		return nil, httpx.FieldErrors{"text": fmt.Sprintf("Digite uma mensagem de até %d caracteres.", maxTextRunes)}
	}
	r, c, err := s.loadMessage(ctx, v, msgID)
	if err != nil {
		return nil, err
	}
	if r.dir != "out" || r.kind != "text" || r.deleted || r.status == nil || (*r.status != "sent" && *r.status != "delivered" && *r.status != "read") {
		return nil, httpx.FieldErrors{"text": "Esta mensagem não pode ser editada."}
	}
	if !canManage(v, r, c) {
		return nil, ErrForbidden
	}
	if time.Since(r.sentAt) > editWindow {
		return nil, httpx.FieldErrors{"text": "O WhatsApp só permite editar até 20 minutos depois do envio."}
	}
	if s.sender == nil || !s.sender.Connected() {
		return nil, ErrUnavailable
	}
	if text == r.body {
		return s.message(ctx, msgID)
	}
	wireText := text
	if r.senderUser != nil {
		wireText = signedText(r.senderName, text)
	}
	if err := s.sender.EditText(ctx, r.chatJID, r.ext, wireText); err != nil {
		return nil, httpx.FieldErrors{"text": "Não foi possível editar pelo WhatsApp."}
	}
	if _, err := s.db.Exec(ctx, `UPDATE messages SET body = $2, edited_at = now() WHERE id = $1`, msgID, text); err != nil {
		return nil, err
	}
	after := s.publishState(ctx, r.convID, nil)
	s.publishMessage(ctx, EventMessageUpdated, msgID, after)
	return s.message(ctx, msgID)
}

// DeleteMessage: scope "me" esconde só para quem pediu; "everyone" apaga no WhatsApp (só mensagens enviadas
// pelo escritório, até 48 horas depois).
func (s *Service) DeleteMessage(ctx context.Context, v Viewer, msgID, scope string) error {
	r, c, err := s.loadMessage(ctx, v, msgID)
	if err != nil {
		return err
	}
	switch scope {
	case "me":
		_, err := s.db.Exec(ctx, `INSERT INTO message_hidden (user_id, message_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, v.ID, msgID)
		return err
	case "everyone":
		if r.deleted {
			return nil
		}
		if r.dir != "out" || r.status == nil || *r.status == "failed" || *r.status == "pending" {
			return httpx.FieldErrors{"scope": "Só dá para apagar para todos mensagens que o escritório enviou."}
		}
		if !canManage(v, r, c) {
			return ErrForbidden
		}
		if time.Since(r.sentAt) > revokeWindow {
			return httpx.FieldErrors{"scope": "O WhatsApp só permite apagar para todos até 2 dias depois do envio."}
		}
		if s.sender == nil || !s.sender.Connected() {
			return ErrUnavailable
		}
		if err := s.sender.Revoke(ctx, r.chatJID, r.ext); err != nil {
			return httpx.FieldErrors{"scope": "Não foi possível apagar pelo WhatsApp."}
		}
		return s.ingestChange(ctx, whatsapp.InboundMessage{ChatJID: r.chatJID, RevokeOf: r.ext})
	default:
		return httpx.FieldErrors{"scope": "Escolha apagar para mim ou para todos."}
	}
}

// ForwardResult é o resultado por conversa de destino.
type ForwardResult struct {
	ConversationID string `json:"conversationId"`
	OK             bool   `json:"ok"`
	Error          string `json:"error,omitempty"`
}

// ForwardMessage reenvia o conteúdo de uma mensagem para outras conversas (cada uma segue as regras normais de envio,
// inclusive assumir o atendimento).
func (s *Service) ForwardMessage(ctx context.Context, v Viewer, msgID string, targets []string) ([]ForwardResult, error) {
	if len(targets) == 0 || len(targets) > maxForwards {
		return nil, httpx.FieldErrors{"conversationIds": fmt.Sprintf("Escolha de 1 a %d conversas.", maxForwards)}
	}
	r, _, err := s.loadMessage(ctx, v, msgID)
	if err != nil {
		return nil, err
	}
	if r.deleted {
		return nil, httpx.FieldErrors{"conversationIds": "Esta mensagem foi apagada."}
	}
	var payload map[string]any
	var raw []byte
	var attKey *string
	if err := s.db.QueryRow(ctx, `SELECT payload, (SELECT a.storage_key FROM attachments a WHERE a.message_id = m.id) FROM messages m WHERE m.id = $1`, msgID).
		Scan(&raw, &attKey); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &payload)
	str := func(k string) string { x, _ := payload[k].(string); return x }
	num := func(k string) float64 { x, _ := payload[k].(float64); return x }

	out := make([]ForwardResult, 0, len(targets))
	seen := map[string]bool{}
	for _, t := range targets {
		if seen[t] {
			continue
		}
		seen[t] = true
		in := SendInput{Kind: r.kind, Text: r.body}
		switch r.kind {
		case "text":
		case "location":
			in.Lat, in.Lng, in.Name, in.Address, in.Text = num("lat"), num("lng"), str("name"), str("address"), ""
		case "image", "audio", "document":
			if attKey == nil || s.store == nil {
				out = append(out, ForwardResult{ConversationID: t, Error: "O arquivo desta mensagem não está disponível."})
				continue
			}
			data, err := s.store.Get(ctx, *attKey)
			if err != nil {
				out = append(out, ForwardResult{ConversationID: t, Error: "O arquivo desta mensagem não está disponível."})
				continue
			}
			in.Att = &media.Processed{Kind: media.Kind(r.kind), Data: data, MimeType: str("mimeType"), FileName: str("fileName"),
				Width: int(num("width")), Height: int(num("height")), DurationSec: int(num("durationSec")), ScanStatus: "clean"}
			in.HD, _ = payload["hd"].(bool)
		default:
			out = append(out, ForwardResult{ConversationID: t, Error: "Este tipo de mensagem não pode ser encaminhado."})
			continue
		}
		if _, _, err := s.Send(ctx, v, t, in); err != nil {
			out = append(out, ForwardResult{ConversationID: t, Error: forwardError(err)})
			continue
		}
		out = append(out, ForwardResult{ConversationID: t, OK: true})
	}
	return out, nil
}

func forwardError(err error) string {
	var fe httpx.FieldErrors
	var taken *TakenError
	switch {
	case errors.As(err, &taken):
		return "Outra pessoa já atende esta conversa."
	case errors.Is(err, ErrNotFound):
		return "Conversa não encontrada."
	case errors.Is(err, ErrClosed):
		return "Atendimento encerrado."
	case errors.Is(err, ErrUnavailable):
		return "WhatsApp desconectado."
	case errors.As(err, &fe):
		for _, m := range fe {
			return m
		}
	}
	return "Não foi possível encaminhar."
}
