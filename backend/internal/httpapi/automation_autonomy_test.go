package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/automation"
)

func TestAutonomyConcurrentEnqueueRespectsClientLimit(t *testing.T) {
	w, _ := autoWorld(t)
	setAutonomy(w, 3)
	cid := w.autoClient("Cliente concorrente", 48)
	var actor automation.Actor
	if err := w.e.pool.QueryRow(t.Context(), `SELECT id::text,user_id::text FROM api_tokens WHERE name='auto'`).Scan(&actor.TokenID, &actor.UserID); err != nil {
		t.Fatal(err)
	}
	actor.Provider = "claude"
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, key := range []string{"one", "two"} {
		go func(key string) {
			<-start
			_, err := w.e.svcs.Automation.Enqueue(t.Context(), actor, automation.EnqueueInput{ContactID: cid, Category: "das", Reason: "DAS disponível", Text: "Olá! A guia do DAS está disponível.", DedupeKey: key})
			results <- err
		}(key)
	}
	close(start)
	accepted := 0
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
			continue
		}
		var refusal *automation.Refusal
		if !errors.As(err, &refusal) || refusal.Code != "client_daily" {
			t.Fatalf("unexpected refusal: %v", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted concurrent sends: %d", accepted)
	}
}

func setAutonomy(w *world, level int) {
	w.e.t.Helper()
	expect(w.e.t, w.admin.do("PUT", "/api/v1/automation/categories/das", map[string]any{
		"enabled": true, "level": level, "windowStart": "08:00", "windowEnd": "18:00",
		"weekdays": []int{1, 2, 3, 4, 5}, "minIntervalDays": 20, "guidelines": "Seja cordial.",
	}), 200)
}

