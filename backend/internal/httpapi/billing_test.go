package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/automation"
)

// fakeGateway: um gateway de pagamento de mentira que o teste controla.
type fakeGateway struct {
	mu      sync.Mutex
	charges map[string]automation.GatewayCharge
}

func (f *fakeGateway) List(context.Context) ([]automation.GatewayCharge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []automation.GatewayCharge
	for _, c := range f.charges {
		if c.Status == automation.ChargeOpen || c.Status == automation.ChargeOverdue {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeGateway) Get(_ context.Context, id string) (automation.GatewayCharge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.charges[id]
	if !ok {
		return automation.GatewayCharge{}, automation.ErrChargeGone
	}
	return c, nil
}

func (f *fakeGateway) set(id string, fn func(*automation.GatewayCharge)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.charges[id]
	fn(&c)
	f.charges[id] = c
}

type deniedGateway struct{}

func (deniedGateway) List(context.Context) ([]automation.GatewayCharge, error) {
	return nil, automation.ErrGatewayAuth
}
func (deniedGateway) Get(context.Context, string) (automation.GatewayCharge, error) {
	return automation.GatewayCharge{}, automation.ErrGatewayAuth
}

func newCharge(id, name, phone string, cents int64) automation.GatewayCharge {
	return automation.GatewayCharge{
		ExternalID: id, CustomerName: name, CustomerPhone: phone, Description: "Honorários 10/2026", AmountCents: cents,
		DueDate: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Status: automation.ChargeOpen, PaymentURL: "https://www.asaas.com/i/" + id,
	}
}

// billingWorld: automação ligada, assunto "cobranca" liberado, gateway "asaas" falso conectado.
func billingWorld(t *testing.T, fake *fakeGateway) (*world, string) {
	t.Helper()
	w, tok := autoWorld(t)
	expect(t, w.admin.do("PUT", "/api/v1/automation/categories/cobranca", map[string]any{
		"label": "Cobrança de honorários", "enabled": true, "level": 1, "windowStart": "08:00", "windowEnd": "18:00",
		"weekdays": []int{1, 2, 3, 4, 5}, "minIntervalDays": 7, "guidelines": "Sem pressão.",
	}), 200)
	w.e.svcs.Automation.SetGatewayFactory(func(provider string, cfg automation.GatewayConfig) (automation.Gateway, error) {
		if cfg.APIKey != "chave-boa" {
			return deniedGateway{}, nil // como o Asaas real: aceita montar e recusa na primeira chamada
		}
		return fake, nil
	})
	return w, tok
}

// clientWithPhone cria um cliente com telefone conhecido e conversa antiga (sem diálogo ativo).
func (w *world) clientWithPhone(name, phone string) string {
	w.receiveFrom(phone, name, "Bom dia")
	c := mustConv(w.e.t, w.admin, name)
	if _, err := w.e.pool.Exec(w.e.t.Context(), `UPDATE messages SET sent_at = $2::timestamptz - interval '48 hours' WHERE conversation_id = $1`, c["id"], autoNow); err != nil {
		w.e.t.Fatal(err)
	}
	return c["contact"].(map[string]any)["id"].(string)
}

func chargeIDs(t *testing.T, w *world, query string) map[string]string { // external id -> id
	t.Helper()
	r := w.admin.do("GET", "/api/v1/automation/charges"+query, nil)
	expect(t, r, 200)
	out := map[string]string{}
	for _, x := range r.body["charges"].([]any) {
		m := x.(map[string]any)
		out[m["description"].(string)+"|"+m["customerName"].(string)] = m["id"].(string)
	}
	return out
}

func sendByStatus(t *testing.T, w *world, status string) []map[string]any {
	t.Helper()
	r := w.admin.do("GET", "/api/v1/automation/sends?status="+status, nil)
	expect(t, r, 200)
	var out []map[string]any
	for _, x := range r.body["sends"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func resetPacing(t *testing.T, w *world) {
	t.Helper()
	if _, err := w.e.pool.Exec(t.Context(), `UPDATE automation_state SET next_allowed_at = 'epoch'`); err != nil {
		t.Fatal(err)
	}
}

const chargeText = "Olá, {nome}! A cobrança de {valor} vence em {vencimento}. Pague por aqui: {link_cobranca}"

func queueCharge(w *world, tok, cid, chargeID, key string) map[string]any {
	args := map[string]any{"contact_id": cid, "category": "cobranca", "reason": "Honorários 10/2026 vencem em 20/11", "text": chargeText, "dedupe_key": key}
	if chargeID != "" {
		args["charge_id"] = chargeID
	}
	out, _ := w.tool(tok, "send_automatic_message", args)
	return out
}

func TestBillingGatewayAndChargeFlow(t *testing.T) {
	fake := &fakeGateway{charges: map[string]automation.GatewayCharge{
		"pay_maria": newCharge("pay_maria", "Maria", "(92) 99123-4567", 15000),
		"pay_ghost": newCharge("pay_ghost", "Desconhecido", "(11) 98888-7777", 9990),
	}}
	w, tok := billingWorld(t, fake)
	maria := w.clientWithPhone("Cliente Maria", "5592991234567")
	code := func(out map[string]any) string { c, _ := out["codigo"].(string); return c }

	// Conexão: chave errada é recusada na sincronização; a chave nunca volta na listagem.
	expect(t, w.ana.do("PUT", "/api/v1/automation/gateways/asaas", map[string]any{"apiKey": "x", "enabled": true}), 403)
	expect(t, w.admin.do("PUT", "/api/v1/automation/gateways/asaas", map[string]any{"enabled": true}), 422) // sem chave
	expect(t, w.admin.do("PUT", "/api/v1/automation/gateways/inexistente", map[string]any{"apiKey": "x", "enabled": true}), 404)
	expect(t, w.admin.do("PUT", "/api/v1/automation/gateways/asaas", map[string]any{"apiKey": "chave-errada", "enabled": true}), 204)
	expect(t, w.admin.do("POST", "/api/v1/automation/gateways/asaas/sync", nil), 502)
	expect(t, w.admin.do("PUT", "/api/v1/automation/gateways/asaas", map[string]any{"apiKey": "chave-boa", "enabled": true}), 204)
	g := w.admin.do("GET", "/api/v1/automation/gateways", nil)
	expect(t, g, 200)
	if raw, _ := json.Marshal(g.body); strings.Contains(string(raw), "chave-boa") || !strings.Contains(string(raw), `"configured":true`) {
		t.Fatalf("gateways: %s", raw)
	}
	var stored []byte
	if err := w.e.pool.QueryRow(t.Context(), `SELECT config_enc FROM billing_gateways WHERE provider = 'asaas'`).Scan(&stored); err != nil || strings.Contains(string(stored), "chave-boa") {
		t.Fatalf("chave guardada em texto puro: %v", err)
	}

	// Sincronização: a cobrança de Maria é vinculada pelo telefone; a do desconhecido fica sem cliente.
	s := w.admin.do("POST", "/api/v1/automation/gateways/asaas/sync", nil)
	expect(t, s, 200)
	if s.body["seen"] != float64(2) || s.body["linked"] != float64(1) || s.body["unlinked"] != float64(1) {
		t.Fatalf("sync = %v", s.body)
	}
	ids := chargeIDs(t, w, "")
	mariaCharge, ghostCharge := ids["Honorários 10/2026|Maria"], ids["Honorários 10/2026|Desconhecido"]
	if mariaCharge == "" || ghostCharge == "" {
		t.Fatalf("cobranças = %v", ids)
	}
	if n := len(chargeIDs(t, w, "?unlinked=1")); n != 1 {
		t.Fatalf("sem vínculo = %d", n)
	}

	// A IA vê só o que está vinculado e nunca o link de pagamento.
	lc, isErr := w.tool(tok, "list_charges", map[string]any{})
	if isErr || lc["total"] != float64(1) {
		t.Fatalf("list_charges = %v", lc)
	}
	if raw, _ := json.Marshal(lc); strings.Contains(string(raw), "asaas.com") || strings.Contains(string(raw), "9123") {
		t.Fatalf("list_charges vaza dados: %s", raw)
	}

	// Recusas do servidor.
	if c := code(queueCharge(w, tok, maria, "", "sem-charge")); c != "content" {
		t.Fatalf("variáveis sem charge_id = %q", c)
	}
	if c := code(queueCharge(w, tok, maria, ghostCharge, "fantasma")); c != "charge_owner" {
		t.Fatalf("cobrança sem cliente = %q", c)
	}
	typed := map[string]any{"contact_id": maria, "category": "cobranca", "reason": "Honorários 10/2026", "dedupe_key": "digitado", "charge_id": mariaCharge,
		"text": "Pague em https://www.asaas.com/i/pay_maria que é mais rápido."}
	if out, _ := w.tool(tok, "send_automatic_message", typed); code(out) != "content" {
		t.Fatalf("link digitado aceito: %v", out)
	}
	das := map[string]any{"contact_id": maria, "category": "das", "reason": "DAS 10/2026", "dedupe_key": "das", "charge_id": mariaCharge, "text": chargeText}
	if out, _ := w.tool(tok, "send_automatic_message", das); code(out) != "charge" {
		t.Fatalf("cobrança em outro assunto: %v", out)
	}
	other := w.clientWithPhone("Cliente Outro", "5592988887777")
	if c := code(queueCharge(w, tok, other, mariaCharge, "alheia")); c != "charge_owner" {
		t.Fatalf("cobrança de outro cliente = %q", c)
	}

	// Caminho feliz: a IA pede, nada sai sem aprovação, e o link real só entra na hora do envio.
	q := queueCharge(w, tok, maria, mariaCharge, "honorarios-2026-10")
	if q["ok"] != true || strings.Contains(q["previa"].(string), "asaas.com") || !strings.Contains(q["previa"].(string), "R$ 150,00") {
		t.Fatalf("enqueue = %v", q)
	}
	before := len(w.e.wa.Sent())
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 0 || len(w.e.wa.Sent()) != before {
		t.Fatalf("saiu sem aprovação")
	}
	pend := sendByStatus(t, w, "pending_approval")
	if len(pend) != 1 || !strings.Contains(pend[0]["preview"].(string), "https://www.asaas.com/i/pay_maria") || !strings.Contains(pend[0]["chargeLabel"].(string), "R$ 150,00") {
		t.Fatalf("prévia do administrador = %v", pend)
	}
	expect(t, w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": []string{q["id"].(string)}}), 200)
	if n := w.e.svcs.Automation.Tick(t.Context()); n != 1 {
		t.Fatalf("Tick = %d", n)
	}
	sent := w.e.wa.Sent()
	txt := sent[len(sent)-1].Text
	for _, want := range []string{"Olá, Cliente!", "R$ 150,00", "20/11/2026", "https://www.asaas.com/i/pay_maria", "PARAR"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("texto enviado sem %q: %q", want, txt)
		}
	}
	if strings.Contains(txt, "{") {
		t.Fatalf("variável sem substituir: %q", txt)
	}

	// A equipe vincula a cobrança do desconhecido a outro cliente; o vínculo manual sobrevive à próxima sincronização.
	expect(t, w.admin.do("PUT", "/api/v1/automation/charges/"+ghostCharge+"/contact", map[string]any{"contactId": other}), 204)
	expect(t, w.admin.do("POST", "/api/v1/automation/gateways/asaas/sync", nil), 200)
	if n := len(chargeIDs(t, w, "?unlinked=1")); n != 0 {
		t.Fatalf("vínculo manual perdido: %d", n)
	}
	expect(t, w.admin.do("PUT", "/api/v1/automation/charges/"+ghostCharge+"/contact", map[string]any{"contactId": "00000000-0000-0000-0000-000000000000"}), 422)
}

// Quem pagou entre a aprovação e o envio nunca recebe a cobrança; valor alterado também trava.
func TestBillingSendTimeChecks(t *testing.T) {
	fake := &fakeGateway{charges: map[string]automation.GatewayCharge{
		"pay_a": newCharge("pay_a", "Ana", "92991110001", 10000),
		"pay_b": newCharge("pay_b", "Beto", "92991110002", 20000),
		"pay_c": newCharge("pay_c", "Caio", "92991110003", 30000),
	}}
	w, tok := billingWorld(t, fake)
	ana := w.clientWithPhone("Cliente Ana", "5592991110001")
	beto := w.clientWithPhone("Cliente Beto", "5592991110002")
	caio := w.clientWithPhone("Cliente Caio", "5592991110003")
	expect(t, w.admin.do("PUT", "/api/v1/automation/gateways/asaas", map[string]any{"apiKey": "chave-boa", "enabled": true}), 204)
	expect(t, w.admin.do("POST", "/api/v1/automation/gateways/asaas/sync", nil), 200)
	ids := chargeIDs(t, w, "")
	byName := func(n string) string { return ids["Honorários 10/2026|"+n] }

	var sendIDs []string
	for _, c := range []struct{ cid, name string }{{ana, "Ana"}, {beto, "Beto"}, {caio, "Caio"}} {
		q := queueCharge(w, tok, c.cid, byName(c.name), "h-"+c.name)
		if q["ok"] != true {
			t.Fatalf("%s: %v", c.name, q)
		}
		sendIDs = append(sendIDs, q["id"].(string))
	}
	expect(t, w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": sendIDs}), 200)

	// Ana pagou; o valor do Beto mudou; o do Caio segue igual.
	fake.set("pay_a", func(c *automation.GatewayCharge) { c.Status = automation.ChargePaid })
	fake.set("pay_b", func(c *automation.GatewayCharge) { c.AmountCents = 25000 })

	before := len(w.e.wa.Sent())
	for i := 0; i < 3; i++ {
		resetPacing(t, w)
		w.e.svcs.Automation.Tick(t.Context())
	}
	sent := w.e.wa.Sent()
	if len(sent) != before+1 || !strings.Contains(sent[len(sent)-1].Text, "R$ 300,00") {
		t.Fatalf("deveria sair só a do Caio: %d novas", len(sent)-before)
	}
	skipped := sendByStatus(t, w, "skipped")
	if len(skipped) != 2 {
		t.Fatalf("puladas = %d", len(skipped))
	}
	var paid, changed bool
	for _, s := range skipped {
		d := s["statusDetail"].(string)
		paid = paid || strings.Contains(d, "já foi paga")
		changed = changed || strings.Contains(d, "mudou depois da aprovação")
	}
	if !paid || !changed {
		t.Fatalf("motivos = %v", skipped)
	}

	// Cobrança paga não pode mais ser pedida.
	if out := queueCharge(w, tok, ana, byName("Ana"), "de-novo"); out["codigo"] != "charge_closed" {
		t.Fatalf("cobrança paga aceita: %v", out)
	}
}

func TestBillingImport(t *testing.T) {
	w, tok := billingWorld(t, &fakeGateway{charges: map[string]automation.GatewayCharge{}})
	cid := w.clientWithPhone("Cliente Importado", "5592977776666")
	row := map[string]any{"id": "F-100", "phone": "(92) 97777-6666", "name": "Importado", "description": "Honorários 10/2026", "amount": "1.234,56", "dueDate": "20/11/2026", "url": "https://pay.example.com/f100"}

	// O domínio do link precisa estar liberado em Controles.
	r := w.admin.do("POST", "/api/v1/automation/charges/import", map[string]any{"charges": []any{row}})
	expect(t, r, 200)
	if r.body["created"] != float64(0) || len(r.body["errors"].([]any)) != 1 {
		t.Fatalf("import sem domínio = %v", r.body)
	}
	st := w.admin.do("GET", "/api/v1/automation/settings", nil)
	expect(t, st, 200)
	st.body["allowedDomains"] = []string{"pay.example.com"}
	expect(t, w.admin.do("PUT", "/api/v1/automation/settings", st.body), 200)

	r = w.admin.do("POST", "/api/v1/automation/charges/import", map[string]any{"charges": []any{row}})
	expect(t, r, 200)
	if r.body["created"] != float64(1) || r.body["linked"] != float64(1) {
		t.Fatalf("import = %v", r.body)
	}
	// Reimportar atualiza em vez de duplicar.
	row["amount"] = "1.300,00"
	r = w.admin.do("POST", "/api/v1/automation/charges/import", map[string]any{"charges": []any{row}})
	expect(t, r, 200)
	if r.body["created"] != float64(0) || r.body["updated"] != float64(1) {
		t.Fatalf("reimport = %v", r.body)
	}

	lc, _ := w.tool(tok, "list_charges", map[string]any{"contact_id": cid})
	if lc["total"] != float64(1) || !strings.Contains(toJSON(lc), "R$ 1.300,00") {
		t.Fatalf("list_charges = %v", lc)
	}
	var chargeID string
	for _, c := range lc["charges"].([]any) {
		chargeID = c.(map[string]any)["id"].(string)
	}
	q := queueCharge(w, tok, cid, chargeID, "import-1")
	if q["ok"] != true {
		t.Fatalf("enqueue de cobrança importada = %v", q)
	}
	// Se o administrador tirar o domínio da lista, o link deixa de ser confiável.
	st.body["allowedDomains"] = []string{}
	expect(t, w.admin.do("PUT", "/api/v1/automation/settings", st.body), 200)
	r = w.admin.do("POST", "/api/v1/automation/sends/approve", map[string]any{"ids": []string{q["id"].(string)}})
	expect(t, r, 200)
	if len(r.body["refused"].(map[string]any)) != 1 {
		t.Fatalf("aprovou com domínio removido: %v", r.body)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
