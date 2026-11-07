package httpapi

import (
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestContactSavedIdentity(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Nome público", "folha de pagamento")
	c := mustConv(t, w.ana, "Nome público")
	contact := c["contact"].(map[string]any)
	if contact["saved"] != false {
		t.Fatalf("push name marked as saved: %v", contact)
	}
	id := contact["id"].(string)
	expect(t, w.ana.do("PATCH", "/api/v1/contacts/"+id, map[string]string{"name": "Nome da equipe"}), 204)
	contact = mustConv(t, w.ana, "Nome da equipe")["contact"].(map[string]any)
	if contact["saved"] != true {
		t.Fatal("staff name not saved")
	}
	if err := w.e.svcs.Conversations.ImportContact(t.Context(), whatsapp.HistoryContact{WAID: phone, Phone: "+" + phone, Name: "Nome na agenda", Saved: true}); err != nil {
		t.Fatal(err)
	}
	contact = mustConv(t, w.ana, "Nome da equipe")["contact"].(map[string]any)
	if contact["saved"] != true {
		t.Fatal("saved contact lost identity")
	}
	// A sincronização não substitui o nome definido pela equipe.
	other := w.receive("Outro nome público", "folha de pagamento")
	if err := w.e.svcs.Conversations.ImportContact(t.Context(), whatsapp.HistoryContact{WAID: other, Phone: "+" + other, Name: "Nome salvo no celular", Saved: true}); err != nil {
		t.Fatal(err)
	}
	contact = mustConv(t, w.ana, "Nome salvo no celular")["contact"].(map[string]any)
	if contact["saved"] != true {
		t.Fatal("address book name not saved")
	}
	// O mesmo indicador acompanha a lista de Clientes e os resultados de busca.
	for _, path := range []string{"/api/v1/contacts", "/api/v1/search?q=Nome"} {
		r := w.admin.do("GET", path, nil)
		expect(t, r, 200)
		found := false
		for _, item := range r.body["contacts"].([]any) {
			x := item.(map[string]any)
			if x["id"] == id {
				found = true
				if x["saved"] != true {
					t.Fatalf("missing identity in %s", path)
				}
			}
		}
		if !found {
			t.Fatalf("contact not found in %s", path)
		}
	}
}
