package httpapi

// Acesso sem terminal: configuração inicial, convite, login só com o autenticador e recuperação.

import (
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/auth"
)

func totp(t *testing.T, secret string, offset time.Duration) string {
	t.Helper()
	c, err := auth.GenerateTOTP(secret, time.Now().Add(offset))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// connectSetupWA faz o pareamento público da instalação inicial (QR) com o provedor simulado.
func connectSetupWA(t *testing.T, e *env, c *client) {
	t.Helper()
	r := c.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{})
	expect(t, r, 200)
	key := str(r.body, "key")
	eventually(t, "QR da instalação", func() bool {
		st := c.do("POST", "/api/v1/setup/whatsapp/status", map[string]any{"key": key})
		return st.status == 200 && st.body["qr"] != nil
	})
	e.wa.CompletePairing()
	eventually(t, "WhatsApp conectado", func() bool { return e.svcs.WhatsApp.Connected() })
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func TestFirstAdminSetupWithoutTerminal(t *testing.T) {
	e := setup(t)
	c := e.client()
	if r := c.do("GET", "/api/v1/setup/status", nil); r.body["needsSetup"] != true {
		t.Fatalf("status: %v", r.body)
	}
	// Sem o WhatsApp do escritório conectado não há administrador.
	expect(t, c.do("POST", "/api/v1/setup/start", map[string]any{"name": "Dona", "email": "dona@x.com"}), 409, "whatsapp_required")
	// O QR só aparece para quem iniciou o pareamento.
	expect(t, c.do("POST", "/api/v1/setup/whatsapp/status", map[string]any{"key": "qualquer"}), 403)
	connectSetupWA(t, e, c)
	// Dados inválidos, código errado e token adulterado.
	expect(t, c.do("POST", "/api/v1/setup/start", map[string]any{"name": "", "email": "x"}), 422)
	r := c.do("POST", "/api/v1/setup/start", map[string]any{"name": "Dona do Escritório", "email": "dona@x.com"})
	expect(t, r, 200)
	secret, token := str(r.body, "secret"), str(r.body, "token")
	expect(t, c.do("POST", "/api/v1/setup/finish", map[string]any{"token": token, "code": "000000"}), 422, "invalid_code")
	expect(t, c.do("POST", "/api/v1/setup/finish", map[string]any{"token": token + "x", "code": totp(t, secret, 0)}), 401, "invalid_invite")

	r = c.do("POST", "/api/v1/setup/finish", map[string]any{"token": token, "code": totp(t, secret, 0)})
	expect(t, r, 201)
	if r.body["user"].(map[string]any)["role"] != "admin" || len(r.body["recoveryCodes"].([]any)) != 8 || r.body["mfaRequired"] != false {
		t.Fatalf("setup: %v", r.body)
	}
	// Já logada como admin e a tela de setup some para sempre.
	expect(t, c.do("GET", "/api/v1/users", nil), 200)
	if r := e.client().do("GET", "/api/v1/setup/status", nil); r.body["needsSetup"] != false {
		t.Fatalf("status depois: %v", r.body)
	}
	expect(t, e.client().do("POST", "/api/v1/setup/start", map[string]any{"name": "Intruso", "email": "i@x.com"}), 409, "setup_done")
	// Com administrador, o backend recusa o pareamento pela tela de entrada (mesmo com o WhatsApp desconectado).
	expect(t, e.client().do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{}), 409, "setup_done")
	expect(t, e.client().do("POST", "/api/v1/setup/whatsapp/status", map[string]any{"key": "x"}), 409, "setup_done")
	// O mesmo token não cria um segundo admin.
	expect(t, e.client().do("POST", "/api/v1/setup/finish", map[string]any{"token": token, "code": totp(t, secret, 30*time.Second)}), 409, "setup_done")
}

