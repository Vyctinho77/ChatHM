package httpapi

// Renomear contatos: nome da equipe vence o do WhatsApp; apagar o nome volta a seguir o WhatsApp.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestRenameContact(t *testing.T) {
	w := setupWorld(t)

	// Pessoa
	phone := w.receive("Joao do Zap", "oi")
	c := mustConv(t, w.ana, "Joao do Zap")
	cid := c["contact"].(map[string]any)["id"].(string)
	expect(t, w.ana.do("PATCH", "/api/v1/contacts/"+cid, map[string]any{"name": "  João Pereira Ltda  "}), 204)
	if conv(t, w.ana, "João Pereira Ltda") == nil {
		t.Fatal("nome novo (sem espaços nas pontas) não apareceu")
	}
	w.receiveFrom(phone, "Joao do Zap", "de novo") // o nome do WhatsApp não desfaz a edição
	if conv(t, w.ana, "João Pereira Ltda") == nil || conv(t, w.ana, "Joao do Zap") != nil {
		t.Fatal("mensagem nova sobrescreveu o nome editado")
	}
	expect(t, w.ana.do("PATCH", "/api/v1/contacts/"+cid, map[string]any{"name": string(make([]rune, 121))}), 422)

	// Grupo: o assunto acompanha o WhatsApp até a equipe definir um nome.
	gid := "120363000000000077@g.us"
	msg := func(subject string) whatsapp.InboundMessage {
		return whatsapp.InboundMessage{
			ExternalID: "G-" + uuid.NewString(), ChatJID: gid, ContactWAID: gid, IsGroup: true,
			GroupName: subject, SenderName: "Marta", Timestamp: time.Now(), Kind: whatsapp.KindText, Body: "oi",
		}
	}
	w.e.wa.Receive(msg("Assunto 1"))
	g := mustConv(t, w.ana, "Assunto 1")
	gcid := g["contact"].(map[string]any)["id"].(string)
	w.e.wa.Receive(msg("Assunto 2"))
	if conv(t, w.ana, "Assunto 2") == nil {
		t.Fatal("grupo sem edição deveria seguir o assunto")
	}
	expect(t, w.ana.do("PATCH", "/api/v1/contacts/"+gcid, map[string]any{"name": "Síndico (nosso nome)"}), 204)
	w.e.wa.Receive(msg("Assunto 3"))
	if conv(t, w.ana, "Síndico (nosso nome)") == nil {
		t.Fatal("assunto do WhatsApp sobrescreveu o nome do grupo definido pela equipe")
	}
	expect(t, w.ana.do("PATCH", "/api/v1/contacts/"+gcid, map[string]any{"name": ""}), 204)
	w.e.wa.Receive(msg("Assunto 4"))
	if conv(t, w.ana, "Assunto 4") == nil {
		t.Fatal("nome apagado deveria voltar a seguir o assunto")
	}
	// Sem sessão não renomeia.
	expect(t, w.e.client().do("PATCH", "/api/v1/contacts/"+gcid, map[string]any{"name": "x"}), 401)
}

func TestAdminRenamesImportedContactWithoutConversation(t *testing.T) {
	w := setupWorld(t)
	var id string
	if err := w.e.pool.QueryRow(t.Context(), `INSERT INTO contacts (wa_id, phone, name) VALUES ('5592977770000', '+5592977770000', 'Importado') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// Quem não é admin e não vê conversa do contato não renomeia; o admin renomeia.
	expect(t, w.caio.do("PATCH", "/api/v1/contacts/"+id, map[string]any{"name": "x"}), 404)
	expect(t, w.admin.do("PATCH", "/api/v1/contacts/"+id, map[string]any{"name": "Renomeado"}), 204)
	var name string
	_ = w.e.pool.QueryRow(t.Context(), `SELECT name FROM contacts WHERE id = $1`, id).Scan(&name)
	if name != "Renomeado" {
		t.Fatalf("nome = %q", name)
	}
}
