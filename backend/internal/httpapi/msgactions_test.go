package httpapi

// Responder (citar), editar, apagar (para mim / para todos) e encaminhar mensagens.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func msgList(t *testing.T, c *client, convID string) []map[string]any {
	t.Helper()
	r := c.do("GET", "/api/v1/conversations/"+convID+"/messages", nil)
	expect(t, r, 200)
	var out []map[string]any
	for _, x := range r.body["messages"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestReplyEditDeleteForward(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Cliente Ações", "Preciso da certidão negativa")
	conv := mustConv(t, w.ana, "Cliente Ações")
	cid := conv["id"].(string)
	in := msgList(t, w.ana, cid)[0]

	// --- Responder: a resposta guarda a prévia da mensagem citada e o WhatsApp recebe a citação.
	rid := uuid.NewString()
	r := w.ana.do("POST", "/api/v1/conversations/"+cid+"/messages", map[string]any{"id": rid, "text": "Claro, envio hoje.", "replyTo": in["id"]})
	expect(t, r, 201)
	q, _ := r.body["quoted"].(map[string]any)
	if q == nil || q["text"] != "Preciso da certidão negativa" || q["fromMe"] != false || q["id"] != in["id"] {
		t.Fatalf("citação = %v", r.body["quoted"])
	}
	if _, leaked := q["senderJid"]; leaked {
		t.Fatal("senderJid não deve sair na API")
	}
	sent := w.e.wa.Sent()
	last := sent[len(sent)-1]
	if last.Quote == nil || last.Quote.ExternalID == "" || last.Text != "Claro, envio hoje." {
		t.Fatalf("o WhatsApp não recebeu a citação: %+v", last)
	}
	// Mensagem de outra conversa/contato ou inexistente não pode ser citada.
	other := w.receive("Outro Cliente", "oi")
	oc := mustConv(t, w.ana, "Outro Cliente")
	_ = other
	oin := msgList(t, w.ana, oc["id"].(string))[0]
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+cid+"/messages", map[string]any{"id": uuid.NewString(), "text": "x", "replyTo": oin["id"]}), 422)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+cid+"/messages", map[string]any{"id": uuid.NewString(), "text": "x", "replyTo": "nao-e-uuid"}), 422)

	// --- Editar: só o próprio texto enviado, e o WhatsApp recebe a edição.
	expect(t, w.ana.do("PATCH", "/api/v1/messages/"+rid, map[string]any{"text": "Claro, envio hoje à tarde."}), 200)
	var edit *whatsapp.InboundMessage
	_ = edit
	sent = w.e.wa.Sent()
	if e := sent[len(sent)-1]; e.EditOf == "" || e.Text != "Claro, envio hoje à tarde." {
		t.Fatalf("edição não chegou ao WhatsApp: %+v", e)
	}
	got := msgList(t, w.ana, cid)
	var mine map[string]any
	for _, m := range got {
		if m["id"] == rid {
			mine = m
		}
	}
	if mine["text"] != "Claro, envio hoje à tarde." || mine["editedAt"] == nil {
		t.Fatalf("mensagem editada = %v", mine)
	}
	expect(t, w.ana.do("PATCH", "/api/v1/messages/"+in["id"].(string), map[string]any{"text": "x"}), 422) // recebida não edita
	expect(t, w.ana.do("PATCH", "/api/v1/messages/"+rid, map[string]any{"text": "  "}), 422)
	expect(t, w.bia.do("PATCH", "/api/v1/messages/"+rid, map[string]any{"text": "x"}), 404) // sem acesso à conversa
	// Passou de 20 minutos: o WhatsApp não aceita mais.
	_, _ = w.e.pool.Exec(t.Context(), `UPDATE messages SET sent_at = now() - interval '21 minutes' WHERE id = $1`, rid)
	expect(t, w.ana.do("PATCH", "/api/v1/messages/"+rid, map[string]any{"text": "tarde demais"}), 422)

	// --- Apagar para mim: some só para quem apagou.
	expect(t, w.ana.do("DELETE", "/api/v1/messages/"+in["id"].(string)+"?scope=me", nil), 204)
	for _, m := range msgList(t, w.ana, cid) {
		if m["id"] == in["id"] {
			t.Fatal("mensagem apagada para mim continua aparecendo")
		}
	}
	if w.admin != nil {
		seen := false
		for _, m := range msgList(t, w.admin, cid) {
			seen = seen || m["id"] == in["id"]
		}
		if !seen {
			t.Fatal("apagar para mim não deve esconder das outras pessoas")
		}
	}
	expect(t, w.ana.do("DELETE", "/api/v1/messages/"+rid+"?scope=outro", nil), 422)

	// --- Apagar para todos: só mensagem do escritório; o conteúdo some e o WhatsApp recebe o pedido.
	fresh := uuid.NewString()
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+cid+"/messages", map[string]any{"id": fresh, "text": "Mensagem errada"}), 201)
	expect(t, w.ana.do("DELETE", "/api/v1/messages/"+in["id"].(string)+"?scope=everyone", nil), 422) // do cliente
	expect(t, w.ana.do("DELETE", "/api/v1/messages/"+fresh+"?scope=everyone", nil), 204)
	sent = w.e.wa.Sent()
	if e := sent[len(sent)-1]; e.RevokeOf == "" {
		t.Fatalf("apagar não chegou ao WhatsApp: %+v", e)
	}
	for _, m := range msgList(t, w.ana, cid) {
		if m["id"] == fresh && (m["deleted"] != true || m["text"] != "") {
			t.Fatalf("mensagem apagada = %v", m)
		}
	}
	expect(t, w.ana.do("PATCH", "/api/v1/messages/"+fresh, map[string]any{"text": "x"}), 422) // apagada não edita

	// --- Encaminhar: reenvia o conteúdo para outras conversas (regras normais de envio).
	before := len(w.e.wa.Sent())
	r = w.ana.do("POST", "/api/v1/messages/"+rid+"/forward", map[string]any{"conversationIds": []string{oc["id"].(string)}})
	expect(t, r, 200)
	res := r.body["results"].([]any)[0].(map[string]any)
	if res["ok"] != true {
		t.Fatalf("encaminhamento = %v", res)
	}
	if s := w.e.wa.Sent(); len(s) != before+1 || s[len(s)-1].Text != "Claro, envio hoje à tarde." {
		t.Fatalf("encaminhada não foi enviada: %+v", s[len(s)-1])
	}
	expect(t, w.ana.do("POST", "/api/v1/messages/"+rid+"/forward", map[string]any{"conversationIds": []string{}}), 422)
	r = w.ana.do("POST", "/api/v1/messages/"+rid+"/forward", map[string]any{"conversationIds": []string{uuid.NewString()}})
	expect(t, r, 200)
	if r.body["results"].([]any)[0].(map[string]any)["ok"] == true {
		t.Fatal("conversa inexistente não pode receber encaminhamento")
	}
	_ = phone
}