func TestSetupCodeProtectsFirstAccess(t *testing.T) {
	e := setup(t, opts{setupToken: "instalar-123"})
	c := e.client()
	if r := c.do("GET", "/api/v1/setup/status", nil); r.body["setupCode"] != true {
		t.Fatalf("status: %v", r.body)
	}
	expect(t, c.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": "errado"}), 403, "setup_forbidden")
	expect(t, c.do("POST", "/api/v1/setup/start", map[string]any{"name": "A", "email": "a@x.com", "installCode": "errado"}), 403, "setup_forbidden")
	connectSetupWA2(t, e, c, "instalar-123")
	expect(t, c.do("POST", "/api/v1/setup/start", map[string]any{"name": "A", "email": "a@x.com", "installCode": "instalar-123"}), 200)
}

// invite: setup do admin + convite de uma pessoa; devolve (admin, código, segredo da pessoa após enroll).
func TestInviteEnrollLoginAndRecovery(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", "admin")
	admin := e.client().mustLogin("adm@x.com")
	team := admin.do("POST", "/api/v1/teams", map[string]string{"name": "Folha"})
	expect(t, team, 201)

	// Só admin convida; papel e equipes são validados.
	expect(t, admin.do("POST", "/api/v1/users/invite", map[string]any{"email": "x", "name": "", "role": "root"}), 422)
	r := admin.do("POST", "/api/v1/users/invite", map[string]any{"email": "bia@x.com", "name": "Bia", "role": "agent", "teamIds": []string{str(team.body, "id")}})
	expect(t, r, 201)
	invite := str(r.body, "inviteCode")
	if len(invite) != 11 || r.body["user"].(map[string]any)["isActive"] != false {
		t.Fatalf("convite: %v", r.body)
	}
	expect(t, e.client().do("POST", "/api/v1/users/invite", map[string]any{"email": "z@x.com", "name": "Z", "role": "agent"}), 401)

	// Antes de aceitar: não entra por senha nem por código.
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": "123456"}), 401)

	// Código errado de convite; certo (aceita minúsculas e sem hífen).
	p := e.client()
	loose := "  " + lower(invite[:5]) + invite[6:] + " "
	r = p.do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "BIA@x.com", "inviteCode": loose})
	expect(t, r, 200)
	secret, token := str(r.body, "secret"), str(r.body, "token")
	fin := func(code string) resp {
		return p.do("POST", "/api/v1/auth/enroll/finish", map[string]any{"email": "bia@x.com", "inviteCode": invite, "token": token, "code": code})
	}
	expect(t, fin("000000"), 422, "invalid_code")
	r = fin(totp(t, secret, 0))
	expect(t, r, 201)
	codes := r.body["recoveryCodes"].([]any)
	if len(codes) != 8 || r.body["user"].(map[string]any)["mfaEnabled"] != true {
		t.Fatalf("enroll: %v", r.body)
	}
	expect(t, p.do("GET", "/api/v1/conversations", nil), 200) // já entrou, ativa, com a equipe
	// Convite é de uso único.
	expect(t, p.do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "bia@x.com", "inviteCode": invite}), 401, "invalid_invite")

	// Login só com e-mail + código. O mesmo código não vale duas vezes; o próximo passo vale.
	l := e.client()
	expect(t, l.do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": totp(t, secret, 0)}), 401, "invalid_credentials") // já usado no enroll
	ok := l.do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": totp(t, secret, 30*time.Second)})
	expect(t, ok, 200)
	if ok.body["mfaRequired"] != false {
		t.Fatalf("login por código não pede segundo fator: %v", ok.body)
	}
	expect(t, l.do("GET", "/api/v1/conversations", nil), 200)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": totp(t, secret, 30*time.Second)}), 401) // replay

	// Código de recuperação: entra uma vez; depois não vale mais.
	rc := codes[0].(string)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": rc}), 200)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": rc}), 401)

	// Gera novos códigos: os antigos deixam de valer.
	nr := l.do("POST", "/api/v1/auth/recovery-codes", nil)
	expect(t, nr, 200)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": codes[1].(string)}), 401)
	if len(nr.body["recoveryCodes"].([]any)) != 8 {
		t.Fatal("8 códigos novos")
	}

	// Celular perdido: o admin redefine; sessões e segredo antigos caem, e novo convite refaz o acesso.
	id := str(ok.body["user"].(map[string]any), "id")
	re := admin.do("POST", "/api/v1/users/"+id+"/invite", nil)
	expect(t, re, 200)
	expect(t, l.do("GET", "/api/v1/conversations", nil), 401)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "bia@x.com", "code": totp(t, secret, 60*time.Second)}), 401)
	expect(t, e.client().do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "bia@x.com", "inviteCode": invite}), 401, "invalid_invite") // convite antigo
	expect(t, e.client().do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "bia@x.com", "inviteCode": str(re.body, "inviteCode")}), 200)
}

