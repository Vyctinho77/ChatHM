package httpapi

// Busca completa e início de conversa com clientes (Fase "contato ativo").

import (
	"testing"

	"github.com/google/uuid"
)

func TestSearchContactsAndMessages(t *testing.T) {
	w := setupWorld(t)
	w.receive("João Pereira Ltda", "Preciso da folha de pagamento de OUTUBRO")
	w.receive("Maria Souza", "Quero emitir uma nota fiscal de serviço")
	idJ := mustConv(t, w.ana, "João Pereira Ltda")["id"].(string)
	expect(t, w.send(w.ana, idJ, "Claro, já envio o holerite do João."), 201)

	search := func(c *client, q string) resp { return c.do("GET", "/api/v1/search?q="+q, nil) }
	count := func(r resp, k string) int { return len(r.body[k].([]any)) }

	// Sem acento e sem diferenciar maiúsculas; acha a mensagem recebida e a enviada.
	r := search(w.ana, "outubro")
	expect(t, r, 200)
	if count(r, "messages") != 1 {
		t.Fatalf("outubro: %v", r.body)
	}
	hit := r.body["messages"].([]any)[0].(map[string]any)
	if hit["conversationId"] != idJ || hit["contactName"] != "João Pereira Ltda" || hit["direction"] != "in" {
		t.Fatalf("hit: %v", hit)
	}
	if r := search(w.ana, "holerite"); count(r, "messages") != 1 || r.body["messages"].([]any)[0].(map[string]any)["direction"] != "out" {
		t.Fatalf("holerite: %v", r.body)
	}
	// Contatos: por nome sem acento e por pedaço do telefone.
	if r := search(w.ana, "joao"); count(r, "contacts") != 1 {
		t.Fatalf("joao: %v", r.body)
	}
	phone := conv(t, w.ana, "João Pereira Ltda")["contact"].(map[string]any)["phone"].(string)
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if r := search(w.ana, digits[len(digits)-6:]); count(r, "contacts") != 1 {
		t.Fatalf("telefone: %v", r.body)
	}
	// Curto demais, curingas e entrada hostil não quebram nem vazam.
	if r := search(w.ana, "a"); count(r, "messages") != 0 || count(r, "contacts") != 0 {
		t.Fatal("busca de 1 caractere não deveria retornar nada")
	}
	if r := search(w.ana, "%25%25"); count(r, "messages") != 0 {
		t.Fatalf("curinga literal: %v", r.body)
	}
	if r := search(w.ana, "%27%3B%20DROP%20TABLE%20messages%3B%20--"); r.status != 200 {
		t.Fatalf("injeção: %d", r.status)
	}
	expect(t, w.e.client().do("GET", "/api/v1/search?q=outubro", nil), 401)
}

func TestSearchRespectsVisibility(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Folha", "dúvida sobre a folha de pagamento secreta")
	// A fila da Folha é da equipe da ana; bia (Notas) e dani (sem equipe) não enxergam, nem pela busca.
	for name, c := range map[string]*client{"bia": w.bia, "caio": w.caio} {
		r := c.do("GET", "/api/v1/search?q=secreta", nil)
		expect(t, r, 200)
		if len(r.body["messages"].([]any)) != 0 {
			t.Fatalf("%s não pode achar mensagens de outra equipe: %v", name, r.body)
		}
		if r := c.do("GET", "/api/v1/search?q=Cliente%20Folha", nil); len(r.body["contacts"].([]any)) != 0 {
			t.Fatalf("%s não pode achar o contato de outra equipe", name)
		}
	}
	if r := w.ana2.do("GET", "/api/v1/search?q=secreta", nil); len(r.body["messages"].([]any)) != 1 {
		t.Fatal("colega da equipe encontra")
	}
	if r := w.admin.do("GET", "/api/v1/search?q=secreta", nil); len(r.body["messages"].([]any)) != 1 {
		t.Fatal("admin encontra")
	}
}

func TestMessagesLimitParam(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Longo", "folha de pagamento 0")
	for i := 1; i < 12; i++ {
		w.receiveFrom(phone, "Cliente Longo", "mensagem "+itoa(i))
	}
	id := mustConv(t, w.ana, "Cliente Longo")["id"].(string)
	n := func(q string) int {
		return len(w.ana.do("GET", "/api/v1/conversations/"+id+"/messages"+q, nil).body["messages"].([]any))
	}
	if n("") != 12 || n("?limit=5") != 5 || n("?limit=99999") != 12 || n("?limit=abc") != 12 {
		t.Fatalf("limites: %d %d %d %d", n(""), n("?limit=5"), n("?limit=99999"), n("?limit=abc"))
	}
}

