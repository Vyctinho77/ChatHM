package httpapi

// Testes de integração ponta a ponta (HTTP → serviços → PostgreSQL).
// Requerem TEST_DATABASE_URL; cada teste usa um banco isolado.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/app"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/platform/testdb"
	"github.com/vyctinho77/chathm/backend/internal/users"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/fake"
)

const (
	origin   = "http://app.test"
	password = "senha-de-teste-forte"
)

var fastHasher = auth.Hasher{Params: auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}}

type env struct {
	t    *testing.T
	srv  *httptest.Server
	pool *pgxpool.Pool
	svcs *app.Services
	wa   *fake.Provider // provedor WhatsApp simulado
	// storageDir: pasta dos anexos (cifrados) deste ambiente de teste.
	storageDir string
}

type opts struct {
	requireAdminMFA bool
	waSession       bool // provedor já com sessão salva (simula reinício)
	selfRegister    bool // habilita POST /auth/register
	setupToken      string
	fresh           bool // instalação nova, sem segmentos de exemplo
}

func setup(t *testing.T, o ...opts) *env {
	t.Helper()
	t.Parallel()
	pool := testdb.New(t)

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	storageDir := t.TempDir()
	cfg := config.Config{
		Env:               "development",
		AllowedOrigins:    []string{origin},
		SessionCookieName: "chathm_session",
		SessionTTL:        time.Hour,
		SessionIdle:       30 * time.Minute,
		EncryptionKey:     base64.StdEncoding.EncodeToString(key),
		StorageDriver:     "fs",
		StorageDir:        storageDir,
		MaxAttachment:     5 << 20,
	}
	var op opts
	if len(o) > 0 {
		op = o[0]
	}
	cfg.RequireAdminMFA = op.requireAdminMFA
	cfg.AllowSelfRegister = op.selfRegister
	cfg.SetupToken = op.setupToken
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var wa *fake.Provider
	svcs, err := app.NewServices(context.Background(), cfg, pool, app.Options{
		Hasher: fastHasher, Logger: logger,
		WhatsAppFactory: fake.Factory(0, func(p *fake.Provider) {
			wa = p
			if op.waSession {
				_ = p.Logout(context.Background())
				p.CompletePairingSilently()
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !op.fresh {
		// Instalação nova não traz segmentos; os testes partem dos exemplos importados.
		if err := svcs.Segments.EnsureDefaults(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err := svcs.WhatsApp.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svcs.WhatsApp.Shutdown)
	srv := httptest.NewServer(NewRouter(Deps{
		Config: cfg, Logger: logger, DB: pool,
		Auth: svcs.Auth, Users: svcs.Users, Teams: svcs.Teams,
		Hub: svcs.Hub, Conversations: svcs.Conversations, Segments: svcs.Segments, Tags: svcs.Tags, WhatsApp: svcs.WhatsApp, Instance: svcs.Instance,
	}))
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, pool: pool, svcs: svcs, wa: wa, storageDir: storageDir}
}

func (e *env) createUser(email string, role auth.Role, teamIDs ...string) users.User {
	e.t.Helper()
	u, err := e.svcs.Users.Create(context.Background(), "", "", users.CreateInput{
		Email: email, Name: strings.Split(email, "@")[0], Password: password, Role: role, TeamIDs: teamIDs,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

type client struct {
	t      *testing.T
	base   string
	hc     *http.Client
	csrf   string
	origin string
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: e.t, base: e.srv.URL, hc: &http.Client{Jar: jar}, origin: origin}
}

type resp struct {
	status int
	body   map[string]any
	header http.Header
}

func (r resp) code() string {
	if e, ok := r.body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func (c *client) do(method, path string, body any) resp {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	if c.csrf != "" {
		req.Header.Set(auth.CSRFHeader, c.csrf)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out := resp{status: res.StatusCode, header: res.Header}
	raw, _ := io.ReadAll(res.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	if tok, ok := out.body["csrfToken"].(string); ok {
		c.csrf = tok
	}
	return out
}

func (c *client) login(email, pw string) resp {
	c.t.Helper()
	return c.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": pw})
}

func (c *client) mustLogin(email string) *client {
	c.t.Helper()
	if r := c.login(email, password); r.status != 200 {
		c.t.Fatalf("login %s: %d %v", email, r.status, r.body)
	}
	return c
}

func expect(t *testing.T, r resp, status int, code ...string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, esperado %d (%v)", r.status, status, r.body)
	}
	if len(code) > 0 && r.code() != code[0] {
		t.Fatalf("código = %q, esperado %q", r.code(), code[0])
	}
}

// ---------------------------------------------------------------------------

func TestHealth(t *testing.T) {
	e := setup(t)
	c := e.client()
	expect(t, c.do("GET", "/healthz", nil), 200)
	r := c.do("GET", "/readyz", nil)
	expect(t, r, 200)
	if r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("cabeçalho de segurança ausente")
	}
}

func TestLoginLogout(t *testing.T) {
	e := setup(t)
	e.createUser("ana@x.com", auth.RoleAgent)
	c := e.client()

	expect(t, c.do("GET", "/api/v1/auth/me", nil), 401, "unauthenticated")
	expect(t, c.login("ana@x.com", "senha-errada-123"), 401, "invalid_credentials")
	expect(t, c.login("ninguem@x.com", password), 401, "invalid_credentials")

	r := c.login("ANA@x.com", password) // e-mail sem diferenciar maiúsculas
	expect(t, r, 200)
	setCookie := r.header.Get("Set-Cookie")
	for _, attr := range []string{"HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(setCookie, attr) {
			t.Fatalf("cookie sem %s: %s", attr, setCookie)
		}
	}
	if r.body["user"].(map[string]any)["email"] != "ana@x.com" {
		t.Fatalf("me = %v", r.body)
	}

	// Senha nunca em texto puro.
	var hash string
	_ = e.pool.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE email='ana@x.com'`).Scan(&hash)
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, password) {
		t.Fatalf("hash inesperado: %s", hash)
	}

	expect(t, c.do("GET", "/api/v1/auth/me", nil), 200)

	// Logout exige CSRF.
	tok := c.csrf
	c.csrf = ""
	expect(t, c.do("POST", "/api/v1/auth/logout", nil), 403, "csrf_failed")
	c.csrf = tok
	expect(t, c.do("POST", "/api/v1/auth/logout", nil), 204)
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 401)

	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action IN ('auth.login.success','auth.logout','auth.login.failure')`).Scan(&n)
	if n != 4 {
		t.Fatalf("auditoria: %d registros, esperado 4", n)
	}
}

func TestBruteForceThrottle(t *testing.T) {
	e := setup(t)
	e.createUser("bia@x.com", auth.RoleAgent)
	c := e.client()
	for i := 0; i < 5; i++ {
		expect(t, c.login("bia@x.com", "senha-errada-123"), 401)
	}
	r := c.login("bia@x.com", password)
	expect(t, r, 429, "too_many_attempts")
	if r.header.Get("Retry-After") == "" {
		t.Fatal("Retry-After ausente")
	}
}

func TestOriginAndCSRF(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)

	evil := e.client()
	evil.origin = "https://evil.example"
	expect(t, evil.login("adm@x.com", password), 403, "origin_not_allowed")

	c := e.client().mustLogin("adm@x.com")
	good := c.csrf
	c.csrf = "token-falso"
	expect(t, c.do("POST", "/api/v1/teams", map[string]string{"name": "Folha"}), 403, "csrf_failed")
	c.csrf, c.origin = good, "https://evil.example"
	expect(t, c.do("POST", "/api/v1/teams", map[string]string{"name": "Folha"}), 403, "origin_not_allowed")
	c.origin = origin
	expect(t, c.do("POST", "/api/v1/teams", map[string]string{"name": "Folha"}), 201)
}

func TestAdminManagesAgentsAndTeams(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	admin := e.client().mustLogin("adm@x.com")

	folha := admin.do("POST", "/api/v1/teams", map[string]string{"name": "Folha de pagamento"})
	expect(t, folha, 201)
	notas := admin.do("POST", "/api/v1/teams", map[string]string{"name": "Emissão de notas"})
	expect(t, notas, 201)
	folhaID, notasID := folha.body["id"].(string), notas.body["id"].(string)
	expect(t, admin.do("POST", "/api/v1/teams", map[string]string{"name": "folha de PAGAMENTO"}), 422, "validation_failed")

	// Admin cadastra atendente já vinculado à equipe Folha.
	created := admin.do("POST", "/api/v1/users", map[string]any{
		"email": "carla@x.com", "name": "Carla", "password": password, "role": "agent", "teamIds": []string{folhaID},
	})
	expect(t, created, 201)
	carlaID := created.body["id"].(string)
	if teams := created.body["teams"].([]any); len(teams) != 1 {
		t.Fatalf("equipes = %v", teams)
	}
	expect(t, admin.do("POST", "/api/v1/users", map[string]any{
		"email": "carla@x.com", "name": "Carla", "password": password, "role": "agent",
	}), 422)
	expect(t, admin.do("POST", "/api/v1/users", map[string]any{
		"email": "fraca@x.com", "name": "Fraca", "password": "123", "role": "agent",
	}), 422)
	expect(t, admin.do("POST", "/api/v1/users", map[string]any{
		"email": "x@x.com", "name": "X", "password": password, "role": "superuser",
	}), 422)

	agent := e.client().mustLogin("carla@x.com")

	// Atendente não acessa APIs de administração.
	expect(t, agent.do("GET", "/api/v1/users", nil), 403, "forbidden")
	expect(t, agent.do("POST", "/api/v1/teams", map[string]string{"name": "Nova"}), 403, "forbidden")
	expect(t, agent.do("PUT", "/api/v1/teams/"+notasID+"/members/"+carlaID, nil), 403)

	// Atendente vê apenas as próprias equipes.
	list := agent.do("GET", "/api/v1/teams", nil)
	expect(t, list, 200)
	if ts := list.body["teams"].([]any); len(ts) != 1 || ts[0].(map[string]any)["id"] != folhaID {
		t.Fatalf("equipes visíveis = %v", ts)
	}
	expect(t, agent.do("GET", "/api/v1/teams/"+folhaID+"/members", nil), 200)
	expect(t, agent.do("GET", "/api/v1/teams/"+notasID, nil), 404)
	expect(t, agent.do("GET", "/api/v1/teams/"+notasID+"/members", nil), 404)
	expect(t, agent.do("GET", "/api/v1/teams/nao-e-uuid", nil), 404)

	// Admin vincula a uma segunda equipe (idempotente) e ela passa a ser visível.
	expect(t, admin.do("PUT", "/api/v1/teams/"+notasID+"/members/"+carlaID, nil), 204)
	expect(t, admin.do("PUT", "/api/v1/teams/"+notasID+"/members/"+carlaID, nil), 204)
	expect(t, agent.do("GET", "/api/v1/teams/"+notasID, nil), 200)
	expect(t, admin.do("DELETE", "/api/v1/teams/"+notasID+"/members/"+carlaID, nil), 204)
	expect(t, agent.do("GET", "/api/v1/teams/"+notasID, nil), 404)

	// Mudança de perfil vale imediatamente (lida do banco a cada requisição).
	expect(t, admin.do("PATCH", "/api/v1/users/"+carlaID, map[string]any{"role": "admin"}), 200)
	expect(t, agent.do("GET", "/api/v1/users", nil), 200)
	expect(t, admin.do("PATCH", "/api/v1/users/"+carlaID, map[string]any{"role": "agent"}), 200)

	// Desativar encerra as sessões.
	expect(t, admin.do("PATCH", "/api/v1/users/"+carlaID, map[string]any{"isActive": false}), 200)
	expect(t, agent.do("GET", "/api/v1/auth/me", nil), 401)
	expect(t, e.client().login("carla@x.com", password), 401)
}

func TestLastAdminProtection(t *testing.T) {
	e := setup(t)
	adm := e.createUser("adm@x.com", auth.RoleAdmin)
	c := e.client().mustLogin("adm@x.com")
	expect(t, c.do("PATCH", "/api/v1/users/"+adm.ID, map[string]any{"role": "agent"}), 409, "last_admin")
	expect(t, c.do("PATCH", "/api/v1/users/"+adm.ID, map[string]any{"isActive": false}), 409, "self_deactivate")

	other := e.createUser("adm2@x.com", auth.RoleAdmin)
	expect(t, c.do("PATCH", "/api/v1/users/"+other.ID, map[string]any{"role": "agent"}), 200)
}

func TestPasswordChangeAndReset(t *testing.T) {
	e := setup(t)
	e.createUser("adm@x.com", auth.RoleAdmin)
	u := e.createUser("dani@x.com", auth.RoleAgent)
	a := e.client().mustLogin("dani@x.com")
	b := e.client().mustLogin("dani@x.com")

	expect(t, a.do("POST", "/api/v1/auth/password", map[string]string{
		"currentPassword": "errada-errada-123", "newPassword": "outra-senha-forte",
	}), 422)
	expect(t, a.do("POST", "/api/v1/auth/password", map[string]string{
		"currentPassword": password, "newPassword": "curta",
	}), 422)
	expect(t, a.do("POST", "/api/v1/auth/password", map[string]string{
		"currentPassword": password, "newPassword": "outra-senha-forte",
	}), 204)
	expect(t, a.do("GET", "/api/v1/auth/me", nil), 200) // sessão atual mantida
	expect(t, b.do("GET", "/api/v1/auth/me", nil), 401) // outras encerradas
	expect(t, e.client().login("dani@x.com", password), 401)
	expect(t, e.client().login("dani@x.com", "outra-senha-forte"), 200)

	admin := e.client().mustLogin("adm@x.com")
	expect(t, admin.do("POST", "/api/v1/users/"+u.ID+"/password", map[string]string{"password": "senha-do-admin-1"}), 204)
	expect(t, a.do("GET", "/api/v1/auth/me", nil), 401)
	expect(t, e.client().login("dani@x.com", "senha-do-admin-1"), 200)
}

func TestSessionIdleExpiry(t *testing.T) {
	e := setup(t)
	e.createUser("eva@x.com", auth.RoleAgent)
	c := e.client().mustLogin("eva@x.com")
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 200)
	_, _ = e.pool.Exec(context.Background(), `UPDATE sessions SET last_seen_at = now() - interval '31 minutes'`)
	expect(t, c.do("GET", "/api/v1/auth/me", nil), 401)
}

func TestThemePreference(t *testing.T) {
	e := setup(t)
	e.createUser("fe@x.com", auth.RoleAgent)
	c := e.client().mustLogin("fe@x.com")
	expect(t, c.do("PATCH", "/api/v1/me/preferences", map[string]string{"theme": "neon"}), 422)
	expect(t, c.do("PATCH", "/api/v1/me/preferences", map[string]string{"theme": "dark"}), 200)
	me := c.do("GET", "/api/v1/auth/me", nil)
	if me.body["user"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("tema = %v", me.body)
	}
}

func TestAdminMFA(t *testing.T) {
	e := setup(t, opts{requireAdminMFA: true})
	e.createUser("adm@x.com", auth.RoleAdmin)
	e.createUser("gil@x.com", auth.RoleAgent)

	// Atendente não é obrigado a usar MFA.
	expect(t, e.client().mustLogin("gil@x.com").do("GET", "/api/v1/teams", nil), 200)

	c := e.client()
	r := c.login("adm@x.com", password)
	expect(t, r, 200)
	if r.body["mfaSetupRequired"] != true {
		t.Fatalf("esperava mfaSetupRequired: %v", r.body)
	}
	expect(t, c.do("GET", "/api/v1/teams", nil), 403, "mfa_setup_required")

	setupRes := c.do("POST", "/api/v1/auth/mfa/setup", nil)
	expect(t, setupRes, 200)
	secret := setupRes.body["secret"].(string)
	if !strings.HasPrefix(setupRes.body["otpauthUrl"].(string), "otpauth://totp/") {
		t.Fatal("otpauthUrl inválida")
	}
	var enc []byte
	_ = e.pool.QueryRow(context.Background(), `SELECT mfa_pending_secret_enc FROM users WHERE email='adm@x.com'`).Scan(&enc)
	if bytes.Contains(enc, []byte(secret)) {
		t.Fatal("segredo MFA gravado em claro")
	}

	expect(t, c.do("POST", "/api/v1/auth/mfa/enable", map[string]string{"code": "000000"}), 422, "invalid_code")
	now := time.Now()
	code, _ := auth.GenerateTOTP(secret, now)
	expect(t, c.do("POST", "/api/v1/auth/mfa/enable", map[string]string{"code": code}), 200)
	expect(t, c.do("GET", "/api/v1/teams", nil), 200)
	expect(t, c.do("POST", "/api/v1/auth/logout", nil), 204)

	// Novo login: segundo fator pendente.
	c2 := e.client()
	r = c2.login("adm@x.com", password)
	if r.body["mfaRequired"] != true {
		t.Fatalf("esperava mfaRequired: %v", r.body)
	}
	expect(t, c2.do("GET", "/api/v1/teams", nil), 403, "mfa_required")
	expect(t, c2.do("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": code}), 422, "invalid_code") // reuso
	oldCSRF := c2.csrf
	next, _ := auth.GenerateTOTP(secret, now.Add(30*time.Second))
	expect(t, c2.do("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": next}), 200)
	if c2.csrf == oldCSRF {
		t.Fatal("token CSRF deveria ser rotacionado após MFA")
	}
	expect(t, c2.do("GET", "/api/v1/teams", nil), 200)

	// Excesso de códigos errados encerra a sessão pendente.
	c3 := e.client()
	c3.login("adm@x.com", password)
	for i := 0; i < 4; i++ {
		expect(t, c3.do("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": "111111"}), 422)
	}
	expect(t, c3.do("POST", "/api/v1/auth/mfa/verify", map[string]string{"code": "111111"}), 429)
	expect(t, c3.do("GET", "/api/v1/auth/me", nil), 401)
}

func TestRejectsNonJSON(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/login", strings.NewReader("email=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestConcurrentDemotionKeepsOneAdmin(t *testing.T) {
	e := setup(t)
	a := e.createUser("a@x.com", auth.RoleAdmin)
	b := e.createUser("b@x.com", auth.RoleAdmin)
	ca := e.client().mustLogin("a@x.com")
	cb := e.client().mustLogin("b@x.com")

	results := make(chan int, 2)
	go func() { results <- ca.do("PATCH", "/api/v1/users/"+b.ID, map[string]any{"role": "agent"}).status }()
	go func() { results <- cb.do("PATCH", "/api/v1/users/"+a.ID, map[string]any{"role": "agent"}).status }()
	got := []int{<-results, <-results}

	var admins int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE role='admin' AND is_active`).Scan(&admins)
	if admins != 1 {
		t.Fatalf("admins ativos = %d (respostas %v)", admins, got)
	}
	// A perdedora recebe 409 (último admin) ou 403 (já foi rebaixada); nunca 500.
	ok := 0
	for _, st := range got {
		switch st {
		case 200:
			ok++
		case 403, 409:
		default:
			t.Fatalf("resposta inesperada %d (%v)", st, got)
		}
	}
	if ok != 1 {
		t.Fatalf("respostas = %v, esperado exatamente uma 200", got)
	}
}