func TestInviteBruteForceVoidsInvite(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", "admin")
	admin := e.client().mustLogin("adm@x.com")
	r := admin.do("POST", "/api/v1/users/invite", map[string]any{"email": "bia@x.com", "name": "Bia", "role": "agent"})
	expect(t, r, 201)
	p := e.client()
	var last resp
	for range 12 {
		last = p.do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "bia@x.com", "inviteCode": "ZZZZZ-ZZZZZ"})
	}
	if last.status != 401 && last.status != 429 {
		t.Fatalf("tentativas em excesso: %d", last.status)
	}
	// Mesmo o código certo não vale depois do bloqueio.
	if got := e.client().do("POST", "/api/v1/auth/enroll/start", map[string]any{"email": "bia@x.com", "inviteCode": str(r.body, "inviteCode")}); got.status == 200 {
		t.Fatal("convite deveria estar invalidado/bloqueado após tentativas")
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func connectSetupWA2(t *testing.T, e *env, c *client, install string) {
	t.Helper()
	r := c.do("POST", "/api/v1/setup/whatsapp/connect", map[string]any{"installCode": install})
	expect(t, r, 200)
	e.wa.CompletePairing()
	eventually(t, "WhatsApp conectado", func() bool { return e.svcs.WhatsApp.Connected() })
}

// Equipe entra com o token do escritório: cria a conta (ativa, sem equipe, com autenticador) sem QR e sem terminal.
func TestTeamJoinsWithOfficeToken(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", "admin")
	admin := e.client().mustLogin("adm@x.com")
	acc := admin.do("GET", "/api/v1/office/access", nil)
	expect(t, acc, 200)
	tok := str(acc.body, "token")
	if len(tok) != 14 {
		t.Fatalf("token: %q", tok)
	}
	// Só administrador vê/gira o token.
	e.createUser("ana@x.com", "agent")
	ana := e.client().mustLogin("ana@x.com")
	expect(t, ana.do("GET", "/api/v1/office/access", nil), 403)
	expect(t, ana.do("POST", "/api/v1/office/access/rotate", nil), 403)
	expect(t, e.client().do("GET", "/api/v1/office/access", nil), 401)

	join := func(c *client, token, email string) resp {
		return c.do("POST", "/api/v1/auth/join/start", map[string]any{"name": "Caio Novo", "email": email, "officeToken": token})
	}
	expect(t, join(e.client(), "AAAA-BBBB-CCCC", "caio@x.com"), 401, "invalid_token")
	expect(t, join(e.client(), tok, "invalido"), 422)
	expect(t, join(e.client(), tok, "ana@x.com"), 409, "email_unavailable")

	p := e.client()
	r := join(p, lower(tok), "CAIO@x.com") // aceita minúsculas
	expect(t, r, 200)
	secret, enr := str(r.body, "secret"), str(r.body, "token")
	fin := func(code string) resp {
		return p.do("POST", "/api/v1/auth/join/finish", map[string]any{"officeToken": tok, "token": enr, "code": code})
	}
	expect(t, fin("000000"), 422, "invalid_code")
	r = fin(totp(t, secret, 0))
	expect(t, r, 201)
	if len(r.body["recoveryCodes"].([]any)) != 8 || r.body["user"].(map[string]any)["role"] != "agent" {
		t.Fatalf("join: %v", r.body)
	}
	expect(t, p.do("GET", "/api/v1/conversations", nil), 200) // ativa na hora (aprovação automática)
	expect(t, p.do("GET", "/api/v1/users", nil), 403)
	// Sem equipe: não vê filas de equipes (só a fila geral, sem responsável).
	if teams := p.do("GET", "/api/v1/teams", nil).body["teams"].([]any); len(teams) != 0 {
		t.Fatalf("sem equipe: %v", teams)
	}
	// Depois do cadastro o token não serve para entrar: a credencial é o autenticador.
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "caio@x.com", "code": tok}), 401)
	expect(t, e.client().do("POST", "/api/v1/auth/login", map[string]any{"email": "caio@x.com", "code": totp(t, secret, 30*time.Second)}), 200)
	// Mesmo e-mail de novo e token antigo depois de girar.
	expect(t, join(e.client(), tok, "caio@x.com"), 409, "email_unavailable")
	rot := admin.do("POST", "/api/v1/office/access/rotate", nil)
	expect(t, rot, 200)
	if str(rot.body, "token") == tok {
		t.Fatal("o token deveria mudar")
	}
	expect(t, join(e.client(), tok, "dani@x.com"), 401, "invalid_token")
	expect(t, join(e.client(), str(rot.body, "token"), "dani@x.com"), 200)
}

func TestOfficeTokenDomainRestriction(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", "admin")
	admin := e.client().mustLogin("adm@x.com")
	tok := str(admin.do("GET", "/api/v1/office/access", nil).body, "token")
	expect(t, admin.do("PATCH", "/api/v1/office/access", map[string]any{"allowedDomain": "não é domínio"}), 422)
	r := admin.do("PATCH", "/api/v1/office/access", map[string]any{"allowedDomain": "@Escritorio.com.br"})
	expect(t, r, 200)
	if r.body["allowedDomain"] != "escritorio.com.br" {
		t.Fatalf("domínio: %v", r.body)
	}
	body := func(email string) map[string]any {
		return map[string]any{"name": "Pessoa", "email": email, "officeToken": tok}
	}
	expect(t, e.client().do("POST", "/api/v1/auth/join/start", body("p@gmail.com")), 422)
	expect(t, e.client().do("POST", "/api/v1/auth/join/start", body("p@outro-escritorio.com.br")), 422)
	expect(t, e.client().do("POST", "/api/v1/auth/join/start", body("p@escritorio.com.br")), 200)
	expect(t, admin.do("PATCH", "/api/v1/office/access", map[string]any{"allowedDomain": ""}), 200)
	expect(t, e.client().do("POST", "/api/v1/auth/join/start", body("p@gmail.com")), 200)
}

func TestOfficeTokenBruteForceIsThrottled(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", "admin")
	c := e.client()
	var last resp
	for range 12 {
		last = c.do("POST", "/api/v1/auth/join/start", map[string]any{"name": "X", "email": "x@x.com", "officeToken": "ZZZZ-ZZZZ-ZZZZ"})
	}
	if last.status != 429 {
		t.Fatalf("esperava 429 após várias falhas: %d", last.status)
	}
}
