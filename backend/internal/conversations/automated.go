package conversations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// ErrAutomatedBlocked: o envio automático não pode sair agora (a conversa mudou desde a aprovação). Reason diz por quê.
type ErrAutomatedBlocked struct{ Reason string }

func (e *ErrAutomatedBlocked) Error() string {
	return "conversations: envio automático bloqueado: " + e.Reason
}

// AutomatedSend é uma mensagem iniciada pela IA (fila de envios automáticos), nunca uma resposta a uma pessoa.
type AutomatedSend struct {
	ID        string           // id da mensagem (uuid, novo a cada tentativa)
	ContactID string           // contato existente
	Author    string           // assinatura em negrito para o cliente (nunca o nome de uma pessoa)
	Text      string           // corpo final, já com variáveis e rodapé
	Att       *media.Processed // PDF opcional (da biblioteca)
	Provider  string           // claude | chatgpt | grok | gemini | other: aparece como logo na conversa
	App       string           // nome do aplicativo de IA
	SendID    string           // id do envio na fila (rastreio)
}

// SendAutomated envia uma mensagem automática. Diferente de Send, ela:
//   - NÃO assume a conversa, não muda responsável, status nem fila;
//   - NÃO zera as não lidas do cliente (ninguém leu);
//   - marca a mensagem como automática, com o provedor da IA (a tela mostra a logo no lugar do nome de um atendente);
//   - recusa se a conversa está adiada (alguém decidiu parar), ou se o contato é um grupo.
//
// Se o contato não tem atendimento aberto, abre um (sem responsável, na fila geral), para a resposta do cliente cair numa fila.
func (s *Service) SendAutomated(ctx context.Context, in AutomatedSend) (Message, error) {
	text := strings.TrimSpace(in.Text)
	if !httpx.ValidUUID(in.ID) || !httpx.ValidUUID(in.ContactID) {
		return nil, ErrNotFound
	}
	kind := "text"
	if in.Att != nil {
		kind = "document"
	}
	limit := maxTextRunes
	if kind == "document" {
		limit = maxCaptionRunes
	}
	if text == "" || utf8.RuneCountInString(text) > limit {
		return nil, httpx.FieldErrors{"text": fmt.Sprintf("O texto deve ter de 1 a %d caracteres.", limit)}
	}
	if s.sender == nil || (kind == "document" && s.store == nil) {
		return nil, ErrUnavailable
	}
	if !s.sender.Connected() {
		return nil, ErrUnavailable
	}

	payload := map[string]any{"automated": true, "aiProvider": in.Provider, "aiApp": in.App}
	if in.SendID != "" {
		payload["automationSendId"] = in.SendID
	}
	var attKey string
	attID := uuid.NewString()
	var storedNew bool
	if kind == "document" {
		payload["mimeType"], payload["sizeBytes"], payload["fileName"] = in.Att.MimeType, len(in.Att.Data), in.Att.FileName
		attKey = "attachments/" + attID
		if err := s.store.Put(ctx, attKey, in.Att.Data); err != nil {
			return nil, err
		}
		storedNew = true
		defer func() {
			if storedNew {
				_ = s.store.Delete(context.WithoutCancel(ctx), attKey)
			}
		}()
	}

	var convID, chatJID, externalID string
	var createdConv bool
	var before userSet
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var waID, contactKind string
		if err := tx.QueryRow(ctx, `SELECT wa_id, kind FROM contacts WHERE id = $1`, in.ContactID).Scan(&waID, &contactKind); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if contactKind == "group" {
			return &ErrAutomatedBlocked{Reason: "grupos não recebem mensagens automáticas"}
		}
		var snoozed *time.Time
		err := tx.QueryRow(ctx, `
			SELECT id, snoozed_until FROM conversations WHERE contact_id = $1 AND status <> 'closed' FOR UPDATE`, in.ContactID).Scan(&convID, &snoozed)
		switch {
		case err == nil:
			if snoozed != nil {
				return &ErrAutomatedBlocked{Reason: "a conversa está adiada"}
			}
		case errors.Is(err, pgx.ErrNoRows):
			if err := tx.QueryRow(ctx, `
				INSERT INTO conversations (contact_id, status) VALUES ($1, 'new')
				ON CONFLICT (contact_id) WHERE status <> 'closed' DO NOTHING RETURNING id`, in.ContactID).Scan(&convID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) { // corrida: alguém abriu primeiro; o envio tenta de novo na próxima volta
					return &ErrAutomatedBlocked{Reason: "a conversa mudou; tentando de novo"}
				}
				return err
			}
			createdConv = true
		default:
			return err
		}
		if !createdConv {
			var err error
			if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `
			SELECT coalesce((SELECT m.chat_jid FROM messages m WHERE m.conversation_id = $1
			                  ORDER BY m.sent_at DESC, m.created_at DESC LIMIT 1), '')`, convID).Scan(&chatJID); err != nil {
			return err
		}
		if chatJID == "" {
			chatJID = jidOf(waID)
		}
		if chatJID == "" {
			return ErrNotFound
		}
		externalID = s.sender.NewMessageID()
		pj, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO messages (id, conversation_id, chat_jid, external_id, direction, kind, body, payload, sender_name, sender_user_id, status, sent_at)
			VALUES ($1, $2, $3, $4, 'out', $5, $6, $7, $8, NULL, 'pending', now())`,
			in.ID, convID, chatJID, externalID, kind, text, pj, in.Author); err != nil {
			return err
		}
		if storedNew {
			sum := sha256.Sum256(in.Att.Data)
			if _, err := tx.Exec(ctx, `
				INSERT INTO attachments (id, message_id, storage_key, mime_type, size_bytes, sha256, file_name, scan_status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				attID, in.ID, attKey, in.Att.MimeType, len(in.Att.Data), hex.EncodeToString(sum[:]), in.Att.FileName, in.Att.ScanStatus); err != nil {
				return err
			}
		}
		// Só a hora da última mensagem: não mexe em não lidas, responsável nem status.
		_, err = tx.Exec(ctx, `UPDATE conversations SET last_message_at = now() WHERE id = $1`, convID)
		return err
	})
	if err != nil {
		return nil, err
	}
	storedNew = false // a mensagem existe: o arquivo é dela

	bg := context.WithoutCancel(ctx)
	published := make(chan struct{})
	go func() {
		defer close(published)
		after := s.publishState(bg, convID, before)
		s.publishMessage(bg, EventMessageCreated, in.ID, after)
	}()

	sendCtx, cancel := context.WithTimeout(bg, 45*time.Second)
	defer cancel()
	wire := signedText(in.Author, text)
	sendErr := s.dispatch(sendCtx, kind, chatJID, externalID, wire, payload, attKey, in.Att)
	if sendErr != nil {
		_, _ = s.db.Exec(sendCtx, `UPDATE messages SET status = 'failed', error = $2 WHERE id = $1 AND status = 'pending'`, in.ID, "Não foi possível enviar pelo WhatsApp.")
	} else {
		_, _ = s.db.Exec(sendCtx, `UPDATE messages SET status = 'sent' WHERE id = $1 AND status = 'pending'`, in.ID)
	}
	go func() {
		<-published
		after := s.publishState(bg, convID, nil)
		s.publishMessage(bg, EventMessageUpdated, in.ID, after)
	}()
	m, err := s.message(sendCtx, in.ID)
	if err != nil {
		return nil, err
	}
	if sendErr != nil {
		return m, fmt.Errorf("conversations: falha ao enviar: %w", sendErr)
	}
	return m, nil
}

