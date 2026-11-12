package automation

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/media"
)

// Atendente autônomo: a IA age como um atendente. Sem categorias, sem aprovação, sem texto pronto, e pode responder
// a quem acabou de escrever. O que continua valendo, de propósito, e não depende da IA:
//   - chave mestra do administrador (desligada por padrão) e o "Pausar tudo";
//   - cliente que pediu PARAR e grupos não recebem;
//   - teto de velocidade, para o número do WhatsApp não ser bloqueado;
//   - tudo fica na conversa (com a marca de IA) e na auditoria.
const (
	attendPer10Min = 40
	attendPerDay   = 600
	attendMaxRunes = 1500
)

// AttendInput: ContactID já foi validado contra a visibilidade de quem pede.
type AttendInput struct {
	ContactID  string
	Text       string
	DocumentID string
}

type AttendResult struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
}

func (s *Service) attendOn(ctx context.Context) (bool, error) {
	var on bool
	err := s.db.QueryRow(ctx, `SELECT enabled AND allow_write AND allow_send AND allow_attend FROM ai_settings WHERE id`).Scan(&on)
	if err != nil {
		return false, nil
	}
	return on, nil
}

// SendAsAttendant envia agora, na hora, e devolve o resultado do envio.
func (s *Service) SendAsAttendant(ctx context.Context, a Actor, in AttendInput) (AttendResult, error) {
	on, err := s.attendOn(ctx)
	if err != nil {
		return AttendResult{}, err
	}
	if !on {
		return AttendResult{}, refuse("attend_off", "O atendente autônomo está desligado pelo administrador.")
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return AttendResult{}, err
	}
	if st.Paused {
		return AttendResult{}, refuse("paused", "Os envios da IA estão pausados pelo administrador.")
	}
	text := strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(text); n == 0 || n > attendMaxRunes {
		return AttendResult{}, refuse("text", "A mensagem deve ter de 1 a %d caracteres.", attendMaxRunes)
	}
	var optOut, group bool
	if err := s.db.QueryRow(ctx, `SELECT automation_opt_out_at IS NOT NULL, kind = 'group' FROM contacts WHERE id = $1`, in.ContactID).Scan(&optOut, &group); err != nil {
		return AttendResult{}, refuse("contact", "Contato não encontrado.")
	}
	if group {
		return AttendResult{}, refuse("group", "A IA não envia mensagens para grupos.")
	}
	if optOut {
		return AttendResult{}, refuse("opt_out", "Este cliente pediu para não receber mensagens automáticas (PARAR).")
	}
	var recent, today int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sent_at > $1::timestamptz - interval '10 minutes'), count(*) FILTER (WHERE sent_at > $1::timestamptz - interval '24 hours')
		  FROM messages WHERE payload->>'aiMode' = 'attend'`, s.now()).Scan(&recent, &today); err != nil {
		return AttendResult{}, err
	}
	if recent >= attendPer10Min || today >= attendPerDay {
		return AttendResult{}, refuse("rate", "Limite de velocidade atingido (%d mensagens em 10 minutos, %d por dia) para proteger o número do WhatsApp. Tente de novo em alguns minutos.", attendPer10Min, attendPerDay)
	}
	var att *media.Processed
	if in.DocumentID != "" {
		d, ref, derr := s.guardDocument(ctx, in.DocumentID, in.ContactID)
		if derr != nil {
			return AttendResult{}, derr
		}
		if ref != nil {
			return AttendResult{}, ref
		}
		var key string
		if err := s.db.QueryRow(ctx, `SELECT storage_key FROM automation_documents WHERE id = $1`, in.DocumentID).Scan(&key); err != nil {
			return AttendResult{}, refuse("document", "O documento não está mais na biblioteca.")
		}
		data, err := s.store.Get(ctx, key)
		if err != nil {
			return AttendResult{}, refuse("document", "Não foi possível ler o documento da biblioteca.")
		}
		p, err := s.media.ProcessInbound(ctx, media.Document, data, d.Name, "application/pdf")
		if err != nil {
			return AttendResult{}, refuse("document", "O documento foi recusado pela validação de segurança.")
		}
		att = &p
	}
	msgID := uuid.NewString()
	msg, err := s.convs.SendAutomated(ctx, conversations.AutomatedSend{
		ID: msgID, ContactID: in.ContactID, Author: Author(st.Signature), Text: text, Att: att,
		Provider: a.Provider, App: a.App, Attend: true, ActorID: a.UserID,
	})
	var blocked *conversations.ErrAutomatedBlocked
	switch {
	case errors.As(err, &blocked):
		return AttendResult{}, refuse("blocked", "Não foi possível enviar: %s.", blocked.Reason)
	case errors.Is(err, conversations.ErrUnavailable):
		return AttendResult{}, refuse("whatsapp", "O WhatsApp do escritório está desconectado.")
	case msg != nil && err != nil: // a mensagem existe, mas o WhatsApp recusou
		return AttendResult{MessageID: msgID, Status: "falhou"}, nil
	case err != nil:
		return AttendResult{}, err
	}
	s.record(ctx, "automation.attend.send", msgID, map[string]any{"contactId": in.ContactID, "provider": a.Provider, "tokenId": a.TokenID, "length": utf8.RuneCountInString(text), "hasDocument": in.DocumentID != ""})
	return AttendResult{MessageID: msgID, Status: "enviado"}, nil
}
