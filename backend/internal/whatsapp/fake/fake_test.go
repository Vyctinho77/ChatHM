package fake

import "testing"

func TestMessageIDsSurviveProviderRestart(t *testing.T) {
	first := New(nil).NewMessageID()
	second := New(nil).NewMessageID()
	if first == "" || second == "" || first == second {
		t.Fatalf("provider restart reused message ID: %q / %q", first, second)
	}
}
