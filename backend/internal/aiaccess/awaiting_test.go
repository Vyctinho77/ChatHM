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

func TestSortAwaitingOldestFirst(t *testing.T) {
	mk := func(id, dir string, ago time.Duration) conversations.Conversation {
		return conversations.Conversation{ID: id, LastMessage: map[string]any{"direction": dir, "sentAt": time.Now().Add(-ago).UTC().Format(time.RFC3339Nano)}}
	}
	// Misturado de propósito: espera curta, conversa sem espera no meio, espera longa, e outras.
	all := []conversations.Conversation{
		mk("novo", "in", 5*time.Minute),
		mk("nossa1", "out", 2*time.Hour),
		mk("antigo", "in", 3*time.Hour),
		mk("nossa2", "out", time.Minute),
		mk("meio", "in", 90*time.Minute),
		mk("nossa3", "out", 10*time.Hour),
		mk("maisantigo", "in", 26*time.Hour),
	}
	sortAwaiting(all)
	var got []string
	for _, c := range all {
		if awaitingSince(c) != nil {
			got = append(got, c.ID)
		}
	}
	want := []string{"maisantigo", "antigo", "meio", "novo"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ordem %v, esperava %v", got, want)
		}
	}
	// Quem não espera nada vai depois de quem espera.
	for i := 0; i < len(want); i++ {
		if awaitingSince(all[i]) == nil {
			t.Fatalf("conversa sem espera apareceu antes de quem espera: %v", all[i].ID)
		}
	}
}
