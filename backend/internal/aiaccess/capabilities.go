package aiaccess

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// capability é o que um token precisa ter para a ferramenta funcionar. A LISTA de ferramentas é a mesma para qualquer token
// (para o agente não perder ferramentas entre um token e outro); sem a capacidade, a chamada é recusada com o motivo e o que fazer.
type capability string

const (
	capRead             capability = "read"
	capWrite            capability = "write"
	capSend             capability = "send"
	capAttend           capability = "attend"
	capAutomate         capability = "automate"
	capAutomateOrAttend capability = "automate_or_attend" // biblioteca de PDFs e diretrizes: servem aos dois modos
)

// toolCapabilities: a capacidade de cada ferramenta. Toda ferramenta registrada precisa estar aqui (um teste confere), para que
// nenhuma nasça sem regra de acesso. Ferramentas de leitura e o controle do próprio agente valem para qualquer token.
var toolCapabilities = map[string]capability{
	// leitura
	"list_conversations": capRead, "get_conversation": capRead, "read_messages": capRead, "search": capRead, "list_contacts": capRead,
	"get_contact": capRead, "list_queues": capRead, "queue_overview": capRead, "list_tags": capRead, "list_agenda": capRead, "list_people": capRead,
	"my_access": capRead, "list_pending_replies": capRead,
	// controle do próprio agente (o que ele já tratou); não altera nada do cliente nem da conversa
	"mark_replies": capRead,
	// alterações
	"add_tag": capWrite, "remove_tag": capWrite, "append_note": capWrite, "rename_contact": capWrite, "snooze": capWrite, "resume": capWrite,
	"transfer": capWrite, "set_reminder": capWrite, "create_agenda_event": capWrite, "draft_reply": capWrite,
	// pedido de envio (a pessoa aprova)
	"send_message": capSend,
	// atendente autônomo
	"send_as_attendant": capAttend,
	// envios automáticos e cobranças
	"send_automatic_message": capAutomate, "cancel_automatic_send": capAutomate, "list_automatic_sends": capAutomate, "automation_status": capAutomate,
	"list_charges": capAutomate, "list_documents": capAutomateOrAttend, "get_send_guidelines": capAutomateOrAttend,
}

// ToolCapabilities devolve (nome → capacidade) para os testes.
func ToolCapabilities() map[string]string {
	out := make(map[string]string, len(toolCapabilities))
	for k, v := range toolCapabilities {
		out[k] = string(v)
	}
	return out
}

func (p Principal) can(c capability) bool {
	switch c {
	case capRead, "":
		return true
	case capWrite:
		return p.Write
	case capSend:
		return p.Send
	case capAttend:
		return p.Attend
	case capAutomate:
		return p.Automate
	case capAutomateOrAttend:
		return p.Automate || p.Attend
	}
	return false
}

// Access descreve uma capacidade do token: se funciona e, se não, por quê e como liberar.
type Access struct {
	Capability  string   `json:"capability"`
	Label       string   `json:"label"`
	Available   bool     `json:"available"`
	Why         string   `json:"why,omitempty"`
	HowToEnable string   `json:"howToEnable,omitempty"`
	Tools       []string `json:"tools"`
}

var capLabels = map[capability]string{
	capRead:     "Leitura (conversas, contatos, filas, agenda)",
	capWrite:    "Alterações (etiquetas, observações, adiar, lembretes, agenda, transferir, rascunho)",
	capSend:     "Pedir envio de mensagem (a pessoa aprova no ChatHM)",
	capAttend:   "Atendente autônomo (enviar e responder na hora)",
	capAutomate: "Envios automáticos (DAS, cobrança, prazos), cobranças e biblioteca de PDFs",
}

