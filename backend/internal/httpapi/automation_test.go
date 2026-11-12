package httpapi

import (
	"strings"
	"testing"
	"time"
)

var autoNow = time.Date(2026, 11, 11, 14, 0, 0, 0, time.UTC)

// autoWorld liga a IA e a automação, libera a categoria "das" e fixa um relógio em horário comercial (quarta, 10h em Manaus).
func autoWorld(t *testing.T) (*world, string) {
	t.Helper()
	w := setupWorld(t)
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": true}), 200)
	expect(t, w.admin.do("PUT", "/api/v1/automation/categories/das", map[string]any{
		"label": "DAS, guias e impostos", "enabled": true, "level": 1, "windowStart": "08:00", "windowEnd": "18:00",
		"weekdays": []int{1, 2, 3, 4, 5}, "minIntervalDays": 20, "guidelines": "Seja cordial.",
	}), 200)
	w.e.svcs.Automation.SetClock(func() time.Time { return autoNow })
	w.e.svcs.Automation.SetRandom(func(int) int { return 0 })
	// O ritmo guarda "quando o próximo envio pode sair" com o now() do banco; sem isso os testes dependem do relógio real passar do relógio fixo.
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE automation_state SET next_allowed_at = 'epoch'`); err != nil {
		t.Fatal(err)
	}
	r := w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "auto", "scopes": []string{"automate"}, "provider": "claude"})
	expect(t, r, 201)
	return w, r.body["token"].(string)
}

// client cria um cliente com conversa e devolve o id do contato (mensagem antiga: sem diálogo ativo).
func (w *world) autoClient(name string, ageHours int) string {
	w.receive(name, "Bom dia")
	c := mustConv(w.e.t, w.admin, name)
	cid := c["contact"].(map[string]any)["id"].(string)
	if _, err := w.e.pool.Exec(w.e.t.Context(), `UPDATE messages SET sent_at = $3::timestamptz - make_interval(hours => $2) WHERE conversation_id = $1`, c["id"], ageHours, autoNow); err != nil {
		w.e.t.Fatal(err)
	}
	return cid
}

func queue(w *world, tok, cid, key, text string) map[string]any {
	out, _ := w.tool(tok, "send_automatic_message", map[string]any{
		"contact_id": cid, "category": "das", "reason": "DAS 10/2026 vence em 20/11", "text": text, "dedupe_key": key,
	})
	return out
}

func TestAutomationFlow(t *testing.T) {
	w, tok := autoWorld(t)
	cid := w.autoClient("Cliente Auto", 48)

	out := queue(w, tok, cid, "das-2026-10", "Olá, {nome}! A guia do DAS de outubro já está disponível e vence em 20/11.")
	if out["ok"] != true || out["status"] != "aguardando_aprovacao" || out["id"] == nil {
		t.Fatalf("enqueue: %v", out)
	}
	id := out["id"].(string)
	before := len(w.e.wa.Sent())

	// Nada sai sem aprovação, mesmo com o worker rodando.
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 || len(w.e.wa.Sent()) != before {
		t.Fatalf("saiu sem aprovação: %d", n)
	}
	// Repetição (mesma chave) é recusada.
	if dup := queue(w, tok, cid, "das-2026-10", "Olá de novo, tudo bem? Guia do DAS."); dup["ok"] == true {
		t.Fatalf("repetição aceita: %v", dup)
	}
	// Só o administrador aprova.
	expect(t, w.ana.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": []string{id}}), 403)
	r := w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": []string{id}})
	expect(t, r, 200)

	if n := w.e.svcs.Automation.Tick(t.Context()); n != 1 {
		t.Fatalf("Tick = %d", n)
	}
	sent := w.e.wa.Sent()
	if len(sent) != before+1 {
		t.Fatalf("enviadas = %d", len(sent))
	}
	txt := sent[len(sent)-1].Text
	if !strings.HasPrefix(txt, "*Assistente do escritório:*") || !strings.Contains(txt, "Cliente") || !strings.Contains(txt, "PARAR") {
		t.Fatalf("texto = %q", txt)
	}
	// A conversa continua sem responsável e a mensagem é marcada como automática, com o provedor.
	c := mustConv(t, w.admin, "Cliente Auto")
	if c["assignee"] != nil && c["assignee"] != "" {
		t.Fatalf("assumiu a conversa: %v", c["assignee"])
	}
	var found bool
	for _, m := range msgList(t, w.admin, c["id"].(string)) {
		if m["automated"] == true && m["aiProvider"] == "claude" && m["aiApp"] == "Claude" {
			found = true
		}
	}
	if !found {
		t.Fatal("mensagem sem marca de automática/provedor")
	}
	// Segundo Tick: o ritmo ainda não deixa (intervalo entre envios).
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 {
		t.Fatalf("ritmo ignorado: %d", n)
	}
}

func TestAutomationGuards(t *testing.T) {
	w, tok := autoWorld(t)
	code := func(out map[string]any) string { c, _ := out["codigo"].(string); return c }
	ok := "Olá, {nome}! A guia do DAS de outubro já está disponível."

	// Diálogo ativo: cliente escreveu agora, ninguém respondeu → nunca entra na conversa.
	hot := w.autoClient("Cliente Quente", 0)
	if c := code(queue(w, tok, hot, "k1", ok)); c != "dialog_active" {
		t.Fatalf("dialog = %q", c)
	}
	cold := w.autoClient("Cliente Frio", 48)
	// Conteúdo.
	for name, text := range map[string]string{
		"negrito": "Olá! *Urgente* pague o DAS.", "link": "Veja https://golpe.com/x agora.",
		"email": "Escreva para fraude@x.com sobre o DAS.", "numero": "Pague no PIX 12345678901 o DAS.",
		"variavel": "Pague para {pix_escritorio} o DAS.",
	} {
		if c := code(queue(w, tok, cold, "c-"+name, text)); c == "" {
			t.Fatalf("%s aceito", name)
		}
	}
	// Categoria desligada ou inexistente.
	out, _ := w.tool(tok, "send_automatic_message", map[string]any{"contact_id": cold, "category": "cobranca", "reason": "Honorários 10/2026", "text": ok, "dedupe_key": "x"})
	if code(out) != "category_off" {
		t.Fatalf("categoria = %v", out)
	}
	// Termos de pressão geram aviso para o aprovador, mas enfileiram.
	p := queue(w, tok, cold, "pressao", "Olá! O DAS de outubro vence logo; evite protesto.")
	if p["ok"] != true || p["avisos"] == nil {
		t.Fatalf("pressão = %v", p)
	}
	// Opt-out: o cliente responde PARAR.
	w.receiveFrom(phoneOf(t, w, "Cliente Frio"), "Cliente Frio", "PARAR")
	eventually(t, "opt-out", func() bool {
		return code(queue(w, tok, cold, "k-optout", ok)) == "opt_out"
	})
	// Contato inexistente.
	out, _ = w.tool(tok, "send_automatic_message", map[string]any{"contact_id": "00000000-0000-0000-0000-000000000000", "category": "das", "reason": "DAS 10/2026", "text": ok, "dedupe_key": "z"})
	if out["ok"] == true {
		t.Fatalf("contato inexistente aceito: %v", out)
	}
	// Pausa geral: recusa e nada sai.
	cold2 := w.autoClient("Cliente Dois", 48)
	expect(t, w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": true, "reason": "teste"}), 204)
	if c := code(queue(w, tok, cold2, "kp", ok)); c != "paused" {
		t.Fatalf("pausa = %q", c)
	}
}

func phoneOf(t *testing.T, w *world, name string) string {
	t.Helper()
	var p string
	if err := w.e.pool.QueryRow(t.Context(), `SELECT wa_id FROM contacts WHERE name = $1`, name).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutomationToolsNeedScopeAndSwitch(t *testing.T) {
	w, tok := autoWorld(t)
	if !toolNames(w, tok)["send_automatic_message"] {
		t.Fatal("token automate sem a ferramenta")
	}
	plain := w.mkWriteToken(w.admin, "escrita")
	w.expectDenied(plain, "send_automatic_message", map[string]any{"contact_id": "x", "category": "das", "reason": "DAS 10/2026", "text": "Olá", "dedupe_key": "k"}, "escopo automate")
	// Chave mestra desligada: o token antigo perde a ferramenta e novos não nascem.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": false}), 200)
	w.expectDenied(tok, "send_automatic_message", map[string]any{"contact_id": "x", "category": "das", "reason": "DAS 10/2026", "text": "Olá", "dedupe_key": "k"}, "Permitir envios automáticos")
	expect(t, w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "n", "scopes": []string{"automate"}, "provider": "claude"}), 422)
	// Sem provedor, não cria.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": true}), 200)
	expect(t, w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "n2", "scopes": []string{"automate"}}), 422)
}

func TestAutomationPacingAndWindow(t *testing.T) {
	w, tok := autoWorld(t)
	a, b := w.autoClient("Cliente A", 48), w.autoClient("Cliente B", 48)
	ids := []string{}
	for i, cid := range []string{a, b} {
		o := queue(w, tok, cid, "das-2026-10", "Olá, {nome}! Guia do DAS disponível.")
		_ = i
		ids = append(ids, o["id"].(string))
	}
	expect(t, w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": ids}), 200)

	// Fora da janela (domingo 16/11, 10h Manaus): nada sai.
	sun := time.Date(2026, 11, 15, 14, 0, 0, 0, time.UTC)
	w.e.svcs.Automation.SetClock(func() time.Time { return sun })
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 {
		t.Fatalf("saiu no domingo: %d", n)
	}
	// Dentro da janela: sai um por vez, respeitando o intervalo.
	now := time.Date(2026, 11, 11, 14, 0, 0, 0, time.UTC)
	w.e.svcs.Automation.SetClock(func() time.Time { return now })
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 1 {
		t.Fatalf("primeiro = %d", n)
	}
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 {
		t.Fatalf("sem intervalo = %d", n)
	}
	now = now.Add(time.Minute)
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 1 {
		t.Fatalf("segundo = %d", n)
	}
	// Pausar tudo: o que sobrar não sai.
	c := w.autoClient("Cliente C", 48)
	o := queue(w, tok, c, "das-2026-10", "Olá, {nome}! Guia do DAS disponível.")
	expect(t, w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": []string{o["id"].(string)}}), 200)
	expect(t, w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": true, "reason": "revisão"}), 204)
	now = now.Add(time.Minute)
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 {
		t.Fatalf("saiu pausado: %d", n)
	}
}

func TestAutomationManualOptOut(t *testing.T) {
	w, tok := autoWorld(t)
	cid := w.autoClient("Cliente Manual", 48)
	// Agente comum não mexe; o administrador marca e desmarca.
	expect(t, w.ana.do("PUT", "/api/v1/automation/contacts/"+cid+"/opt-out", map[string]any{"optOut": true}), 403)
	expect(t, w.admin.do("PUT", "/api/v1/automation/contacts/"+cid+"/opt-out", map[string]any{"optOut": true}), 204)
	r := w.admin.do("GET", "/api/v1/automation/contacts/"+cid, nil)
	expect(t, r, 200)
	if r.body["optOut"] != true {
		t.Fatalf("estado = %v", r.body)
	}
	if out := queue(w, tok, cid, "m1", "Olá, {nome}! Guia do DAS disponível."); out["codigo"] != "opt_out" {
		t.Fatalf("opt-out ignorado: %v", out)
	}
	expect(t, w.admin.do("PUT", "/api/v1/automation/contacts/"+cid+"/opt-out", map[string]any{"optOut": false}), 204)
	if out := queue(w, tok, cid, "m1", "Olá, {nome}! Guia do DAS disponível."); out["ok"] != true {
		t.Fatalf("depois de desmarcar: %v", out)
	}
}
