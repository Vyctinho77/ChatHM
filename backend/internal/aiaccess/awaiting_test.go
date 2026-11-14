package aiaccess

import (
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/conversations"
)

func TestAwaitingFilter(t *testing.T) {
	at := time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339Nano)
	in := conversations.Conversation{LastMessage: map[string]any{"direction": "in", "sentAt": at}}
	out := conversations.Conversation{LastMessage: map[string]any{"direction": "out", "sentAt": at}}
	snoozedUntil := time.Now().Add(time.Hour)
	sn := in
	sn.SnoozedUntil = &snoozedUntil
	if !matchFilter(in, "awaiting", "u") || matchFilter(out, "awaiting", "u") || matchFilter(sn, "awaiting", "u") {
		t.Fatal("awaiting deve pegar só conversas cuja última mensagem é do cliente e que não estão adiadas")
	}
	if matchFilter(conversations.Conversation{}, "awaiting", "u") {
		t.Fatal("sem mensagem não está aguardando")
	}
}
