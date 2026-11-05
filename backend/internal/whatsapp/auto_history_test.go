package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type historyRequestProvider struct {
	Provider
	requests chan struct{}
	release  chan struct{}
}

func (p *historyRequestProvider) RequestHistory(ctx context.Context) error {
	p.requests <- struct{}{}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestAutomaticHistoryRequestsOncePerDevice(t *testing.T) {
	p := &historyRequestProvider{requests: make(chan struct{}, 10), release: make(chan struct{})}
	m := NewManager(nil, nil, nil, func(Handler) Provider { return p }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.status = Status{State: StateConnected}
	for range 10 {
		m.requestInitialHistory("device-one")
	}
	select {
	case <-p.requests:
	case <-time.After(time.Second):
		t.Fatal("automatic history was not requested")
	}
	if m.Status().HistoryRequest != "requesting" {
		t.Fatal("missing request progress")
	}
	close(p.release)
	deadline := time.Now().Add(time.Second)
	for m.Status().HistoryRequest != "requested" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.Status().HistoryRequest != "requested" {
		t.Fatal("request did not finish")
	}
	// A later connection/profile event for the same device cannot re-trigger it.
	m.requestInitialHistory("device-one")
	select {
	case <-p.requests:
		t.Fatal("same device requested duplicate history")
	default:
	}
	m.requestInitialHistory("device-two")
	select {
	case <-p.requests:
	case <-time.After(time.Second):
		t.Fatal("new pairing did not request history")
	}
}