// accessFor monta o estado de cada capacidade para este token, com o motivo e o caminho para liberar quando falta algo.
func (m *MCP) accessFor(ctx context.Context, p Principal) ([]Access, error) {
	st, err := m.svc.Settings(ctx)
	if err != nil {
		return nil, err
	}
	has := func(scope string) bool { return contains(p.Scopes, scope) }
	toolsOf := func(c capability) []string {
		var out []string
		for name, cc := range toolCapabilities {
			if cc == c || (c == capAutomate && cc == capAutomateOrAttend) {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	mk := func(c capability, ok bool, why, how string) Access {
		return Access{Capability: string(c), Label: capLabels[c], Available: ok, Why: why, HowToEnable: how, Tools: toolsOf(c)}
	}
	personalOnly := func(c capability) (string, string) {
		if p.OAuth {
			return "conexões por login (ChatGPT, claude.ai) nunca recebem este escopo", "use um token pessoal criado em MCP → Tokens, com o escopo " + string(c)
		}
		return "", ""
	}
	out := []Access{mk(capRead, true, "", "")}
	switch {
	case p.Write:
		out = append(out, mk(capWrite, true, "", ""))
	case !has(ScopeWrite):
		out = append(out, mk(capWrite, false, "este token não tem o escopo de escrita (write)", "crie um token com o escopo write em MCP → Tokens"))
	default:
		out = append(out, mk(capWrite, false, "o administrador desligou \"Permitir alterações\"", "ele liga em MCP → Permissões"))
	}
	switch {
	case p.Send:
		out = append(out, mk(capSend, true, "", ""))
	case !has(ScopeSend):
		out = append(out, mk(capSend, false, "este token não tem o escopo send", "crie um token com o escopo send em MCP → Tokens"))
	default:
		out = append(out, mk(capSend, false, "o administrador desligou \"Permitir pedidos de envio\"", "ele liga em MCP → Permissões"))
	}
	switch {
	case p.Attend:
		out = append(out, mk(capAttend, true, "", ""))
	case p.OAuth:
		why, how := personalOnly(capAttend)
		out = append(out, mk(capAttend, false, why, how))
	case !has(ScopeAttend):
		out = append(out, mk(capAttend, false, "este token não tem o escopo attend", "crie um token pessoal com o escopo attend em MCP → Tokens"))
	case !st.AllowAttend || !st.AllowWrite || !st.AllowSend:
		out = append(out, mk(capAttend, false, "o administrador não liberou o atendente autônomo (precisa de alterações, pedidos de envio e atendente autônomo)", "ele liga em MCP → Permissões"))
	default:
		out = append(out, mk(capAttend, false, "indisponível neste momento", ""))
	}
	switch {
	case p.Automate:
		out = append(out, mk(capAutomate, true, "", ""))
	case p.OAuth:
		why, how := personalOnly(capAutomate)
		out = append(out, mk(capAutomate, false, why, how))
	case !has(ScopeAutomate):
		out = append(out, mk(capAutomate, false, "este token não tem o escopo automate", "crie um token pessoal com o escopo automate em MCP → Tokens"))
	default:
		out = append(out, mk(capAutomate, false, "o administrador desligou \"Permitir envios automáticos\"", "ele liga em MCP → Permissões"))
	}
	return out, nil
}

// gate confere se o token pode usar a ferramenta. Falha FECHADO: uma ferramenta sem regra no mapa é recusada (nunca liberada por esquecimento).
func (m *MCP) gate(ctx context.Context, p Principal, name string) error {
	c, known := toolCapabilities[name]
	if !known {
		m.svc.Record(ctx, p, name, map[string]any{"denied": true, "noRule": true}, false)
		return errors.New("a ferramenta " + name + " está sem regra de acesso e foi bloqueada; avise o administrador do ChatHM")
	}
	if !p.can(c) {
		m.svc.Record(ctx, p, name, map[string]any{"denied": true}, false)
		return m.denial(ctx, p, name, c)
	}
	return nil
}

// denial é a resposta a uma ferramenta que este token não pode usar: diz o que falta e como liberar, em vez de a ferramenta sumir.
func (m *MCP) denial(ctx context.Context, p Principal, tool string, c capability) error {
	access, err := m.accessFor(ctx, p)
	if err != nil {
		return errors.New("esta ferramenta não está liberada para este token")
	}
	want := c
	if c == capAutomateOrAttend {
		want = capAutomate
	}
	var a Access
	for _, x := range access {
		if capability(x.Capability) == want {
			a = x
		}
	}
	// Para a biblioteca e as diretrizes, qualquer um dos dois modos basta: se o atendente existe, não precisa do automate.
	msg := "A ferramenta " + tool + " existe, mas este token não pode usá-la: " + a.Why + "."
	if a.HowToEnable != "" {
		msg += " Para liberar: " + a.HowToEnable + "."
	}
	if c == capAutomateOrAttend {
		msg += " Serve ao escopo automate ou ao attend."
	}
	msg += " Use my_access para ver tudo o que este token pode fazer."
	return errors.New(strings.TrimSpace(msg))
}
