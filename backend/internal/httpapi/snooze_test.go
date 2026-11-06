package httpapi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

func TestSnoozePermissionsAndResume(t *testing.T) {
	w := setupWorld(t)
	w.receive("Adiar teste", "folha de pagamento")
	c := mustConv(t, w.admin, "Adiar teste")
	id := c["id"].(string)
	path := "/api/v1/conversations/" + id
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	body := map[string]string{"until": until}
	// A visible teammate cannot hide an unassigned conversation or somebody else's work.
	expect(t, w.ana.do("POST", path+"/snooze", body), 403)
	expect(t, w.bia.do("POST", path+"/snooze", body), 404)
	expect(t, w.send(w.ana, id, "Vou conferir"), 201)
	ws := w.ana.websocket(t)
	eventually(t, "snooze websocket connected", func() bool { return w.e.svcs.Hub.Count() == 1 })
	expect(t, w.ana2.do("POST", path+"/snooze", body), 403)
	expect(t, w.ana.do("POST", path+"/snooze", map[string]any{}), 422)
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": "invalid"}), 400)
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": time.Now().Add(-time.Hour).Format(time.RFC3339)}), 422)
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": time.Now().AddDate(2, 0, 0).Format(time.RFC3339)}), 422)
	expect(t, w.ana.do("POST", path+"/snooze", body), 204)
	eventually(t, "snooze event", func() bool { return ws.last("conversation.updated")["snoozedUntil"] != nil })
	c = mustConv(t, w.ana, "Adiar teste")
	storedUntil, err := time.Parse(time.RFC3339, c["snoozedUntil"].(string))
	if err != nil {
		t.Fatal(err)
	}
	wantedUntil, _ := time.Parse(time.RFC3339, until)
	if !storedUntil.Equal(wantedUntil) || c["status"] != "in_progress" || c["segment"] != "folha" || c["assignee"].(map[string]any)["id"] != w.users["ana"] {
		t.Fatalf("snooze changed assignment/queue: %v", c)
	}
	expect(t, w.send(w.ana, id, "Não deve enviar enquanto adiado"), 422)
	if len(w.e.wa.Sent()) != 1 {
		t.Fatal("snoozing sent a WhatsApp message")
	}
	expect(t, w.ana2.do("POST", path+"/resume", nil), 403)
	expect(t, w.admin.do("POST", path+"/snooze", map[string]string{"until": time.Now().Add(2 * time.Hour).Format(time.RFC3339)}), 204)
	expect(t, w.ana.do("POST", path+"/resume", nil), 204)
	eventually(t, "resume event", func() bool { return ws.last("conversation.updated")["snoozedUntil"] == nil })
	if mustConv(t, w.ana, "Adiar teste")["snoozedUntil"] != nil {
		t.Fatal("not resumed")
	}
	expect(t, w.ana.do("POST", path+"/snooze", body), 204)
	expect(t, w.ana.do("POST", path+"/close", nil), 204)
	expect(t, w.admin.do("POST", path+"/snooze", body), 409)
	expect(t, w.admin.do("POST", path+"/resume", nil), 409)
	var snoozed bool
	if err := w.e.pool.QueryRow(t.Context(), `SELECT snoozed_until IS NOT NULL FROM conversations WHERE id=$1`, id).Scan(&snoozed); err != nil {
		t.Fatal(err)
	}
	if snoozed {
		t.Fatal("closed conversation retained snooze")
	}
}

func TestSnoozeIncomingMessage(t *testing.T) {
	w := setupWorld(t)
	phone := w.receive("Retorno cliente", "folha de pagamento")
	id := mustConv(t, w.admin, "Retorno cliente")["id"].(string)
	expect(t, w.send(w.ana, id, "Vou analisar"), 201)
	path := "/api/v1/conversations/" + id
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": time.Now().Add(time.Hour).Format(time.RFC3339)}), 204)
	// Imported history and our own messages must not cancel an intentional snooze.
	for _, historical := range []bool{true, false} {
		m := inbound(uuid.NewString(), phone, "Retorno cliente", "Histórico ou mensagem do escritório")
		m.Historical, m.FromMe = historical, !historical
		if _, err := w.e.svcs.Conversations.Ingest(t.Context(), m); err != nil {
			t.Fatal(err)
		}
		if mustConv(t, w.ana, "Retorno cliente")["snoozedUntil"] == nil {
			t.Fatal("non-customer message resumed conversation")
		}
	}
	m := inbound(uuid.NewString(), phone, "Retorno cliente", "Tenho uma atualização")
	if _, err := w.e.svcs.Conversations.Ingest(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	c := mustConv(t, w.ana, "Retorno cliente")
	if c["id"] != id || c["snoozedUntil"] != nil || c["assignee"].(map[string]any)["id"] != w.users["ana"] {
		t.Fatalf("bad resume: %v", c)
	}
	// Re-delivery of that same message must not undo a later snooze.
	expect(t, w.ana.do("POST", path+"/snooze", map[string]string{"until": time.Now().Add(time.Hour).Format(time.RFC3339)}), 204)
	if created, err := w.e.svcs.Conversations.Ingest(t.Context(), m); err != nil || created {
		t.Fatalf("duplicate: %v %v", created, err)
	}
	if mustConv(t, w.ana, "Retorno cliente")["snoozedUntil"] == nil {
		t.Fatal("duplicate resumed conversation")
	}
}

func TestSnoozeScheduledAndStartupRecovery(t *testing.T) {
	w := setupWorld(t)
	w.receive("Retorno agendado", "folha de pagamento")
	id := mustConv(t, w.admin, "Retorno agendado")["id"].(string)
	path := "/api/v1/conversations/" + id
	expect(t, w.admin.do("POST", path+"/snooze", map[string]string{"until": time.Now().Add(time.Hour).Format(time.RFC3339)}), 204)
	if n, err := w.e.svcs.Conversations.ResumeDue(t.Context()); err != nil || n != 0 {
		t.Fatalf("early return: %d %v", n, err)
	}
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE conversations SET snoozed_until=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// A fresh service reads the persisted deadline on startup, without any browser open.
	fresh := conversations.NewService(w.e.pool, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); fresh.RunSnoozeWorker(ctx) }()
	eventually(t, "startup snooze recovery", func() bool { return mustConv(t, w.admin, "Retorno agendado")["snoozedUntil"] == nil })
	cancel()
	<-done
	var n int
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE target_id=$1 AND action='conversation.resume'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("scheduled returns: %d", n)
	}
	// Two API workers may process the same batch safely.
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE conversations SET snoozed_until=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); count, err := fresh.ResumeDue(t.Context()); results <- count; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for count := range results {
		total += count
	}
	if total != 1 {
		t.Fatalf("duplicate worker return: %d", total)
	}
}
