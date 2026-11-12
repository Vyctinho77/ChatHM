package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

func personalReminders(t *testing.T, c *client) []any {
	t.Helper()
	r := c.do("GET", "/api/v1/conversation-reminders", nil)
	expect(t, r, 200)
	return r.body["reminders"].([]any)
}

func TestPersonalReminderPrivacyAndPersistence(t *testing.T) {
	w := setupWorld(t)
	w.receive("Lembrete privado", "folha de pagamento")
	id := mustConv(t, w.admin, "Lembrete privado")["id"].(string)
	path := "/api/v1/conversations/" + id + "/reminder"
	text := "Aguardar documentos do cliente até sexta-feira"
	body := map[string]string{"text": text, "color": "amber"}
	expect(t, w.e.client().do("GET", "/api/v1/conversation-reminders", nil), 401)
	expect(t, w.ana.do("PUT", path, body), 404) // not assigned yet
	expect(t, w.send(w.ana, id, "Vou verificar"), 201)
	ownerWS, adminWS := w.ana.websocket(t), w.admin.websocket(t)
	eventually(t, "reminder sockets", func() bool { return w.e.svcs.Hub.Count() == 2 })
	csrf := w.ana.csrf
	w.ana.csrf = ""
	expect(t, w.ana.do("PUT", path, body), 403)
	w.ana.csrf = csrf
	expect(t, w.ana.do("PUT", path, body), 204)
	eventually(t, "personal reminder event", func() bool { return ownerWS.count("conversation.reminder.updated") > 0 })
	if adminWS.count("conversation.reminder.updated") != 0 {
		t.Fatal("private event broadcast to administrator")
	}
	if event := ownerWS.last("conversation.reminder.updated"); len(event) != 1 || event["id"] != id {
		t.Fatal("reminder event should only carry its conversation ID")
	}
	for _, other := range []*client{w.admin, w.ana2, w.bia} {
		if len(personalReminders(t, other)) != 0 {
			t.Fatal("another user read a personal note")
		}
		expect(t, other.do("PUT", path, body), 404)
		expect(t, other.do("DELETE", path, nil), 404)
	}
	for _, c := range []*client{w.admin, w.ana2, w.ana} {
		r := c.do("GET", "/api/v1/conversations/"+id, nil)
		expect(t, r, 200)
		serialized, err := json.Marshal(r.body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serialized), text) {
			t.Fatal("private text leaked into shared conversation")
		}
	}
	// A newly constructed service reads the saved text and color from PostgreSQL.
	fresh := conversations.NewService(w.e.pool, nil)
	reminders, err := fresh.ListReminders(t.Context(), conversations.Viewer{ID: w.users["ana"]})
	if err != nil || len(reminders) != 1 || reminders[0].Text != text || reminders[0].Color != "amber" {
		t.Fatalf("reminder not persisted: %v %v", reminders, err)
	}
	// The next assignee gets a separate personal note; returning restores only one's own.
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/transfer", map[string]string{"userId": w.users["ana2"]}), 204)
	if len(personalReminders(t, w.ana)) != 0 || len(personalReminders(t, w.ana2)) != 0 {
		t.Fatal("note carried over to new assignee")
	}
	expect(t, w.ana.do("PUT", path, body), 404)
	expect(t, w.ana2.do("PUT", path, map[string]string{"text": "Nota de outra pessoa", "color": "sky"}), 204)
	expect(t, w.ana2.do("POST", "/api/v1/conversations/"+id+"/transfer", map[string]string{"userId": w.users["ana"]}), 204)
	saved := personalReminders(t, w.ana)
	if len(saved) != 1 || saved[0].(map[string]any)["text"] != text {
		t.Fatal("own note overwritten by another user's note")
	}
	expect(t, w.ana.do("DELETE", path, nil), 204)
	if len(personalReminders(t, w.ana)) != 0 {
		t.Fatal("note not removed")
	}
	if len(w.e.wa.Sent()) != 1 {
		t.Fatal("reminder operation sent a WhatsApp message")
	}
}

