package aiaccess

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vyctinho77/chathm/backend/internal/automation"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

// registerAgent: ferramentas que o agente usa sobre si mesmo e sobre o seu fluxo de trabalho (não alteram dados do cliente).
func (m *MCP) registerAgent(s *mcp.Server, p Principal, v conversations.Viewer) {
	m.registerReplies(s, p, v)
	m.registerDelivery(s, p, v)
	type none struct{}
	addTool(m, s, p, "my_access", "Mostra quem você é neste servidor e o que este token pode fazer: perfil, escopos, cada capacidade (disponível ou não, o motivo e como liberar), privacidade e limites. Chame no começo: a lista de ferramentas é a mesma para todos os tokens, e o que o token não pode usar é recusado.",
		func(ctx context.Context, _ none) (any, error) {
			access, err := m.accessFor(ctx, p)
			if err != nil {
				return nil, err
			}
			st, err := m.svc.Settings(ctx)
			if err != nil {
				return nil, err
			}
			webhookOn := false
			if wc, werr := m.svc.Webhook(ctx); werr == nil {
				webhookOn = wc.Enabled && wc.URL != ""
			}
			role := "agent"
			switch {
			case p.Admin:
				role = "admin"
			case p.Manager:
				role = "manager"
			}
			kind := "personal"
			if p.OAuth {
				kind = "oauth"
			}
			out := map[string]any{
				"serverVersion":    serverVersion,
				"toolListIsStable": true,
				"user":             map[string]any{"id": p.UserID, "name": p.Name, "role": role, "note": "Você age como esta pessoa: só vê o que ela vê."},
				"agent":            map[string]any{"name": p.Agent, "tokenKind": kind, "provider": p.Provider, "app": p.App, "client": p.ClientName, "scopes": p.Scopes},
				"capabilities":     access,
				"privacy": map[string]any{
					"alwaysHidden":  "senhas, códigos, tokens, chaves, cartão, conta bancária, PIX e boleto",
					"maskDocuments": st.MaskDocuments || p.OAuth, "maskPhones": st.MaskPhones || p.OAuth, "maskEmails": st.MaskEmails || p.OAuth,
					"strictNames": st.StrictPrivacy, "contactPhoneVisible": st.ShowContactPhone && !st.StrictPrivacy && !p.OAuth,
				},
				"limits": map[string]any{"readsPerMinute": m.svc.rateLimit(), "writesPerMinute": 20},
				"notifications": map[string]any{
					"webhookEnabled": webhookOn,
					"howItWorks":     "O ChatHM não empurra mensagens por este canal. Para ser avisado quando o cliente responde, o administrador liga o aviso por webhook em MCP → Avisos automáticos; o aviso diz quem respondeu e em qual conversa, e você lê o resto com list_pending_replies.",
				},
				"messageAuthors": map[string]any{
					"customer": "o cliente",
					"staff":    "uma pessoa do escritório; isTokenOwner = é a pessoa dona deste token (um teste dela, por exemplo)",
					"ai":       "uma IA (atendimento por IA ou envio automático); isYou = foi este mesmo agente",
				},
			}
			if p.Lane != "" && m.auto != nil {
				if l, lerr := m.auto.Lane(ctx, p.Lane); lerr == nil {
					out["lane"] = laneView(l)
				}
			}
			return out, nil
		})
}

// laneView: a raia do token, em palavras que a IA entende (as regras que o servidor aplica a cada envio).
func laneView(l automation.Lane) map[string]any {
	return map[string]any{
		"key": l.Key, "label": l.Label, "enabled": l.Enabled, "mode": l.Mode,
		"modeNote":    "shadow = o que você enviar vira rascunho e nada vai ao cliente; live = envia de verdade",
		"canInitiate": l.CanInitiate, "onlyInQueue": l.SegmentID != "",
		"limits": map[string]any{"maxTurnsPerConversation": l.MaxTurns, "maxConsecutiveWithoutReply": l.MaxConsecutive, "perContactPerDay": l.ContactDaily, "humanQuietMinutes": l.HumanQuietMinutes},
		"rules": []string{
			"Responda só a conversas da raia (conversation_id).",
			"Se um atendente está na conversa ou ela foi passada para uma pessoa, não escreva (humano_ativo, passada_para_humano).",
			"Foto ou documento do cliente, dados pessoais, pedido de atendente ou reclamação: use request_human.",
			"Não escreva senhas, códigos, CPF/CNPJ, e-mails" + map[bool]string{true: "", false: ", valores nem números longos"}[l.AllowDigits] + ".",
			"Links só destes domínios: " + strings.Join(append([]string{}, l.AllowedDomains...), ", ") + " (vazio = nenhum).",
		},
		"discloseAi":   l.DiscloseAI,
		"guidelines":   l.Guidelines,
		"howToHandOff": "request_human(conversation_id, reason, summary)",
	}
}
