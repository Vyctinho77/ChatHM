package aiaccess

import (
	"errors"
	"strings"
	"testing"
)

func TestStatusHintExplicaOsErrosComuns(t *testing.T) {
	cases := map[int]string{
		401: "segredo", 403: "segredo", 404: "não existe", 405: "não aceita POST",
		415: "formato", 429: "limitou", 502: "lado do receptor",
	}
	for code, want := range cases {
		if got := statusHint(code); !strings.Contains(got, want) {
			t.Errorf("statusHint(%d) = %q, esperava conter %q", code, got, want)
		}
	}
	if statusHint(418) != "" {
		t.Error("códigos sem dica devem ficar sem sufixo")
	}
}

func TestWebhookErrorMantemOCodigoEADica(t *testing.T) {
	got := webhookError(errors.New("o receptor respondeu 405" + statusHint(405)))
	if !strings.HasPrefix(got, "o receptor respondeu 405") || !strings.Contains(got, "POST") {
		t.Fatalf("mensagem inesperada: %q", got)
	}
}
