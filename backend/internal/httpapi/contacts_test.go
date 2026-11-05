package httpapi

import (
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"testing"
)

func TestContactDirectoryUsesConversationVisibility(t *testing.T) {
	e := setup(t)
	e.createUser("admin@contacts.test", auth.RoleAdmin)
	e.createUser("agent@contacts.test", auth.RoleAgent)
	admin := e.client().mustLogin("admin@contacts.test")
	agent := e.client().mustLogin("agent@contacts.test")
	if err := e.svcs.Conversations.ImportContact(t.Context(), whatsapp.HistoryContact{WAID: "5592111111111", Name: "Sem histórico"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svcs.Conversations.Ingest(t.Context(), inbound("visible", "5592222222222", "Cliente", "Olá")); err != nil {
		t.Fatal(err)
	}
	expect(t, e.client().do("GET", "/api/v1/contacts", nil), 401)
	all := admin.do("GET", "/api/v1/contacts", nil)
	expect(t, all, 200)
	if len(all.body["contacts"].([]any)) != 2 {
		t.Fatalf("admin directory: %v", all.body)
	}
	visible := agent.do("GET", "/api/v1/contacts", nil)
	expect(t, visible, 200)
	list := visible.body["contacts"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["name"] != "Cliente" {
		t.Fatalf("agent saw hidden directory contacts: %v", visible.body)
	}
	if _, ok := list[0].(map[string]any)["conversationId"]; !ok {
		t.Fatal("missing conversation link")
	}
	expect(t, agent.do("POST", "/api/v1/whatsapp/history", nil), 403)
}