func TestOpenConversationWithContactAndByPhone(t *testing.T) {
	w := setupWorld(t)
	// Contato que só existe no catálogo (sem conversa): vindo de outra equipe, o admin o vê; qualquer pessoa pode abrir por telefone.
	r := w.ana.do("POST", "/api/v1/conversations/open", map[string]any{"phone": "(92) 99111-2233", "name": "Cliente Ativo"})
	expect(t, r, 200)
	id := r.body["id"].(string)
	if r.body["status"] != "in_progress" || r.body["assignee"] == nil || r.body["lastMessage"] != nil {
		t.Fatalf("conversa nova: %v", r.body)
	}
	if got := r.body["contact"].(map[string]any); got["name"] != "Cliente Ativo" || got["phone"] != "+55 92 99111-2233" {
		t.Fatalf("contato: %v", got)
	}
	// Idempotente; é privada: outra pessoa não vê e não consegue abrir o mesmo cliente.
	again := w.ana.do("POST", "/api/v1/conversations/open", map[string]any{"phone": "5592991112233"})
	expect(t, again, 200)
	if again.body["id"] != id {
		t.Fatal("deveria devolver a mesma conversa")
	}
	if conv(t, w.ana2, "Cliente Ativo") != nil {
		t.Fatal("conversa iniciada é privada")
	}
	expect(t, w.ana2.do("POST", "/api/v1/conversations/open", map[string]any{"phone": "5592991112233"}), 409, "conversation_taken")
	// Telefones inválidos.
	for _, bad := range []string{"", "123", "abc", "99999999999999999999"} {
		expect(t, w.ana.do("POST", "/api/v1/conversations/open", map[string]any{"phone": bad}), 422)
	}

	// A primeira mensagem sai para o número certo (sem histórico anterior).
	expect(t, w.send(w.ana, id, "Olá! Aqui é do escritório."), 201)
	sent := w.e.wa.Sent()
	if len(sent) != 1 || sent[0].ChatJID != "5592991112233@s.whatsapp.net" {
		t.Fatalf("destino: %+v", sent)
	}
	// A resposta do cliente cai na mesma conversa, que segue com a ana.
	w.receiveFrom("5592991112233", "Cliente Ativo", "Oi, bom dia!")
	list := w.ana.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any)
	if len(list) != 2 {
		t.Fatalf("mensagens: %d", len(list))
	}
	if a := w.assignee(t, id); a == nil {
		t.Fatal("a conversa deveria continuar com a ana")
	}

	// Pelo cadastro de contatos: um contato importado, sem conversa.
	cs := w.admin.do("GET", "/api/v1/contacts", nil).body["contacts"].([]any)
	var contactID string
	for _, c := range cs {
		if c.(map[string]any)["name"] == "Cliente Ativo" {
			contactID = c.(map[string]any)["id"].(string)
		}
	}
	if contactID == "" {
		t.Fatal("contato não listado")
	}
	expect(t, w.ana.do("POST", "/api/v1/contacts/"+contactID+"/conversation", nil), 200)
	expect(t, w.ana2.do("POST", "/api/v1/contacts/"+contactID+"/conversation", nil), 409, "conversation_taken")
	expect(t, w.ana.do("POST", "/api/v1/contacts/"+uuid.NewString()+"/conversation", nil), 404)
	expect(t, w.ana.do("POST", "/api/v1/contacts/xyz/conversation", nil), 404)
	expect(t, w.e.client().do("POST", "/api/v1/conversations/open", map[string]any{"phone": "5592991112233"}), 401)

	// Depois de encerrar, o contato pode ser aberto de novo por qualquer pessoa.
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	expect(t, w.ana2.do("POST", "/api/v1/contacts/"+contactID+"/conversation", nil), 200)
}

func TestOpenedEmptyConversationAppearsInRealtimeOnlyForOwner(t *testing.T) {
	w := setupWorld(t)
	anaWS, ana2WS := w.ana.websocket(t), w.ana2.websocket(t)
	eventually(t, "ws", func() bool { return w.e.svcs.Hub.Count() == 2 })
	expect(t, w.ana.do("POST", "/api/v1/conversations/open", map[string]any{"phone": "5592987654321", "name": "Novo"}), 200)
	eventually(t, "dona recebe", func() bool { return anaWS.count("conversation.updated") >= 1 })
	if ana2WS.count("conversation.updated") != 0 {
		t.Fatal("colega não deveria receber a conversa privada")
	}
}