func TestReminderValidationAndClosedConversation(t *testing.T) {
	w := setupWorld(t)
	w.receive("Validar lembrete", "folha de pagamento")
	id := mustConv(t, w.admin, "Validar lembrete")["id"].(string)
	expect(t, w.send(w.ana, id, "Vou conferir"), 201)
	path := "/api/v1/conversations/" + id + "/reminder"
	expect(t, w.ana.do("PUT", path, map[string]string{"text": strings.Repeat("a", 241), "color": "olive"}), 422)
	expect(t, w.ana.do("PUT", path, map[string]string{"text": "teste", "color": "#ffffff"}), 422)
	for _, color := range []string{"olive", "amber", "sky", "rose", "violet", "neutral"} {
		expect(t, w.ana.do("PUT", path, map[string]string{"text": "  Retornar contato  ", "color": color}), 204)
		saved := personalReminders(t, w.ana)
		if len(saved) != 1 || saved[0].(map[string]any)["text"] != "Retornar contato" || saved[0].(map[string]any)["color"] != color {
			t.Fatal("text or color not updated")
		}
	}
	expect(t, w.ana.do("PUT", path, map[string]string{"text": strings.Repeat("😊", 240), "color": "olive"}), 204)
	expect(t, w.ana.do("PUT", path, map[string]string{"text": "  ", "color": "olive"}), 204)
	if len(personalReminders(t, w.ana)) != 0 {
		t.Fatal("empty note retained")
	}
	expect(t, w.ana.do("PUT", path, map[string]string{"text": "Teste final", "color": "rose"}), 204)
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/close", nil), 204)
	if len(personalReminders(t, w.ana)) != 0 {
		t.Fatal("closed conversation reminder listed")
	}
	expect(t, w.ana.do("PUT", path, map[string]string{"text": "Não alterar", "color": "rose"}), 409)
	var rls bool
	if err := w.e.pool.QueryRow(t.Context(), `SELECT relrowsecurity FROM pg_class WHERE oid='conversation_reminders'::regclass`).Scan(&rls); err != nil {
		t.Fatal(err)
	}
	if !rls {
		t.Fatal("personal reminders table missing RLS")
	}
}

func TestTransferCanShareTheReminder(t *testing.T) {
	w := setupWorld(t)
	share := func(c *client, convID, userID string) resp {
		return c.do("POST", "/api/v1/conversations/"+convID+"/transfer", map[string]any{"userId": userID, "shareReminder": true})
	}
	handover := func(convID string) (note, color, by *string) {
		t.Helper()
		err := w.e.pool.QueryRow(t.Context(), `SELECT handover_note, handover_color, handover_by::text
			FROM conversation_assignments WHERE conversation_id = $1 AND ended_at IS NULL`, convID).Scan(&note, &color, &by)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	setup := func(name string) string {
		w.receive(name, "folha de pagamento")
		id := mustConv(t, w.admin, name)["id"].(string)
		expect(t, w.send(w.ana, id, "Vou verificar"), 201)
		expect(t, w.ana.do("PUT", "/api/v1/conversations/"+id+"/reminder", map[string]string{"text": "Cliente espera o contrato", "color": "sky"}), 204)
		return id
	}

	// Com autorização, o lembrete vai para quem recebe e fica no histórico do atendimento.
	id := setup("Repasse com lembrete")
	expect(t, share(w.ana, id, w.users["ana2"]), 204)
	got := personalReminders(t, w.ana2)
	if len(got) != 1 || got[0].(map[string]any)["text"] != "Cliente espera o contrato" || got[0].(map[string]any)["color"] != "sky" {
		t.Fatalf("reminder not shared with the new assignee: %v", got)
	}
	if note, color, by := handover(id); note == nil || *note != "Cliente espera o contrato" || color == nil || *color != "sky" || by == nil || *by != w.users["ana"] {
		t.Fatal("handover not recorded on the new assignment")
	}
	// Quem recebeu edita à vontade; o registro do repasse não muda.
	path := "/api/v1/conversations/" + id + "/reminder"
	expect(t, w.ana2.do("PUT", path, map[string]string{"text": "Contrato enviado", "color": "olive"}), 204)
	if note, _, _ := handover(id); note == nil || *note != "Cliente espera o contrato" {
		t.Fatal("handover record changed with the new owner's edit")
	}
	// O lembrete de quem transferiu continua só dele.
	if len(personalReminders(t, w.ana)) != 0 || len(personalReminders(t, w.bia)) != 0 {
		t.Fatal("reminder visible to someone else")
	}

	// Um administrador que transfere a conversa de outra pessoa não repassa o lembrete dela.
	id = setup("Repasse por administrador")
	expect(t, share(w.admin, id, w.users["ana2"]), 204)
	if len(personalReminders(t, w.ana2)) != 1 { // só o da conversa anterior
		t.Fatal("administrator transfer shared someone else's reminder")
	}
	if note, _, by := handover(id); note != nil || by != nil {
		t.Fatal("handover recorded without the owner's reminder")
	}

	// Sem lembrete salvo, pedir para compartilhar não muda nada.
	w.receive("Repasse sem lembrete", "folha de pagamento")
	id = mustConv(t, w.admin, "Repasse sem lembrete")["id"].(string)
	expect(t, w.send(w.ana, id, "Vou verificar"), 201)
	expect(t, share(w.ana, id, w.users["ana2"]), 204)
	if note, _, _ := handover(id); note != nil {
		t.Fatal("handover recorded without a reminder")
	}
}
