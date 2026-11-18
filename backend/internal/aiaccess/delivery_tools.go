package aiaccess

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vyctinho77/chathm/backend/internal/automation"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// deliveryLabel: o andamento de uma mensagem enviada, em palavras. "entregue" e "lida" significam que chegou: não reenvie.
var deliveryLabel = map[string]string{
	"pending":   "na fila de envio: ainda não chegou ao WhatsApp",
	"sent":      "enviada ao WhatsApp, ainda sem confirmação de entrega no celular do cliente",
	"delivered": "entregue no celular do cliente, ainda não lida",
	"read":      "lida pelo cliente",
	"failed":    "falhou: não chegou ao cliente",
}

// registerDelivery: perguntas sobre um envio, antes (check_send) e depois (message_status), para não tentar nem reenviar à toa.
func (m *MCP) registerDelivery(s *mcp.Server, p Principal, v conversations.Viewer) {
	type statusIn struct {
		MessageID string `json:"message_id" jsonschema:"id da mensagem enviada (devolvido por send_as_attendant, ou o id de uma mensagem de read_messages)"`
	}
	addTool(m, s, p, "message_status", "Diz se uma mensagem que o escritório enviou chegou ao WhatsApp do cliente e se ele já leu: status, horários de entrega, leitura e reprodução (áudio) e se o cliente escreveu depois. Use antes de reenviar: se está entregue ou lida, não reenvie.",
		func(ctx context.Context, in statusIn) (any, error) {
			if !httpx.ValidUUID(in.MessageID) {
				return nil, errors.New("message_id inválido")
			}
			var convID string
			if err := m.svc.db.QueryRow(ctx, `SELECT conversation_id::text FROM messages WHERE id = $1`, in.MessageID).Scan(&convID); err != nil {
				return nil, errors.New("mensagem não encontrada (ou você não tem acesso a ela)")
			}
			msgs, err := m.convs.Messages(ctx, v, convID, 5000)
			if err != nil {
				return nil, errors.New("mensagem não encontrada (ou você não tem acesso a ela)")
			}
			idx := -1
			for i, x := range msgs {
				if str(x["id"]) == in.MessageID {
					idx = i
					break
				}
			}
			if idx < 0 {
				return nil, errors.New("mensagem não encontrada (ou você não tem acesso a ela)")
			}
			msg := msgs[idx]
			if str(msg["direction"]) != "out" {
				return nil, errors.New("esta mensagem é do cliente: só mensagens enviadas pelo escritório têm entrega e leitura")
			}
			if msg["deleted"] == true {
				return map[string]any{"messageId": in.MessageID, "status": "deleted", "andamento": "apagada para todos"}, nil
			}
			status := str(msg["status"])
			if status == "" {
				status = "sent"
			}
			out := map[string]any{"messageId": in.MessageID, "conversationId": convID, "status": status, "andamento": deliveryLabel[status],
				"sentAt": str(msg["sentAt"]), "arrived": status == "delivered" || status == "read",
				"read": status == "read"}
			for _, k := range []string{"deliveredAt", "readAt", "playedAt"} {
				if t := str(msg[k]); t != "" {
					out[k] = t
				}
			}
			sentAt, _ := time.Parse(time.RFC3339Nano, str(msg["sentAt"]))
			answered := false
			for _, later := range msgs[idx+1:] {
				if str(later["direction"]) == "in" {
					answered = true
					break
				}
			}
			out["customerWroteAfter"] = answered
			if status == "sent" && !sentAt.IsZero() && time.Since(sentAt) > 10*time.Minute {
				out["aviso"] = "Enviada há mais de 10 minutos e sem confirmação de entrega: o celular do cliente pode estar desligado ou sem internet. Reenviar duplicaria a mensagem se ela chegar depois."
			}
			return out, nil
		})

	if m.auto == nil {
		return
	}
	type checkIn struct {
		ConversationID string     `json:"conversation_id,omitempty" jsonschema:"id da conversa do cliente. Use este OU contact_id"`
		ContactID      string     `json:"contact_id,omitempty" jsonschema:"id do contato (list_contacts ou search)"`
		Mode           string     `json:"mode,omitempty" jsonschema:"como pretende enviar: attendant (send_as_attendant, padrão) ou automatic (send_automatic_message; exige category)"`
		Category       string     `json:"category,omitempty" jsonschema:"assunto do envio automático: das, cobranca, prazos ou aviso"`
		Text           string     `json:"text,omitempty" jsonschema:"o texto que pretende enviar (opcional): token preso a uma raia também confere as regras do conteúdo"`
		Sources        []sourceIn `json:"sources,omitempty" jsonschema:"as fontes que pretende citar (opcional)"`
	}
	addTool(m, s, p, "check_send", "Responde, sem enviar nada, se um envio PASSARIA agora: wouldSend, os bloqueios (com o mesmo código das recusas reais: cliente pediu PARAR, grupo, pausa, limite de velocidade ou do dia, conversa recente com o cliente, em atendimento, adiada, categoria desligada), avisos (fora do horário: espera na fila até quando) e há quanto tempo o cliente escreveu. A resposta vale para este instante; o envio real confere tudo de novo.",
		func(ctx context.Context, in checkIn) (any, error) {
			contactID := in.ContactID
			switch {
			case in.ConversationID != "":
				c, err := m.convs.Get(ctx, v, in.ConversationID)
				if err != nil {
					return nil, errors.New("conversa não encontrada (ou você não tem acesso a ela)")
				}
				contactID = c.Contact.ID
			case contactID != "":
				all, err := m.convs.ListContacts(ctx, v)
				if err != nil {
					return nil, err
				}
				found := false
				for _, c := range all {
					if c.ID == contactID {
						found = true
						break
					}
				}
				if !found {
					return nil, errors.New("contato não encontrado (ou você não tem acesso a ele)")
				}
			default:
				return nil, errors.New("informe conversation_id ou contact_id")
			}
			if p.Lane != "" {
				return m.auto.LaneCheck(ctx, automation.Actor{UserID: p.UserID, TokenID: p.TokenID, Provider: p.Provider, App: p.App, Agent: p.Agent, Lane: p.Lane},
					automation.AttendInput{ContactID: contactID, ConversationID: in.ConversationID, Text: in.Text, Sources: toSources(in.Sources)})
			}
			mode := in.Mode
			if mode == "" {
				mode = "attendant"
			}
			if mode == "automatic" && in.Category == "" {
				return nil, errors.New("para mode automatic informe a category (das, cobranca, prazos ou aviso)")
			}
			res, err := m.auto.Preflight(ctx, automation.PreflightInput{ContactID: contactID, Mode: mode, Category: in.Category})
			var ref *automation.Refusal
			if errors.As(err, &ref) {
				return nil, errors.New(ref.Message)
			}
			if err != nil {
				return nil, errors.New("não foi possível checar agora; tente de novo")
			}
			return res, nil
		})
}
