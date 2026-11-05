package httpapi

import (
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestHistoryImportDoesNotDuplicateOrInflateUnread(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	contact := whatsapp.HistoryContact{WAID: "5592999999999", Phone: "+5592999999999", Name: "Cliente salvo"}
	if err := e.svcs.Conversations.ImportContact(ctx, contact); err != nil {
		t.Fatal(err)
	}
	historical := inbound("old-id", contact.WAID, "", "mensagem antiga")
	historical.Historical = true
	historical.Timestamp = time.Now().Add(-24 * time.Hour)
	if created, err := e.svcs.Conversations.Ingest(ctx, historical); err != nil || !created {
		t.Fatalf("history import: %v %v", created, err)
	}
	if created, err := e.svcs.Conversations.Ingest(ctx, historical); err != nil || created {
		t.Fatalf("duplicate: %v %v", created, err)
	}
	live := inbound("live-id", contact.WAID, "", "mensagem nova")
	if _, err := e.svcs.Conversations.Ingest(ctx, live); err != nil {
		t.Fatal(err)
	}
	var unread, count int
	var name string
	if err := e.pool.QueryRow(ctx, `SELECT c.unread_count,ct.name,(SELECT count(*) FROM messages m WHERE m.conversation_id=c.id) FROM conversations c JOIN contacts ct ON ct.id=c.contact_id WHERE ct.wa_id=$1`, contact.WAID).Scan(&unread, &name, &count); err != nil {
		t.Fatal(err)
	}
	if unread != 1 || count != 2 || name != contact.Name {
		t.Fatalf("unread=%d messages=%d name=%q", unread, count, name)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE conversations SET status='closed',closed_at=now()`); err != nil {
		t.Fatal(err)
	}
	historical.ExternalID = "older-id"
	historical.Timestamp = historical.Timestamp.Add(-time.Hour)
	if _, err := e.svcs.Conversations.Ingest(ctx, historical); err != nil {
		t.Fatal(err)
	}
	var threads, open int
	if err := e.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE status<>'closed') FROM conversations`).Scan(&threads, &open); err != nil {
		t.Fatal(err)
	}
	if threads != 1 || open != 0 {
		t.Fatalf("historical message reopened a closed conversation: threads=%d open=%d", threads, open)
	}
}
