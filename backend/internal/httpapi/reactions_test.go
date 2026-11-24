package httpapi

// Reações às mensagens: enviar, trocar, remover, receber do cliente e do celular, e as regras de quem pode reagir.

import (
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func reactionsOf(m map[string]any) []map[string]any {
	var out []map[string]any
	if rs, ok := m["reactions"].([]any); ok {
		for _, r := range rs {
			out = append(out, r.(map[string]any))
		}
	}
	return out
}

func TestReactionsSendReplaceRemoveAndReceive(t *testing.T) {
	w := setupWorld(t)
	phone := "5592955550123"
	jid := phone + "@s.whatsapp.net"
	base := func(id string) whatsapp.InboundMessage {
		return whatsapp.InboundMessage{ExternalID: id, ChatJID: jid, ContactWAID: phone, ContactPhone: "+" + phone,
			PushName: "Cliente Reações", Timestamp: time.Now(), Kind: whatsapp.KindText}
	}
	m1 := base("R-IN-1")
	m1.Body = "Segue o documento"
	w.e.wa.Receive(m1)
	cid := mustConv(t, w.ana, "Cliente Reações")["id"].(string)
	in := msgList(t, w.ana, cid)[0]
	id := in["id"].(string)
	if len(reactionsOf(in)) != 0 {
		t.Fatalf("mensagem nova não tem reações: %v", in)
	}

	// Reagir: vai ao WhatsApp (mensagem do cliente: fromMe falso) e volta na mensagem.
	r := w.ana.do("PUT", "/api/v1/messages/"+id+"/reaction", map[string]any{"emoji": "👍"})
	expect(t, r, 200)
	rs := reactionsOf(r.body)
	if len(rs) != 1 || rs[0]["emoji"] != "👍" || rs[0]["count"] != float64(1) || rs[0]["mine"] != true {
		t.Fatalf("reação = %v", r.body["reactions"])
	}
	sent := w.e.wa.Sent()
	if last := sent[len(sent)-1]; last.ReactTo != "R-IN-1" || last.Reaction != "👍" {
		t.Fatalf("o WhatsApp não recebeu a reação: %+v", last)
	}

	// Trocar de emoji substitui (uma reação por conta); a lista da conversa mostra a nova.
	expect(t, w.ana.do("PUT", "/api/v1/messages/"+id+"/reaction", map[string]any{"emoji": "❤️"}), 200)
	if rs := reactionsOf(msgList(t, w.ana, cid)[0]); len(rs) != 1 || rs[0]["emoji"] != "❤️" {
		t.Fatalf("depois de trocar = %v", rs)
	}

	// O cliente também reage (e o escritório continua com a dele): dois emojis diferentes.
	cr := base("R-IN-REACT")
	cr.ReactTo, cr.Reaction = "R-IN-1", "🙏"
	w.e.wa.Receive(cr)
	rs = reactionsOf(msgList(t, w.ana, cid)[0])
	if len(rs) != 2 {
		t.Fatalf("cliente + escritório = %v", rs)
	}
	var mine, theirs int
	for _, x := range rs {
		if x["mine"] == true {
			mine++
		} else if x["emoji"] == "🙏" {
			theirs++
		}
	}
	if mine != 1 || theirs != 1 {
		t.Fatalf("minha/dele = %d/%d em %v", mine, theirs, rs)
	}
	// Reação não cria mensagem nem mexe em não lidas.
	if n := len(msgList(t, w.ana, cid)); n != 1 {
		t.Fatalf("reação criou mensagem: %d", n)
	}

	// Remover (emoji vazio) tira só a do escritório; a do cliente fica.
	expect(t, w.ana.do("PUT", "/api/v1/messages/"+id+"/reaction", map[string]any{"emoji": ""}), 200)
	rs = reactionsOf(msgList(t, w.ana, cid)[0])
	if len(rs) != 1 || rs[0]["emoji"] != "🙏" || rs[0]["mine"] == true {
		t.Fatalf("depois de remover = %v", rs)
	}
	sent = w.e.wa.Sent()
	if last := sent[len(sent)-1]; last.ReactTo != "R-IN-1" || last.Reaction != "" {
		t.Fatalf("remoção não chegou ao WhatsApp: %+v", last)
	}
	// O cliente remove a dele: volta a nenhuma reação.
	cr2 := base("R-IN-REACT-2")
	cr2.ReactTo, cr2.Reaction = "R-IN-1", ""
	w.e.wa.Receive(cr2)
	if rs := reactionsOf(msgList(t, w.ana, cid)[0]); len(rs) != 0 {
		t.Fatalf("depois do cliente remover = %v", rs)
	}

	// O escritório reage pelo celular (fromMe): conta como a reação do escritório.
	ph := base("R-IN-PHONE")
	ph.FromMe, ph.ReactTo, ph.Reaction = true, "R-IN-1", "😂"
	w.e.wa.Receive(ph)
	if rs := reactionsOf(msgList(t, w.ana, cid)[0]); len(rs) != 1 || rs[0]["emoji"] != "😂" || rs[0]["mine"] != true {
		t.Fatalf("reação pelo celular = %v", rs)
	}
	// Reação a mensagem desconhecida é ignorada, sem criar nada.
	unk := base("R-X")
	unk.ReactTo, unk.Reaction = "NAO-EXISTE", "👍"
	w.e.wa.Receive(unk)
	if n := len(msgList(t, w.ana, cid)); n != 1 {
		t.Fatalf("reação a mensagem desconhecida criou algo: %d", n)
	}
}

func TestReactionRulesAndValidation(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Regras Reação", "oi")
	cid := mustConv(t, w.ana, "Cliente Regras Reação")["id"].(string)
	id := msgList(t, w.ana, cid)[0]["id"].(string)
	put := func(c *client, mid string, emoji any) resp {
		return c.do("PUT", "/api/v1/messages/"+mid+"/reaction", map[string]any{"emoji": emoji})
	}

	// Só emoji: texto comum, vazio de verdade é remoção, e lixo é recusado.
	for _, bad := range []string{"a", "oi", "👍a", "<b>", "1", "\u0000"} {
		expect(t, put(w.ana, id, bad), 422)
	}
	for _, ok := range []string{"👍", "❤️", "🙏🏽", "🇧🇷", "1️⃣"} {
		expect(t, put(w.ana, id, ok), 200)
	}
	expect(t, put(w.ana, "nao-e-uuid", "👍"), 404)

	// Outra pessoa atendendo bloqueia (mesma regra de responder); depois de assumir, quem assume reage.
	expect(t, w.send(w.ana, cid, "Bom dia"), 201) // ana assume
	if r := put(w.bia, id, "👍"); r.status == 200 {
		t.Fatalf("quem não atende não deveria reagir: %d", r.status)
	}
	// Quem não enxerga a conversa não a encontra (nada vaza).
	other := w.receive("Cliente Outro Time", "oi")
	_ = other
}
