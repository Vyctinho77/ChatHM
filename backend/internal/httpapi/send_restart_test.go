package httpapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/fake"
)

// Preserve transport/connection behavior while replacing the ID generator, as on restart.
type restartedIDSender struct {
	conversations.Sender
	ids *fake.Provider
}

func (s restartedIDSender) NewMessageID() string { return s.ids.NewMessageID() }

type collidingIDSender struct{ conversations.Sender }

func (s collidingIDSender) NewMessageID() string { return "forced-provider-collision" }

func TestOwnerCanSendAfterFakeProviderRestart(t *testing.T) {
	w := setupWorld(t)
	opened := w.ana.do("POST", "/api/v1/conversations/open", map[string]string{"phone": "+5592999994444", "name": "Iniciada por mim"})
	expect(t, opened, 200)
	id := opened.body["id"].(string)
	w.e.svcs.Conversations.SetSender(restartedIDSender{Sender: w.e.svcs.WhatsApp, ids: fake.New(nil)})
	first := w.send(w.ana, id, "Primeiro envio")
	expect(t, first, 201)
	// A new provider starts with no memory of the IDs already stored in the database.
	w.e.svcs.Conversations.SetSender(restartedIDSender{Sender: w.e.svcs.WhatsApp, ids: fake.New(nil)})
	body := map[string]string{"id": uuid.NewString(), "text": "Envio depois de reiniciar"}
	path := "/api/v1/conversations/" + id + "/messages"
	second := w.ana.do("POST", path, body)
	expect(t, second, 201)
	if second.body["status"] != "sent" {
		t.Fatalf("send after restart = %v", second.body)
	}
	expect(t, w.ana.do("POST", path, body), 200) // repeated request stays idempotent
	if len(w.e.wa.Sent()) != 2 {
		t.Fatal("restart or retry lost/duplicated a message")
	}
	var assignee string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT assignee_id FROM conversations WHERE id=$1`, id).Scan(&assignee); err != nil {
		t.Fatal(err)
	}
	if assignee != w.users["ana"] {
		t.Fatal("restart changed the responsible user")
	}
}

func TestProviderIDCollisionIsNotReportedAsAnotherAssignee(t *testing.T) {
	w := setupWorld(t)
	w.receive("Colisão técnica", "folha de pagamento")
	id := mustConv(t, w.ana, "Colisão técnica")["id"].(string)
	w.e.svcs.Conversations.SetSender(collidingIDSender{Sender: w.e.svcs.WhatsApp})
	expect(t, w.send(w.ana, id, "Primeiro envio"), 201)
	r := w.send(w.ana, id, "Colisão forçada")
	if r.status != 500 || r.code() == "conversation_taken" {
		t.Fatalf("technical collision impersonated an assignment conflict: %d %v", r.status, r.body)
	}
	if len(w.e.wa.Sent()) != 1 {
		t.Fatal("colliding message reached the provider")
	}
}
