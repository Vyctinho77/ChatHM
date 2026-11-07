package httpapi

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestTypingVisibilityAndMessageStop(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Digitando teste", "folha de pagamento")
	c := mustConv(t, w.ana, "Digitando teste")
	id := c["id"].(string)
	owner, teammate, admin, outsider := w.ana.websocket(t), w.ana2.websocket(t), w.admin.websocket(t), w.bia.websocket(t)
	eventually(t, "typing sockets", func() bool { return w.e.svcs.Hub.Count() == 4 })
	update := whatsapp.Typing{ChatJID: phone + "@s.whatsapp.net", ContactWAID: phone, SenderID: "contact", Active: true}
	w.e.svcs.WhatsApp.OnTyping(update)
	eventually(t, "typing visible to team", func() bool {
		return owner.count("conversation.typing") == 1 && teammate.count("conversation.typing") == 1 && admin.count("conversation.typing") == 1
	})
	if outsider.count("conversation.typing") != 0 {
		t.Fatal("typing leaked to unrelated team")
	}
	event := owner.last("conversation.typing")
	expires, err := time.Parse(time.RFC3339Nano, event["expiresAt"].(string))
	if err != nil || !expires.After(time.Now()) || expires.After(time.Now().Add(11*time.Second)) || event["conversationId"] != id || event["active"] != true {
		t.Fatalf("bad typing event: %v", event)
	}
	after := mustConv(t, w.ana, "Digitando teste")
	if after["status"] != c["status"] || after["unreadCount"] != c["unreadCount"] || after["assignee"] != nil {
		t.Fatal("typing modified atendimento")
	}
	update.Active = false
	w.e.svcs.WhatsApp.OnTyping(update)
	eventually(t, "pause delivered", func() bool { return owner.last("conversation.typing")["active"] == false })
	update.Active = true
	w.e.svcs.WhatsApp.OnTyping(update)
	eventually(t, "typing again", func() bool { return owner.last("conversation.typing")["active"] == true })
	w.receiveFrom(phone, "Digitando teste", "Pronto, enviei")
	eventually(t, "message stops typing", func() bool { return owner.last("conversation.typing")["active"] == false })
	// Unknown chats do not create contacts or conversations.
	var beforeCount, afterCount int
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM contacts`).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	w.e.svcs.WhatsApp.OnTyping(whatsapp.Typing{ChatJID: "unknown@s.whatsapp.net", ContactWAID: "unknown", SenderID: "contact", Active: true})
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM contacts`).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if beforeCount != afterCount {
		t.Fatal("typing created a contact")
	}
	expect(t, w.admin.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	previousEvents := owner.count("conversation.typing")
	w.e.svcs.WhatsApp.OnTyping(update)
	if owner.count("conversation.typing") != previousEvents {
		t.Fatal("closed chat still received typing")
	}
}

func TestGroupTypingPreservesSender(t *testing.T) {
	w := setupWorld(t)
	group := "120363000000000222@g.us"
	m := whatsapp.InboundMessage{ExternalID: uuid.NewString(), ChatJID: group, ContactWAID: group, IsGroup: true, GroupName: "Grupo digitando", SenderName: "Marta", SenderID: "sender-a", Timestamp: time.Now(), Kind: whatsapp.KindText, Body: "Olá"}
	if _, err := w.e.svcs.Conversations.Ingest(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	ws := w.admin.websocket(t)
	eventually(t, "group typing socket", func() bool { return w.e.svcs.Hub.Count() == 1 })
	w.e.svcs.WhatsApp.OnTyping(whatsapp.Typing{ChatJID: group, ContactWAID: group, SenderID: "sender-a", Active: true})
	eventually(t, "group sender typing", func() bool { return ws.last("conversation.typing")["senderId"] == "sender-a" })
	m.ExternalID = uuid.NewString()
	if _, err := w.e.svcs.Conversations.Ingest(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	eventually(t, "group sender stopped after message", func() bool {
		return ws.last("conversation.typing")["senderId"] == "sender-a" && ws.last("conversation.typing")["active"] == false
	})
}
