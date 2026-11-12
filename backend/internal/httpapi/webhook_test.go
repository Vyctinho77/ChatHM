package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/aiaccess"
)

type hookReq struct {
	header http.Header
	body   []byte
}

type hookReceiver struct {
	mu     sync.Mutex
	reqs   []hookReq
	status int
}

func newHookReceiver(t *testing.T) (*httptest.Server, *hookReceiver) {
	t.Helper()
	rec := &hookReceiver{status: 200}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, hookReq{header: r.Header.Clone(), body: b})
		st := rec.status
		rec.mu.Unlock()
		w.WriteHeader(st)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func (r *hookReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *hookReceiver) last() hookReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[len(r.reqs)-1]
}

func (r *hookReceiver) setStatus(s int) {
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
}

// deliverNow faz vencer os avisos pendentes e entrega (sem esperar a janela de 5 s que junta as rajadas).
func deliverNow(t *testing.T, w *world) int {
	t.Helper()
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE ai_webhook_deliveries SET next_attempt_at = now() - interval '1 second' WHERE status = 'pending'`); err != nil {
		t.Fatal(err)
	}
	return w.e.svcs.AI.DeliverDue(t.Context())
}

func pendingHooks(t *testing.T, w *world) (n, maxCount int) {
	t.Helper()
	if err := w.e.pool.QueryRow(t.Context(), `SELECT count(*), coalesce(max(message_count), 0) FROM ai_webhook_deliveries WHERE status = 'pending'`).Scan(&n, &maxCount); err != nil {
		t.Fatal(err)
	}
	return
}

func decodeHook(t *testing.T, r hookReq) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.body, &out); err != nil {
		t.Fatalf("corpo do webhook inválido: %v", err)
	}
	return out
}

func configureHook(t *testing.T, w *world, url string, onlyAI, preview bool) string {
	t.Helper()
	r := w.admin.do("PUT", "/api/v1/ai/webhook", map[string]any{"enabled": true, "url": url, "onlyAiReplies": onlyAI, "includePreview": preview})
	expect(t, r, 200)
	secret, _ := r.body["secret"].(string)
	return secret
}

// Conversa em que a IA já falou (envio do atendente), e depois o cliente responde.
func aiConversation(t *testing.T, w *world, toks map[string]string, name string) (convID, phone string) {
	t.Helper()
	phone = w.receive(name, "Bom dia")
	convID = mustConv(t, w.dani, name)["id"].(string)
	if out, _ := w.tool(toks["attend"], "send_as_attendant", map[string]any{"conversation_id": convID, "text": "Seu DAS vence dia 20."}); out["ok"] != true {
		t.Fatalf("envio da IA: %v", out)
	}
	return convID, phone
}

func TestWebhookDeliversSignedMinimalNoticeWhenCustomerReplies(t *testing.T) {
	w, toks := agentTokens(t)
	w.e.svcs.AI.AllowPrivateWebhooks()
	srv, rec := newHookReceiver(t)

	// Só o administrador vê e configura.
	expect(t, w.ana.do("GET", "/api/v1/ai/webhook", nil), 403)
	expect(t, w.ana.do("PUT", "/api/v1/ai/webhook", map[string]any{"enabled": true, "url": srv.URL}), 403)
	expect(t, w.admin.do("PUT", "/api/v1/ai/webhook", map[string]any{"enabled": true, "url": "isso não é um endereço"}), 422)
	secret := configureHook(t, w, srv.URL, true, false)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("o segredo deveria ser mostrado na primeira vez: %q", secret)
	}
	// Depois, nunca mais: a configuração diz só que existe.
	cfg := w.admin.do("GET", "/api/v1/ai/webhook", nil)
	expect(t, cfg, 200)
	if cfg.body["hasSecret"] != true || cfg.body["secret"] != nil || strings.Contains(toJSON(cfg.body), secret) {
		t.Fatalf("a configuração não pode devolver o segredo: %v", cfg.body)
	}
	var stored []byte
	if err := w.e.pool.QueryRow(t.Context(), `SELECT secret_enc FROM ai_webhook WHERE id`).Scan(&stored); err != nil || strings.Contains(string(stored), secret) {
		t.Fatalf("segredo guardado em texto puro: %v", err)
	}
	if again := configureHook(t, w, srv.URL, true, false); again != "" {
		t.Fatal("salvar de novo não deveria gerar outro segredo")
	}

	convID, phone := aiConversation(t, w, toks, "Cliente Aviso")
	const secretText = "Já paguei o DAS, obrigado!"
	w.receiveFrom(phone, "Cliente Aviso", secretText)
	if n, c := pendingHooks(t, w); n != 1 || c != 1 {
		t.Fatalf("pendentes = %d (contagem %d)", n, c)
	}
	if got := w.e.svcs.AI.DeliverDue(t.Context()); got != 0 || rec.count() != 0 { // ainda dentro da janela que junta rajadas
		t.Fatalf("saiu antes da janela: %d", got)
	}
	if deliverNow(t, w) != 1 || rec.count() != 1 {
		t.Fatalf("entregas = %d", rec.count())
	}
	req := rec.last()
	if !aiaccess.VerifyWebhookSignature(secret, req.header.Get("X-ChatHM-Signature"), req.body, time.Now(), 5*time.Minute) {
		t.Fatal("a assinatura não confere com o segredo")
	}
	if req.header.Get("X-ChatHM-Event") != "message.received" || req.header.Get("X-ChatHM-Delivery") == "" || req.header.Get("User-Agent") != "ChatHM-Webhook/1" {
		t.Fatalf("cabeçalhos: %v", req.header)
	}
	ev := decodeHook(t, req)
	conv := ev["conversation"].(map[string]any)
	if ev["type"] != "message.received" || conv["id"] != convID || conv["queue"].(map[string]any)["key"] != "geral" || ev["replyToAi"] != true ||
		ev["contact"].(map[string]any)["name"] != "Cliente Aviso" || ev["messages"].(map[string]any)["count"] != float64(1) {
		t.Fatalf("evento = %v", ev)
	}
	if ev["preview"] != nil || strings.Contains(string(req.body), "paguei") {
		t.Fatal("o texto da mensagem não pode sair por padrão")
	}
	if n, _ := pendingHooks(t, w); n != 0 {
		t.Fatal("o aviso entregue ainda consta como pendente")
	}

	// Rajada: três respostas viram UM aviso, com a contagem.
	for _, txt := range []string{"uma", "duas", "três"} {
		w.receiveFrom(phone, "Cliente Aviso", txt)
	}
	if n, c := pendingHooks(t, w); n != 1 || c != 3 {
		t.Fatalf("rajada: pendentes = %d, contagem = %d", n, c)
	}
	deliverNow(t, w)
	if got := decodeHook(t, rec.last())["messages"].(map[string]any)["count"]; got != float64(3) {
		t.Fatalf("contagem do aviso = %v", got)
	}

	// Conversa em que nenhuma IA falou: sem aviso (por padrão), com aviso quando o administrador amplia.
	phone2 := w.receive("Cliente Sem IA", "Oi")
	w.receiveFrom(phone2, "Cliente Sem IA", "Alguém aí?")
	if n, _ := pendingHooks(t, w); n != 0 {
		t.Fatal("avisou uma conversa em que nenhuma IA falou")
	}
	configureHook(t, w, srv.URL, false, false)
	w.receiveFrom(phone2, "Cliente Sem IA", "Alguém aí mesmo?")
	if n, _ := pendingHooks(t, w); n != 1 {
		t.Fatalf("com onlyAiReplies desligado deveria avisar: %d", n)
	}
	deliverNow(t, w)
	if decodeHook(t, rec.last())["replyToAi"] != false {
		t.Fatalf("replyToAi deveria ser falso: %v", decodeHook(t, rec.last()))
	}
}

func TestWebhookPreviewIsOptInAndMasked(t *testing.T) {
	w, toks := agentTokens(t)
	w.e.svcs.AI.AllowPrivateWebhooks()
	srv, rec := newHookReceiver(t)
	configureHook(t, w, srv.URL, true, true)
	_, phone := aiConversation(t, w, toks, "Cliente Trecho")
	w.receiveFrom(phone, "Cliente Trecho", "Meu CPF é 123.456.789-01, ligue (92) 98800-1111, e-mail ana@empresa.com. senha: Abc12345! Pode conferir a guia?")
	deliverNow(t, w)
	prev, _ := decodeHook(t, rec.last())["preview"].(string)
	if prev == "" || !strings.Contains(prev, "guia") {
		t.Fatalf("o trecho deveria vir quando o administrador liga: %q", prev)
	}
	for _, leak := range []string{"123.456.789-01", "98800-1111", "ana@empresa.com", "Abc12345!"} {
		if strings.Contains(prev, leak) {
			t.Fatalf("o trecho vaza %q: %s", leak, prev)
		}
	}
	// Trecho longo é cortado.
	w.receiveFrom(phone, "Cliente Trecho", strings.Repeat("palavra ", 100))
	deliverNow(t, w)
	if p, _ := decodeHook(t, rec.last())["preview"].(string); len([]rune(p)) > 161 {
		t.Fatalf("trecho com %d caracteres", len([]rune(p)))
	}
}

func TestWebhookRetriesWithBackoffThenGivesUpAndDisablesItself(t *testing.T) {
	w, toks := agentTokens(t)
	w.e.svcs.AI.AllowPrivateWebhooks()
	srv, rec := newHookReceiver(t)
	configureHook(t, w, srv.URL, true, false)
	_, phone := aiConversation(t, w, toks, "Cliente Falha")

	rec.setStatus(500)
	w.receiveFrom(phone, "Cliente Falha", "primeira")
	if deliverNow(t, w) != 0 {
		t.Fatal("o receptor falhou: nada deveria constar como entregue")
	}
	var attempts int
	var next time.Time
	var lastErr string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT attempts, next_attempt_at, last_error FROM ai_webhook_deliveries WHERE status = 'pending'`).Scan(&attempts, &next, &lastErr); err != nil {
		t.Fatalf("o aviso deveria seguir pendente para reenvio: %v", err)
	}
	if attempts != 1 || !next.After(time.Now().Add(5*time.Second)) || !strings.Contains(lastErr, "500") {
		t.Fatalf("tentativas=%d próxima=%v erro=%q", attempts, next, lastErr)
	}
	cfg := w.admin.do("GET", "/api/v1/ai/webhook", nil)
	if cfg.body["lastStatus"] != "500" || cfg.body["lastError"] == "" {
		t.Fatalf("o administrador deveria ver o último erro: %v", cfg.body)
	}
	// O receptor volta: o mesmo aviso sai (sem duplicar) e a falha some.
	rec.setStatus(200)
	if deliverNow(t, w) != 1 || rec.count() != 2 {
		t.Fatalf("reenvio: recebidos = %d", rec.count())
	}
	if n, _ := pendingHooks(t, w); n != 0 {
		t.Fatal("ficou pendente depois de entregue")
	}

	// Esgotadas as tentativas, o aviso é descartado; muitas falhas seguidas desligam o webhook sozinho.
	rec.setStatus(500)
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE ai_webhook SET consecutive_failures = 19`); err != nil {
		t.Fatal(err)
	}
	w.receiveFrom(phone, "Cliente Falha", "segunda")
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE ai_webhook_deliveries SET attempts = 5 WHERE status = 'pending'`); err != nil {
		t.Fatal(err)
	}
	deliverNow(t, w)
	var status string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT status FROM ai_webhook_deliveries ORDER BY created_at DESC LIMIT 1`).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("status do aviso esgotado = %q, %v", status, err)
	}
	cfg = w.admin.do("GET", "/api/v1/ai/webhook", nil)
	if cfg.body["enabled"] != false || !strings.Contains(cfg.body["disabledReason"].(string), "Desligado sozinho") {
		t.Fatalf("deveria desligar sozinho: %v", cfg.body)
	}
	// Desligado: mensagens novas não geram avisos.
	w.receiveFrom(phone, "Cliente Falha", "terceira")
	if n, _ := pendingHooks(t, w); n != 0 {
		t.Fatal("webhook desligado gerou aviso")
	}
}

func TestWebhookTestEndpointRotateSecretAndDisableClearsQueue(t *testing.T) {
	w, toks := agentTokens(t)
	w.e.svcs.AI.AllowPrivateWebhooks()
	srv, rec := newHookReceiver(t)
	expect(t, w.admin.do("POST", "/api/v1/ai/webhook/test", nil), 422) // sem endereço salvo
	secret := configureHook(t, w, srv.URL, true, false)

	r := w.admin.do("POST", "/api/v1/ai/webhook/test", nil)
	expect(t, r, 200)
	if r.body["ok"] != true || r.body["status"] != float64(200) || rec.count() != 1 {
		t.Fatalf("teste: %v", r.body)
	}
	req := rec.last()
	if req.header.Get("X-ChatHM-Event") != "webhook.test" || decodeHook(t, req)["type"] != "webhook.test" ||
		!aiaccess.VerifyWebhookSignature(secret, req.header.Get("X-ChatHM-Signature"), req.body, time.Now(), 5*time.Minute) {
		t.Fatalf("aviso de teste: %v", req.header)
	}
	rec.setStatus(503)
	if r := w.admin.do("POST", "/api/v1/ai/webhook/test", nil); r.body["ok"] != false || r.body["status"] != float64(503) || r.body["error"] == "" {
		t.Fatalf("teste com falha: %v", r.body)
	}
	rec.setStatus(200)

	// Trocar o segredo: o antigo deixa de valer.
	rot := w.admin.do("POST", "/api/v1/ai/webhook/secret", nil)
	expect(t, rot, 200)
	newSecret := rot.body["secret"].(string)
	if newSecret == "" || newSecret == secret {
		t.Fatalf("segredo novo = %q", newSecret)
	}
	expect(t, w.ana.do("POST", "/api/v1/ai/webhook/secret", nil), 403)
	w.admin.do("POST", "/api/v1/ai/webhook/test", nil)
	req = rec.last()
	if aiaccess.VerifyWebhookSignature(secret, req.header.Get("X-ChatHM-Signature"), req.body, time.Now(), time.Minute) ||
		!aiaccess.VerifyWebhookSignature(newSecret, req.header.Get("X-ChatHM-Signature"), req.body, time.Now(), time.Minute) {
		t.Fatal("depois de trocar, só o segredo novo deveria validar")
	}

	// Desligar descarta o que estava pendente (religar não despeja avisos velhos).
	_, phone := aiConversation(t, w, toks, "Cliente Desliga")
	w.receiveFrom(phone, "Cliente Desliga", "oi")
	if n, _ := pendingHooks(t, w); n != 1 {
		t.Fatalf("pendentes = %d", n)
	}
	expect(t, w.admin.do("PUT", "/api/v1/ai/webhook", map[string]any{"enabled": false, "url": srv.URL, "onlyAiReplies": true}), 200)
	before := rec.count()
	if deliverNow(t, w) != 0 || rec.count() != before {
		t.Fatal("webhook desligado entregou")
	}
	if n, _ := pendingHooks(t, w); n != 0 {
		t.Fatal("o pendente deveria ter sido descartado")
	}
	// my_access informa se o aviso está ligado.
	out, _ := w.tool(toks["read"], "my_access", map[string]any{})
	if out["notifications"].(map[string]any)["webhookEnabled"] != false {
		t.Fatalf("my_access: %v", out["notifications"])
	}
}
