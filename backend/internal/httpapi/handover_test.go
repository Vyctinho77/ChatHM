package httpapi

// Entrega da instância a um novo cliente: apaga tudo, desconecta o WhatsApp e libera a instalação por código único.

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func (e *env) count(table string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestHandoverWipesInstanceAndRequiresOneTimeInstallCode(t *testing.T) {
	e := setup(t, opts{fresh: true, setupToken: "env-token"})
	c := e.client()

	// Instalação do primeiro administrador (como na tela): QR do WhatsApp → dados → autenticador.
	connectSetupWA2(t, e, c, "env-token")
	r := c.do("POST", "/api/v1/setup/start", map[string]any{"name": "Dono Teste", "email": "dono@x.com", "installCode": "env-token"})
	expect(t, r, 200)
	secret, tok := str(r.body, "secret"), str(r.body, "token")
	expect(t, c.do("POST", "/api/v1/setup/finish", map[string]any{"token": tok, "code": totp(t, secret, 0)}), 201)

	// Há dados reais: conversa, mensagem enviada com anexo (arquivo guardado) e equipe.
	cid := mustConv(t, c, e.receiveName("Cliente Entrega", "olá"))["id"].(string)
	up := c.upload("/api/v1/conversations/"+cid+"/messages", map[string]string{"id": uuid.NewString(), "kind": "document"}, "doc.pdf", []byte("%PDF-1.4 conteudo"))
	expect(t, up, 201)
	expect(t, c.do("POST", "/api/v1/teams", map[string]string{"name": "Folha"}), 201)
	if countFiles(t, e.storageDir) == 0 || e.count("users") != 1 || e.count("messages") == 0 {
		t.Fatal("dados de partida")
	}

	h := func(confirm, code string) resp {
		return c.do("POST", "/api/v1/office/handover", map[string]any{"confirm": confirm, "code": code})
	}
	// Precisa digitar a confirmação e o código atual do autenticador.
	expect(t, h("", totp(t, secret, 30*time.Second)), 422)
	expect(t, h("entregar", "000000"), 422, "invalid_code")
	expect(t, e.client().do("POST", "/api/v1/office/handover", map[string]any{"confirm": "ENTREGAR", "code": "123456"}), 401)
	if e.count("users") != 1 {
		t.Fatal("nada deveria ter sido apagado ainda")
	}

	// Entrega.
	ok := h("entregar", totp(t, secret, 30*time.Second))
	expect(t, ok, 200)
	code := str(ok.body, "installCode")
	if len(code) != 11 {
		t.Fatalf("código: %q", code)
	}
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 401) // sessão encerrada
	for _, tab := range []string{"users", "sessions", "contacts", "conversations", "messages", "attachments", "teams", "office_access"} {
		if n := e.count(tab); n != 0 {
			t.Fatalf("%s deveria estar vazia: %d", tab, n)
		}
	}
	if countFiles(t, e.storageDir) != 0 {
		t.Fatal("os anexos deveriam ter sido apagados do armazenamento")
	}
	if e.svcs.WhatsApp.Connected() || e.count("whatsapp_connections") != 1 {
		t.Fatal("WhatsApp deveria estar desconectado e a linha de estado recriada")
	}

	// Instalação voltou ao início e exige o código de entrega (o SETUP_TOKEN do servidor não vale mais).
	n := e.client()
	if o := n.do("GET", "/api/v1/auth/options", nil); o.body["needsSetup"] != true || o.body["setupCode"] != true {
		t.Fatalf("opções: %v", o.body)
	}
	expect(t, n.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": "env-token"}), 403, "setup_forbidden")
	expect(t, n.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": "AAAAA-AAAAA"}), 403, "setup_forbidden")
	expect(t, n.do("POST", "/api/v1/setup/start", map[string]any{"name": "Intruso", "email": "i@x.com", "installCode": "env-token"}), 403, "setup_forbidden")

	// O cliente usa o código (aceita minúsculas e sem hífen) e vira o administrador do escritório dele.
	loose := lower(code[:5]) + code[6:]
	connectSetupWA2(t, e, n, loose)
	r = n.do("POST", "/api/v1/setup/start", map[string]any{"name": "Cliente Novo", "email": "novo@cliente.com", "installCode": code})
	expect(t, r, 200)
	secret2 := str(r.body, "secret")
	expect(t, n.do("POST", "/api/v1/setup/finish", map[string]any{"token": str(r.body, "token"), "code": totp(t, secret2, 0)}), 201)
	if e.count("users") != 1 {
		t.Fatal("só o novo administrador")
	}

	// O código é de uso único: depois de usado, a instalação está concluída.
	expect(t, e.client().do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": code}), 409, "setup_done")
	if o := e.client().do("GET", "/api/v1/auth/options", nil); o.body["needsSetup"] != false {
		t.Fatalf("opções depois: %v", o.body)
	}
	var unused int
	_ = e.pool.QueryRow(t.Context(), "SELECT count(*) FROM install_codes WHERE used_at IS NULL").Scan(&unused)
	if unused != 0 {
		t.Fatal("código deveria estar consumido")
	}
	_ = whatsapp.StateConnected
}

func TestInstallCodeBruteForceIsThrottled(t *testing.T) {
	e := setup(t, opts{fresh: true, setupToken: "segredo-do-servidor"})
	c := e.client()
	var last resp
	for range 40 {
		last = c.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": "palpite"})
	}
	if last.status != 429 {
		t.Fatalf("esperava 429 depois de muitas tentativas: %d", last.status)
	}
	// Mesmo o código certo fica bloqueado durante a janela.
	if r := c.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": "segredo-do-servidor"}); r.status != 429 {
		t.Fatalf("bloqueio: %d", r.status)
	}
}

func TestHandoverNeedsAuthenticatorAndAdmin(t *testing.T) {
	e := setup(t) // admin criado direto (sem autenticador) e um atendente
	e.createUser("adm@x.com", "admin")
	e.createUser("ana@x.com", "agent")
	admin := e.client().mustLogin("adm@x.com")
	ana := e.client().mustLogin("ana@x.com")
	expect(t, ana.do("POST", "/api/v1/office/handover", map[string]any{"confirm": "ENTREGAR", "code": "123456"}), 403)
	// Sem autenticador ativo o administrador não pode entregar.
	expect(t, admin.do("POST", "/api/v1/office/handover", map[string]any{"confirm": "ENTREGAR", "code": "123456"}), 409, "mfa_not_enabled")
	if e.count("users") != 2 {
		t.Fatal("nada deveria ser apagado")
	}
}
