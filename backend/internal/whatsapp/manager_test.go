package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/platform/testdb"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
)

type scannedProvider struct {
	Provider
	events chan PairEvent
}

type connectionPublisher struct{ connected chan struct{} }

func (p connectionPublisher) Publish(typ string, data any, _ realtime.Filter) {
	if st, ok := data.(Status); typ == EventStatus && ok && st.State == StateConnected {
		p.connected <- struct{}{}
	}
}

func TestConnectionPublishedBeforeDatabaseWrite(t *testing.T) {
	pool := testdb.New(t)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `SELECT id FROM whatsapp_connections WHERE id=1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	pub := connectionPublisher{connected: make(chan struct{}, 1)}
	m := NewManager(pool, pub, nil, func(Handler) Provider { return &scannedProvider{} }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.status = Status{State: StatePendingQR}
	m.qr = &QR{Code: "temporary", ExpiresAt: time.Now().Add(time.Minute)}
	done := make(chan struct{})
	go func() { m.OnConnected(Account{Phone: "+5592999999999"}); close(done) }()
	select {
	case <-pub.connected:
	case <-time.After(time.Second):
		t.Fatal("connection event blocked by database write")
	}
	if m.CurrentQR() != nil || m.Status().State != StateConnected {
		t.Fatal("QR not cleared immediately")
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("connection did not finish saving")
	}
	if m.setIfPending(Status{State: StatePendingQR}, &QR{Code: "late"}) {
		t.Fatal("late QR overwrote connected state")
	}
}

func (p *scannedProvider) Pair(context.Context) (<-chan PairEvent, error) { return p.events, nil }
func (p *scannedProvider) Close()                                         {}

func TestScannedQRDoesNotInventConnectionFailure(t *testing.T) {
	pool := testdb.New(t)
	p := &scannedProvider{events: make(chan PairEvent, 1)}
	m := NewManager(pool, nil, nil, func(Handler) Provider { return p }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.pairTimeout = 100 * time.Millisecond
	if err := m.Connect(t.Context(), "", ""); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	p.events <- PairEvent{Success: true}
	close(p.events)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := m.Status()
		if st.State == StateConnected {
			t.Fatal("scan incorrectly treated as a live connection")
		}
		if st.State == StatePendingQR && st.LastError != "" {
			time.Sleep(200 * time.Millisecond)
			if m.Status().State != StatePendingQR || !m.Status().PairingComplete {
				t.Fatal("local QR timer incorrectly invalidated paired session")
			}
			if m.CurrentQR() != nil {
				t.Fatal("stale QR after scan")
			}
			m.OnAuthenticated(Account{Phone: "+5592999999999"})
			if m.Status().State != StateConnected || !m.Status().Preparing || m.Connected() {
				t.Fatal("authentication should remove QR while messaging prepares")
			}
			m.OnConnected(Account{Phone: "+5592999999999"})
			if !m.Connected() || m.Status().Preparing {
				t.Fatal("connection never became ready")
			}
			m.OnAuthenticated(Account{})
			if !m.Connected() {
				t.Fatal("late authentication notification downgraded ready connection")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan confirmation did not arrive")
}
