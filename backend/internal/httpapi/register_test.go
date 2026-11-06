package httpapi

import "testing"

func reg(c *client, name, email, pw string) resp {
	return c.do("POST", "/api/v1/auth/register", map[string]string{"name": name, "email": email, "password": pw})
}

func TestRegisterIsDisabledByDefault(t *testing.T) {
	e := setup(t)
	c := e.client()
	expect(t, reg(c, "Nova", "nova@example.com", password), 404, "registration_disabled")
	if r := c.do("GET", "/api/v1/auth/options", nil); r.status != 200 || r.body["selfRegister"] != false {
		t.Fatalf("options: %v", r.body)
	}
}

// Conta criada por conta própria nasce inativa: sem sessão, sem login e sem ver conversas até o admin aprovar.
func TestRegisterCreatesPendingAccountUntilAdminApproves(t *testing.T) {
	e := setup(t, opts{selfRegister: true})
	e.createUser("adm@x.com", "admin")
	admin := e.client().mustLogin("adm@x.com")
	c := e.client()
	if r := c.do("GET", "/api/v1/auth/options", nil); r.body["selfRegister"] != true {
		t.Fatalf("options: %v", r.body)
	}
	r := reg(c, "Nova Pessoa", "nova@example.com", password)
	expect(t, r, 202)
	if r.body["pending"] != true || r.body["csrfToken"] != nil {
		t.Fatalf("resposta: %v", r.body)
	}
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 401)
	expect(t, e.client().login("nova@example.com", password), 401, "invalid_credentials")
	expect(t, reg(e.client(), "Duplicada", "NOVA@example.com", password), 409, "email_unavailable")

	// O admin aprova em Equipe (ativa a conta); só então a pessoa entra, como atendente.
	var id string
	if err := e.pool.QueryRow(t.Context(), `SELECT id FROM users WHERE email='nova@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	expect(t, admin.do("PATCH", "/api/v1/users/"+id, map[string]any{"isActive": true}), 200)
	n := e.client().mustLogin("nova@example.com")
	expect(t, n.do("GET", "/api/v1/users", nil), 403)
	if got := n.do("GET", "/api/v1/auth/me", nil).body["user"].(map[string]any)["role"]; got != "agent" {
		t.Fatalf("papel: %v", got)
	}
}

func TestRegisterValidationAndRateLimit(t *testing.T) {
	e := setup(t, opts{selfRegister: true})
	c := e.client()
	for range 5 {
		expect(t, reg(c, "", "invalid", "short"), 422)
	}
	expect(t, reg(c, "Pessoa", "pessoa@example.com", password), 429, "too_many_attempts")
}

func TestRegisterRejectsRoleAndCrossOrigin(t *testing.T) {
	e := setup(t, opts{selfRegister: true})
	c := e.client()
	expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "Pessoa", "email": "pessoa@example.com", "password": password, "role": "admin"}), 400)
	c.origin = "https://evil.test"
	expect(t, reg(c, "Pessoa", "pessoa@example.com", password), 403, "origin_not_allowed")
}
