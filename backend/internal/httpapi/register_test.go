package httpapi

import "testing"

func TestRegisterCreatesAgentAndSession(t *testing.T) {
	e := setup(t)
	c := e.client()
	expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "Nova Pessoa", "email": "nova@example.com", "password": password}), 201)
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 200)
	expect(t, c.do("GET", "/api/v1/users", nil), 403)
	var role string
	if err := e.pool.QueryRow(t.Context(), `SELECT role FROM users WHERE email='nova@example.com'`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "agent" {
		t.Fatalf("unexpected role: %s", role)
	}
	expect(t, e.client().do("POST", "/api/v1/auth/register", map[string]string{"name": "Duplicada", "email": "NOVA@example.com", "password": password}), 409, "email_unavailable")
}

func TestRegisterValidationAndRateLimit(t *testing.T) {
	e := setup(t)
	c := e.client()
	for range 5 {
		expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "", "email": "invalid", "password": "short"}), 422)
	}
	expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "Pessoa", "email": "pessoa@example.com", "password": password}), 429, "too_many_attempts")
}

func TestRegisterRejectsRoleAndCrossOrigin(t *testing.T) {
	e := setup(t)
	c := e.client()
	expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "Pessoa", "email": "pessoa@example.com", "password": password, "role": "admin"}), 400)
	c.origin = "https://evil.test"
	expect(t, c.do("POST", "/api/v1/auth/register", map[string]string{"name": "Pessoa", "email": "pessoa@example.com", "password": password}), 403, "origin_not_allowed")
}