func autoSendState(t *testing.T, w *world, id string) string {
	t.Helper()
	var status string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT status FROM automation_sends WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestAutonomyPersistsAndSendsWithoutBrowserOrApproval(t *testing.T) {
	w, tok := autoWorld(t)
	setAutonomy(w, 3)
	cid := w.autoClient("Cliente autônomo", 48)
	out := queue(w, tok, cid, "auto", "Olá, {nome}! A guia do DAS já está disponível.")
	if out["status"] != "na_fila" || out["envio_previsto"] == nil {
		t.Fatalf("queue: %v", out)
	}
	id := out["id"].(string)
	expect(t, w.admin.do("POST", "/api/v1/auth/logout", nil), 204)
	// Nova instância do worker, sem sessão web: o pedido vem do banco.
	worker := automation.NewService(w.e.pool, w.e.svcs.Conversations, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker.SetClock(func() time.Time { return autoNow })
	before := len(w.e.wa.Sent())
	if worker.Tick(t.Context()) != 1 || autoSendState(t, w, id) != "sent" || len(w.e.wa.Sent()) != before+1 {
		t.Fatal("autonomous queue did not send")
	}
	if worker.Tick(t.Context()) != 0 || len(w.e.wa.Sent()) != before+1 {
		t.Fatal("duplicate send")
	}
	var approved bool
	if err := w.e.pool.QueryRow(t.Context(), `SELECT approved_by IS NOT NULL FROM automation_sends WHERE id=$1`, id).Scan(&approved); err != nil || approved {
		t.Fatalf("fabricated human approval: %v", err)
	}
}

func TestAutonomyWarningsAndDowngradeRequireReview(t *testing.T) {
	w, tok := autoWorld(t)
	setAutonomy(w, 3)
	cid := w.autoClient("Cliente sinalizado", 48)
	out := queue(w, tok, cid, "warning", "Olá! Evite protesto e confira a guia do DAS.")
	if out["status"] != "aguardando_aprovacao" {
		t.Fatalf("warning bypassed: %v", out)
	}
	cid2 := w.autoClient("Cliente mudança", 48)
	out = queue(w, tok, cid2, "downgrade", "Olá, {nome}! Sua guia do DAS está disponível.")
	id := out["id"].(string)
	setAutonomy(w, 1)
	if w.e.svcs.Automation.Tick(t.Context()) != 0 || autoSendState(t, w, id) != "pending_approval" {
		t.Fatal("downgrade ignored")
	}
	setAutonomy(w, 3)
	if w.e.svcs.Automation.Tick(t.Context()) != 0 {
		t.Fatal("pending requests silently upgraded")
	}
}

func TestAutonomyRechecksServerRules(t *testing.T) {
	for _, rule := range []string{"paused", "window", "master", "revoked", "expired", "assigned", "optout", "cancelled", "category", "dialog", "content", "pressure"} {
		t.Run(rule, func(t *testing.T) {
			w, tok := autoWorld(t)
			setAutonomy(w, 3)
			cid := w.autoClient("Cliente regra", 48)
			out := queue(w, tok, cid, "rule", "Olá, {nome}! A guia do DAS está disponível.")
			id := out["id"].(string)
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, err := w.e.pool.Exec(t.Context(), sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			switch rule {
			case "paused":
				expect(t, w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": true}), 204)
			case "window":
				w.e.svcs.Automation.SetClock(func() time.Time { return time.Date(2026, 11, 15, 14, 0, 0, 0, time.UTC) })
			case "master":
				exec(`UPDATE ai_settings SET allow_automation=false`)
			case "revoked":
				exec(`UPDATE api_tokens SET revoked_at=now() WHERE id=(SELECT token_id FROM automation_sends WHERE id=$1)`, id)
			case "expired":
				exec(`UPDATE api_tokens SET expires_at=now()-interval '1 hour' WHERE id=(SELECT token_id FROM automation_sends WHERE id=$1)`, id)
			case "assigned":
				exec(`UPDATE conversations SET assignee_id=$2,status='in_progress' WHERE contact_id=$1`, cid, w.users["ana"])
			case "optout":
				expect(t, w.admin.do("PUT", "/api/v1/automation/contacts/"+cid+"/opt-out", map[string]any{"optOut": true}), 204)
			case "cancelled":
				expect(t, w.admin.do("POST", "/api/v1/automation/sends/cancel", map[string]any{"ids": []string{id}}), 200)
			case "category":
				exec(`UPDATE automation_categories SET enabled=false WHERE key='das'`)
			case "dialog":
				exec(`UPDATE messages SET sent_at=$2 WHERE conversation_id IN (SELECT id FROM conversations WHERE contact_id=$1)`, cid, autoNow)
			case "content":
				exec(`UPDATE automation_sends SET text='Pagamento pelo PIX 12345678901234' WHERE id=$1`, id)
			case "pressure":
				exec(`UPDATE automation_sends SET text='Evite protesto e confira sua guia.' WHERE id=$1`, id)
			}
			before := len(w.e.wa.Sent())
			if w.e.svcs.Automation.Tick(t.Context()) != 0 || len(w.e.wa.Sent()) != before {
				t.Fatalf("rule ignored: %s", rule)
			}
		})
	}
}

func TestAutonomyOAuthConsentRefreshAndDisconnect(t *testing.T) {
	w, _ := autoWorld(t)
	setAutonomy(w, 3)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "allowWrite": true, "allowSend": true, "allowAutomation": true, "allowOauth": true, "oauthRedirectHosts": []string{"chatgpt.com"}}), 200)
	const redirect = "https://chatgpt.com/connector_platform_oauth_redirect"
	st, client, _ := w.jsonPost("/oauth/register", map[string]any{"client_name": "ChatGPT", "redirect_uris": []string{redirect}})
	if st != 201 {
		t.Fatalf("registration: %d", st)
	}
	clientID := client["client_id"].(string)
	connect := func(level string) map[string]any {
		t.Helper()
		verifier, challenge := pkce()
		r := w.admin.do("POST", "/api/v1/ai/oauth/authorize", map[string]any{"clientId": clientID, "redirectUri": redirect, "codeChallenge": challenge, "codeChallengeMethod": "S256", "level": level, "approve": true})
		expect(t, r, 200)
		back, _ := url.Parse(r.body["redirectTo"].(string))
		st, pair := w.formPost("/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {back.Query().Get("code")}, "redirect_uri": {redirect}, "code_verifier": {verifier}})
		if st != 200 {
			t.Fatalf("token exchange: %d %v", st, pair)
		}
		return pair
	}
	read := connect("read")
	w.expectDenied(read["access_token"].(string), "send_automatic_message", map[string]any{"contact_id": "x", "category": "das", "reason": "DAS 10/2026", "text": "Olá", "dedupe_key": "k"}, "nunca recebem")
	pair := connect("automate")
	tok := pair["access_token"].(string)
	if !toolNames(w, tok)["send_automatic_message"] {
		t.Fatal("explicit consent missing automation")
	}
	cid := w.autoClient("Cliente OAuth", 48)
	out := queue(w, tok, cid, "oauth", "Olá, {nome}! A guia do DAS está disponível.")
	if out["status"] != "na_fila" {
		t.Fatalf("OAuth automation: %v", out)
	}
	// Renovar revoga o token de acesso antigo, mas não a autorização da fila.
	st, next := w.formPost("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {pair["refresh_token"].(string)}})
	if st != 200 {
		t.Fatalf("refresh: %d %v", st, next)
	}
	if w.e.svcs.Automation.Tick(t.Context()) != 1 {
		t.Fatal("OAuth rotation cancelled authorized queue")
	}
	cid2 := w.autoClient("Cliente desconexão", 48)
	out = queue(w, next["access_token"].(string), cid2, "disconnect", "Olá, {nome}! A guia do DAS está disponível.")
	id := out["id"].(string)
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE oauth_grants SET revoked_at=now() WHERE id=(SELECT grant_id FROM api_tokens WHERE id=(SELECT token_id FROM automation_sends WHERE id=$1))`, id); err != nil {
		t.Fatal(err)
	}
	w.e.svcs.Automation.SetClock(func() time.Time { return autoNow.Add(time.Minute) })
	if w.e.svcs.Automation.Tick(t.Context()) != 0 || autoSendState(t, w, id) != "skipped" {
		t.Fatal("disconnected OAuth still sent")
	}
}
