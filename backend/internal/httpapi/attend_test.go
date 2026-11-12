package httpapi

import (
	"strings"
	"testing"
)

func attendWorld(t *testing.T) (*world, string) {
	t.Helper()
	w := setupWorld(t)
	set := func(attend bool) {
		expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": false, "allowAttend": attend}), 200)
	}
	// Desligado por padrão: ninguém cria o token.
	set(false)
	expect(t, w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "x", "scopes": []string{"attend"}, "provider": "grok"}), 422)
	set(true)
	// Só administrador cria; precisa informar o provedor.
	expect(t, w.ana.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "x", "scopes": []string{"attend"}, "provider": "grok"}), 403)
	expect(t, w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "x", "scopes": []string{"attend"}}), 422)
	r := w.admin.do("POST", "/api/v1/ai/tokens", map[string]any{"name": "Grok atendente", "scopes": []string{"attend"}, "provider": "grok"})
	expect(t, r, 201)
	return w, r.body["token"].(string)
}

func TestAttendSendsFreelyAsAttendant(t *testing.T) {
	w, tok := attendWorld(t)
	if !toolNames(w, tok)["send_as_attendant"] {
		t.Fatal("token attend sem a ferramenta")
	}
	// Cliente acabou de escrever, e a conversa está com uma pessoa: o atendente autônomo responde mesmo assim.
	w.receive("Cliente Atendido", "Oi, preciso do boleto https://x.com 123456789012")
	c := mustConv(t, w.admin, "Cliente Atendido")
	convID := c["id"].(string)
	expect(t, w.send(w.admin, convID, "Já te atendo."), 201) // a conversa passa a ter responsável
	before := len(w.e.wa.Sent())
	out, isErr := w.tool(tok, "send_as_attendant", map[string]any{"conversation_id": convID, "text": "Olá! Segue o link https://gov.br/das e o número 123456789012. *Qualquer dúvida*, me chame."})
	if isErr || out["ok"] != true || out["status"] != "enviado" {
		t.Fatalf("envio: %v", out)
	}
	sent := w.e.wa.Sent()
	if len(sent) != before+1 || !strings.HasPrefix(sent[len(sent)-1].Text, "*Assistente do escritório:*") || !strings.Contains(sent[len(sent)-1].Text, "https://gov.br/das") {
		t.Fatalf("enviado = %+v", sent[len(sent)-1])
	}
	var marked bool
	for _, m := range msgList(t, w.admin, convID) {
		if m["automated"] == true && m["aiMode"] == "attend" && m["aiProvider"] == "grok" {
			marked = true
		}
	}
	if !marked {
		t.Fatal("mensagem sem a marca de atendimento por IA")
	}
	// Por contato, sem conversa aberta.
	cid := c["contact"].(map[string]any)["id"].(string)
	out, _ = w.tool(tok, "send_as_attendant", map[string]any{"contact_id": cid, "text": "Mais uma informação."})
	if out["ok"] != true {
		t.Fatalf("por contato: %v", out)
	}
	// Sem destino.
	if _, isErr := w.tool(tok, "send_as_attendant", map[string]any{"text": "oi"}); !isErr {
		t.Fatal("sem destino deveria dar erro")
	}
}

func TestAttendKeepsOnlyEssentialRails(t *testing.T) {
	w, tok := attendWorld(t)
	code := func(out map[string]any) string { c, _ := out["codigo"].(string); return c }
	w.receive("Cliente Parar", "oi")
	c := mustConv(t, w.admin, "Cliente Parar")
	convID := c["id"].(string)
	// PARAR vale para o atendente autônomo também.
	w.receiveFrom(phoneOf(t, w, "Cliente Parar"), "Cliente Parar", "PARAR")
	eventually(t, "opt-out", func() bool {
		out, _ := w.tool(tok, "send_as_attendant", map[string]any{"conversation_id": convID, "text": "olá"})
		return code(out) == "opt_out"
	})
	// Pausa geral.
	w.receive("Cliente Pausa", "oi")
	pid := mustConv(t, w.admin, "Cliente Pausa")["id"].(string)
	expect(t, w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": true, "reason": "teste"}), 204)
	if out, _ := w.tool(tok, "send_as_attendant", map[string]any{"conversation_id": pid, "text": "olá"}); code(out) != "paused" {
		t.Fatalf("pausa: %v", out)
	}
	expect(t, w.admin.do("POST", "/api/v1/automation/pause", map[string]any{"paused": false, "reason": ""}), 204)
	// Chave mestra desligada: a ferramenta some e o envio é recusado.
	expect(t, w.admin.do("PUT", "/api/v1/ai/settings", map[string]any{"enabled": true, "maskDocuments": true, "allowWrite": true, "allowSend": true, "allowAutomation": false, "allowAttend": false}), 200)
	w.expectDenied(tok, "send_as_attendant", map[string]any{"conversation_id": pid, "text": "olá"}, "administrador")
}