func TestIncomingEditRevokeAndQuote(t *testing.T) {
	w := setupWorld(t)
	phone := "5592955550001"
	jid := phone + "@s.whatsapp.net"
	base := func(id string) whatsapp.InboundMessage {
		return whatsapp.InboundMessage{ExternalID: id, ChatJID: jid, ContactWAID: phone, ContactPhone: "+" + phone,
			PushName: "Cliente Eventos", Timestamp: time.Now(), Kind: whatsapp.KindText}
	}
	m1 := base("IN-1")
	m1.Body = "Preciso de uma segunda via"
	w.e.wa.Receive(m1)
	cid := mustConv(t, w.ana, "Cliente Eventos")["id"].(string)

	// Edição vinda do WhatsApp altera o texto e marca como editada.
	ed := base("IN-1-EDIT")
	ed.EditOf, ed.Body = "IN-1", "Preciso de uma segunda via do DAS"
	w.e.wa.Receive(ed)
	ms := msgList(t, w.ana, cid)
	if len(ms) != 1 || ms[0]["text"] != "Preciso de uma segunda via do DAS" || ms[0]["editedAt"] == nil {
		t.Fatalf("edição recebida = %v", ms)
	}

	// Resposta do cliente a uma mensagem nossa (citação) guarda a prévia da original.
	expect(t, w.send(w.ana, cid, "Qual o CNPJ?"), 201)
	var ourExt string
	_ = w.e.pool.QueryRow(t.Context(), `SELECT external_id FROM messages WHERE direction = 'out' AND conversation_id = $1`, cid).Scan(&ourExt)
	re := base("IN-2")
	re.Body = "12.345.678/0001-95"
	re.Quote = &whatsapp.InboundQuote{ExternalID: ourExt, FromMe: true}
	w.e.wa.Receive(re)
	ms = msgList(t, w.ana, cid)
	q, _ := ms[len(ms)-1]["quoted"].(map[string]any)
	if q == nil || q["text"] != "Qual o CNPJ?" || q["fromMe"] != true {
		t.Fatalf("citação recebida = %v", ms[len(ms)-1])
	}

	// "Apagar para todos" no WhatsApp: o conteúdo some, a marca fica.
	rv := base("IN-1-REVOKE")
	rv.RevokeOf = "IN-1"
	w.e.wa.Receive(rv)
	ms = msgList(t, w.ana, cid)
	if ms[0]["deleted"] != true || ms[0]["text"] != "" {
		t.Fatalf("mensagem apagada = %v", ms[0])
	}
	// Edição/apagar de mensagem desconhecida é ignorado, sem criar nada.
	unk := base("IN-X")
	unk.RevokeOf = "NAO-EXISTE"
	w.e.wa.Receive(unk)
	if got := len(msgList(t, w.ana, cid)); got != 3 {
		t.Fatalf("mensagens = %d, queria 3", got)
	}
}