// IsOptOut: a mensagem do cliente pede para parar de receber avisos automáticos ("PARAR", "parar", "Stop", "não quero mais receber"...).
// Só mensagens curtas e inequívocas: "cancelar" sozinho pode ser o cancelamento do serviço e fica de fora.
func IsOptOut(body string) bool {
	t := strings.TrimSpace(strings.ToLower(body))
	if utf8.RuneCountInString(t) > 40 {
		return false
	}
	t = strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'â', 'ã':
			return 'a'
		case 'é', 'ê':
			return 'e'
		case 'í':
			return 'i'
		case 'ó', 'ô', 'õ':
			return 'o'
		case 'ú':
			return 'u'
		case 'ç':
			return 'c'
		case '.', '!', ',', '?', '"', '\'':
			return -1
		}
		return r
	}, t)
	t = strings.Join(strings.Fields(t), " ")
	switch t {
	case "parar", "pare", "stop", "sair", "descadastrar", "parar avisos", "parar mensagens", "nao quero receber", "nao quero mais receber", "nao quero mais mensagens":
		return true
	}
	return false
}

// optOut marca o contato como "não perturbe" para envios automáticos (idempotente) e registra na auditoria.
func (s *Service) optOut(ctx context.Context, waID string) {
	_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `UPDATE contacts SET automation_opt_out_at = now() WHERE wa_id = $1 AND automation_opt_out_at IS NULL RETURNING id::text`, waID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contact.automation_opt_out", TargetType: "contact", TargetID: id, Metadata: map[string]any{"by": "client"}})
	})
}

// CanSend: o WhatsApp do escritório está conectado agora (a fila de envios automáticos espera quando não está).
func (s *Service) CanSend() bool { return s.sender != nil && s.sender.Connected() }
