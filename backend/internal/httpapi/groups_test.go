package httpapi

// Grupos do WhatsApp como conversas/contatos do tipo "group" e fotos de perfil.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestGroupConversationsAndAvatars(t *testing.T) {
	w := setupWorld(t)
	gid := "120363000000000001@g.us"
	msg := func(sender, text string) whatsapp.InboundMessage {
		return whatsapp.InboundMessage{
			ExternalID: "G-" + uuid.NewString(), ChatJID: gid, ContactWAID: gid, IsGroup: true,
			GroupName: "Condomínio Bella Vista", SenderName: sender, Timestamp: time.Now(),
			Kind: whatsapp.KindText, Body: text,
		}
	}
	w.e.wa.Receive(msg("Marta", "Alguém consegue a certidão negativa?"))
	w.e.wa.Receive(msg("João", "Eu envio amanhã"))

	// Uma única conversa para o grupo, marcada como grupo e com o assunto como nome.
	c := conv(t, w.ana, "Condomínio Bella Vista")
	if c == nil {
		t.Fatalf("grupo não aparece nas conversas: %v", list(t, w.ana))
	}
	contact := c["contact"].(map[string]any)
	if contact["kind"] != "group" {
		t.Fatalf("kind = %v", contact["kind"])
	}
	n := 0
	for _, x := range list(t, w.ana) {
		if x["contact"].(map[string]any)["name"] == "Condomínio Bella Vista" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("conversas do grupo = %d, queria 1", n)
	}

	// Cada mensagem guarda quem escreveu.
	r := w.ana.do("GET", "/api/v1/conversations/"+c["id"].(string)+"/messages", nil)
	expect(t, r, 200)
	ms := r.body["messages"].([]any)
	if len(ms) != 2 || ms[0].(map[string]any)["authorName"] != "Marta" || ms[1].(map[string]any)["authorName"] != "João" {
		t.Fatalf("autores: %v", ms)
	}

	// A resposta vai para o JID do grupo.
	expect(t, w.send(w.ana, c["id"].(string), "Enviaremos hoje."), 201)
	sent := w.e.wa.Sent()
	if len(sent) == 0 || sent[len(sent)-1].ChatJID != gid {
		t.Fatalf("resposta não foi para o grupo: %+v", sent)
	}

	// O assunto acompanha renomeações.
	renamed := msg("Marta", "novo nome")
	renamed.GroupName = "Bella Vista — Síndico"
	w.e.wa.Receive(renamed)
	if conv(t, w.ana, "Bella Vista — Síndico") == nil {
		t.Fatal("grupo renomeado não atualizou o nome")
	}

	// Foto de perfil: sem foto não há URL; com foto, a URL aparece e o endpoint entrega o JPEG.
	id := contact["id"].(string)
	if contact["avatarUrl"] != nil && contact["avatarUrl"] != "" {
		t.Fatalf("sem foto não deve haver avatar: %v", contact["avatarUrl"])
	}
	expect(t, w.ana.do("GET", "/api/v1/contacts/"+id+"/avatar", nil), 404)
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0}
	if _, err := w.e.pool.Exec(context.Background(),
		`UPDATE contacts SET avatar = $2, avatar_version = 3 WHERE id = $1`, id, jpeg); err != nil {
		t.Fatal(err)
	}
	c = conv(t, w.ana, "Bella Vista — Síndico")
	if got := c["contact"].(map[string]any)["avatarUrl"]; got != "/api/v1/contacts/"+id+"/avatar?v=3" {
		t.Fatalf("avatar = %v", got)
	}
	rr := w.ana.do("GET", "/api/v1/contacts/"+id+"/avatar?v=3", nil)
	if rr.status != 200 {
		t.Fatalf("avatar status %d", rr.status)
	}
	expect(t, w.e.client().do("GET", "/api/v1/contacts/"+id+"/avatar", nil), 401)
}
