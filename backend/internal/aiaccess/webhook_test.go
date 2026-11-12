package aiaccess

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestWebhookURLValidation(t *testing.T) {
	for _, ok := range []string{"https://hooks.exemplo.com.br/chathm", "https://n8n.empresa.com:8443/webhook/abc?x=1"} {
		if _, err := validateWebhookURL(ok, false); err != nil {
			t.Errorf("%q recusado: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "ftp://x.com/a", "http://exemplo.com/a", "https://", "https://user:senha@exemplo.com/a", "https://exemplo.com/a#frag",
		"https://localhost/a", "https://127.0.0.1/a", "https://10.0.0.5/a", "https://192.168.1.10/a", "https://169.254.169.254/latest/meta-data",
		"https://[::1]/a", "https://100.64.0.1/a", "https://0.0.0.0/a", "https://" + strings.Repeat("a", 500) + ".com",
	} {
		if _, err := validateWebhookURL(bad, false); err == nil {
			t.Errorf("%q aceito", bad)
		}
	}
	// Em testes (endereços internos liberados) o http local é aceito.
	if _, err := validateWebhookURL("http://127.0.0.1:8080/x", true); err != nil {
		t.Errorf("http local recusado em modo de teste: %v", err)
	}
}

func TestBlockedIPCoversInternalRanges(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.100.100.100", "::1", "fe80::1", "0.0.0.0", "224.0.0.1", "::ffff:10.0.0.1"} {
		if !blockedIP(netip.MustParseAddr(ip)) {
			t.Errorf("%s deveria ser bloqueado", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "200.147.67.142", "2606:4700:4700::1111"} {
		if blockedIP(netip.MustParseAddr(ip)) {
			t.Errorf("%s é público e não deveria ser bloqueado", ip)
		}
	}
}

// O bloqueio vale na CONEXÃO (depois de resolver o nome): um servidor local nunca é alcançado e redirecionamentos não são seguidos.
func TestWebhookClientRefusesInternalTargetsAndRedirects(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	if _, err := newWebhookClient(false).Get(srv.URL); err == nil || !strings.Contains(err.Error(), "destino interno bloqueado") {
		t.Fatalf("deveria bloquear o destino interno: %v", err)
	}
	if hit {
		t.Fatal("o servidor interno recebeu a requisição")
	}
	// Redirecionamento: não é seguido (devolve a resposta 302 como está).
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer redir.Close()
	resp, err := newWebhookClient(true).Get(redir.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d; o redirecionamento não deveria ser seguido", resp.StatusCode)
	}
}

func TestWebhookSignature(t *testing.T) {
	secret, body, now := "whsec_teste", []byte(`{"type":"message.received"}`), time.Now()
	h := sign(secret, now.Unix(), body)
	if !VerifyWebhookSignature(secret, h, body, now, 5*time.Minute) {
		t.Fatal("assinatura válida recusada")
	}
	if VerifyWebhookSignature("outro", h, body, now, 5*time.Minute) {
		t.Error("segredo errado aceito")
	}
	if VerifyWebhookSignature(secret, h, []byte(`{"type":"adulterado"}`), now, 5*time.Minute) {
		t.Error("corpo adulterado aceito")
	}
	if VerifyWebhookSignature(secret, h, body, now.Add(time.Hour), 5*time.Minute) {
		t.Error("assinatura antiga (repetição) aceita")
	}
	for _, bad := range []string{"", "v1=abc", "t=0,v1=abc", "lixo"} {
		if VerifyWebhookSignature(secret, bad, body, now, time.Hour) {
			t.Errorf("cabeçalho %q aceito", bad)
		}
	}
}

func TestWebhookErrorsDoNotLeakTheAddress(t *testing.T) {
	for _, in := range []error{http.ErrHandlerTimeout, errString("Post \"https://segredo.com/x?token=abc\": dial tcp 1.2.3.4:443: connect: refused")} {
		if got := webhookError(in); strings.Contains(got, "segredo") || strings.Contains(got, "token=") {
			t.Errorf("erro vaza o endereço: %q", got)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
