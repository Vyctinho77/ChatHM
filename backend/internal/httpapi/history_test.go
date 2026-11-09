package httpapi

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
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

func historicalPDF(id, phone, name string, data []byte, download func(context.Context) ([]byte, error)) whatsapp.InboundMessage {
	m := inbound(id, phone, name, "")
	m.Historical = true
	m.Timestamp = time.Now().Add(-60 * 24 * time.Hour)
	m.Kind = whatsapp.KindDocument
	m.Payload = map[string]any{"fileName": "guia.pdf", "mimeType": "application/pdf", "sizeBytes": uint64(len(data)), "historical": true}
	m.Download = download
	return m
}

func attachmentCount(t *testing.T, e *env, ext string) int {
	t.Helper()
	var n int
	_ = e.pool.QueryRow(t.Context(), `SELECT count(*) FROM attachments a JOIN messages m ON m.id = a.message_id WHERE m.external_id = $1`, ext).Scan(&n)
	return n
}

func TestHistoricalPDFIsDownloadedInBackgroundWithinBudget(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	pdf := []byte("%PDF-1.4\n%guia do mes antiga")
	e.svcs.Conversations.SetHistoryMediaBudget(1 << 20)
	phone := "5592988887777"

	// PDF do histórico entra (a mensagem aparece na hora) e o arquivo chega em segundo plano.
	m := historicalPDF("old-pdf-1", phone, "Cliente PDF", pdf, func(context.Context) ([]byte, error) { return pdf, nil })
	if created, err := e.svcs.Conversations.Ingest(ctx, m); err != nil || !created {
		t.Fatalf("ingest: %v %v", created, err)
	}
	eventually(t, "PDF do histórico baixado", func() bool { return attachmentCount(t, e, "old-pdf-1") == 1 })

	// Arquivo que o WhatsApp já apagou: a mensagem fica, marcada como falha, sem derrubar nada.
	gone := historicalPDF("old-pdf-2", phone, "Cliente PDF", pdf, func(context.Context) ([]byte, error) { return nil, errors.New("404: expirou") })
	if _, err := e.svcs.Conversations.Ingest(ctx, gone); err != nil {
		t.Fatal(err)
	}
	eventually(t, "falha marcada", func() bool {
		var st string
		_ = e.pool.QueryRow(ctx, `SELECT coalesce(payload->>'mediaState','') FROM messages WHERE external_id = 'old-pdf-2'`).Scan(&st)
		return st == "failed"
	})

	// Reentrega do histórico: o PDF que falhou é tentado de novo (agora o arquivo existe) e o que já tinha arquivo não duplica.
	again := historicalPDF("old-pdf-2", phone, "Cliente PDF", pdf, func(context.Context) ([]byte, error) { return pdf, nil })
	if created, err := e.svcs.Conversations.Ingest(ctx, again); err != nil || created {
		t.Fatalf("reentrega não deve criar mensagem: %v %v", created, err)
	}
	eventually(t, "PDF recuperado na reentrega", func() bool { return attachmentCount(t, e, "old-pdf-2") == 1 })
	same := historicalPDF("old-pdf-1", phone, "Cliente PDF", pdf, func(context.Context) ([]byte, error) { return pdf, nil })
	if _, err := e.svcs.Conversations.Ingest(ctx, same); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if attachmentCount(t, e, "old-pdf-1") != 1 {
		t.Fatal("o mesmo PDF não pode ser anexado duas vezes")
	}

	// A mensagem de histórico não conta como não lida nem reabre a conversa.
	var unread int
	_ = e.pool.QueryRow(ctx, `SELECT coalesce(max(unread_count), 0) FROM conversations`).Scan(&unread)
	if unread != 0 {
		t.Fatalf("histórico não pode gerar não lidas: %d", unread)
	}
}

func TestHistoricalPDFStopsAtSpaceBudgetAndCanBeDisabled(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	pdf := []byte("%PDF-1.4\n" + strings.Repeat("x", 4000))
	phone := "5592977776666"
	var downloads atomic.Int32
	download := func(context.Context) ([]byte, error) { downloads.Add(1); return pdf, nil }

	// Desligado (teto 0): a mensagem entra, nada é baixado.
	e.svcs.Conversations.SetHistoryMediaBudget(0)
	if _, err := e.svcs.Conversations.Ingest(ctx, historicalPDF("b-off", phone, "Cliente Teto", pdf, download)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if downloads.Load() != 0 || attachmentCount(t, e, "b-off") != 0 {
		t.Fatal("com o recurso desligado nada deve ser baixado")
	}

	// Cabe só um PDF no teto: o primeiro baixa, o segundo é marcado como pulado (continua visível, só sem arquivo).
	e.svcs.Conversations.SetHistoryMediaBudget(int64(len(pdf)) + 100)
	if _, err := e.svcs.Conversations.Ingest(ctx, historicalPDF("b-1", phone, "Cliente Teto", pdf, download)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "primeiro cabe", func() bool { return attachmentCount(t, e, "b-1") == 1 })
	if _, err := e.svcs.Conversations.Ingest(ctx, historicalPDF("b-2", phone, "Cliente Teto", pdf, download)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "segundo pulado pelo teto", func() bool {
		var st string
		_ = e.pool.QueryRow(ctx, `SELECT coalesce(payload->>'mediaState','') FROM messages WHERE external_id = 'b-2'`).Scan(&st)
		return st == "skipped"
	})
	if attachmentCount(t, e, "b-2") != 0 {
		t.Fatal("acima do teto não pode baixar")
	}
	var downloadedBeforeBudgetCheck = downloads.Load()
	if downloadedBeforeBudgetCheck != 1 {
		t.Fatalf("o PDF acima do teto nem deveria ser baixado (downloads=%d)", downloadedBeforeBudgetCheck)
	}
}

func TestHistoryRequestWindowIsValidatedAndAdminOnly(t *testing.T) {
	w := setupWorld(t)
	path := "/api/v1/whatsapp/history"
	expect(t, w.ana.do("POST", path, map[string]any{"days": 90}), 403)
	for _, bad := range []int{-1, 366, 100000} {
		expect(t, w.admin.do("POST", path, map[string]any{"days": bad}), 422)
	}
	// Janela válida (e sem corpo) chega ao provedor; o simulador dos testes não sabe pedir histórico: 409, sem erro de validação.
	for _, ok := range []map[string]any{{"days": 1}, {"days": 365}, {"days": 90}, {}} {
		expect(t, w.admin.do("POST", path, ok), 409, "whatsapp_unavailable")
	}
}
