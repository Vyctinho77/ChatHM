package automation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMoney(t *testing.T) {
	for in, want := range map[string]int64{"150": 15000, "150,00": 15000, "150.00": 15000, "1.234,56": 123456, "R$ 99,9": 9990, "0,50": 50, ".5": 50} {
		if got, ok := ParseMoney(in); !ok || got != want {
			t.Errorf("ParseMoney(%q) = %d, %v; quer %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "0", "0,00", "1,234", "1.2.3,4,5"} {
		if got, ok := ParseMoney(in); ok {
			t.Errorf("ParseMoney(%q) aceito: %d", in, got)
		}
	}
}

func TestFormatBRL(t *testing.T) {
	for in, want := range map[int64]string{5: "R$ 0,05", 15000: "R$ 150,00", 123456: "R$ 1.234,56", 100000000: "R$ 1.000.000,00"} {
		if got := formatBRL(in); got != want {
			t.Errorf("formatBRL(%d) = %q; quer %q", in, got, want)
		}
	}
}

func TestPhoneKey(t *testing.T) {
	for in, want := range map[string]string{"5592991234567": "92/91234567", "92991234567": "92/91234567", "559291234567": "92/91234567", "55999999999": "55/99999999"} {
		ddd, last := phoneKey(in)
		if got := ddd + "/" + last; got != want {
			t.Errorf("phoneKey(%q) = %q; quer %q", in, got, want)
		}
	}
	if d, l := phoneKey("12345"); d != "" || l != "" {
		t.Errorf("número curto aceito: %q %q", d, l)
	}
}

func TestChargeVariables(t *testing.T) {
	fields := map[string]string{"pix_escritorio": "chave"}
	text := "Olá, {nome}! {valor} vence em {vencimento}: {link_cobranca}"
	if _, ref := ValidateTextCharge(text, nil, fields, false); ref == nil {
		t.Fatal("variáveis da cobrança aceitas sem charge_id")
	}
	if _, ref := ValidateTextCharge(text, nil, fields, true); ref != nil {
		t.Fatalf("recusado com charge_id: %v", ref.Message)
	}
	if _, ref := ValidateText("Pague em https://golpe.com/x", nil, fields); ref == nil {
		t.Fatal("link digitado aceito")
	}
	if flags, _ := ValidateTextCharge("Pague R$ 150 hoje: {link_cobranca}", nil, fields, true); !contains(flags, "valor_digitado") {
		t.Fatalf("valor digitado sem aviso: %v", flags)
	}
	if _, err := CleanFields(map[string]string{"valor": "x"}); err == nil {
		t.Fatal("campo fixo com nome reservado aceito")
	}
	cv := &ChargeVars{Link: "https://www.asaas.com/i/1", Amount: "R$ 150,00", Due: "20/11/2026"}
	out := RenderCharge(text, "Maria Souza", fields, cv, false)
	for _, want := range []string{"Olá, Maria!", "R$ 150,00", "20/11/2026", "https://www.asaas.com/i/1", "PARAR"} {
		if !strings.Contains(out, want) {
			t.Errorf("texto sem %q: %q", want, out)
		}
	}
	if masked := RenderCharge(text, "Maria", fields, cv, true); strings.Contains(masked, "asaas.com") {
		t.Errorf("prévia mascarada vaza o link: %q", masked)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestParseChargesCSV(t *testing.T) {
	csv := "\xef\xbb\xbfCódigo;Telefone;Cliente;Descrição;Valor;Vencimento;Situação;Link\n" +
		"A1;(92) 99123-4567;Maria;Honorários 10/2026;\"1.234,56\";20/11/2026;aberto;https://pay.example.com/a1\n" +
		"A2;;José;;abc;20/11/2026;;https://pay.example.com/a2\n" +
		"A3;;Ana;;50,00;2026-12-01;pago;\n"
	rows, errs, err := ParseChargesCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(errs) != 1 || !strings.Contains(errs[0], "linha 3") {
		t.Fatalf("rows=%d errs=%v", len(rows), errs)
	}
	if r := rows[0]; r.ExternalID != "A1" || r.AmountCents != 123456 || r.Status != ChargeOpen || r.Phone != "(92) 99123-4567" || r.DueDate.Format("2006-01-02") != "2026-11-20" {
		t.Fatalf("linha 1 = %+v", r)
	}
	if rows[1].Status != ChargePaid {
		t.Fatalf("situação = %q", rows[1].Status)
	}
	if _, _, err := ParseChargesCSV(strings.NewReader("a;b\n1;2\n")); err == nil {
		t.Fatal("planilha sem as colunas obrigatórias aceita")
	}
}

func TestAsaasAdapter(t *testing.T) {
	var gotKey, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotUA = r.Header.Get("access_token"), r.Header.Get("User-Agent")
		if gotKey != "k-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/v3/payments" && r.URL.Query().Get("status") == "PENDING":
			if r.URL.Query().Get("offset") == "0" {
				_, _ = w.Write([]byte(`{"hasMore":true,"data":[{"id":"pay_1","customer":"cus_1","value":150.5,"status":"PENDING","dueDate":"2026-11-20","description":"Honorários","invoiceUrl":"https://www.asaas.com/i/1","billingType":"BOLETO"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"hasMore":false,"data":[{"id":"pay_2","customer":"cus_1","value":99,"status":"AWAITING_RISK_ANALYSIS","dueDate":"2026-11-25","bankSlipUrl":"https://www.asaas.com/b/2"}]}`))
			}
		case r.URL.Path == "/v3/payments" && r.URL.Query().Get("status") == "OVERDUE":
			_, _ = w.Write([]byte(`{"hasMore":false,"data":[{"id":"pay_3","customer":"cus_2","value":10,"status":"OVERDUE","dueDate":"2026-10-01","invoiceUrl":"https://www.asaas.com/i/3"}]}`))
		case r.URL.Path == "/v3/payments/pay_9":
			_, _ = w.Write([]byte(`{"id":"pay_9","customer":"cus_1","value":150.5,"status":"RECEIVED","dueDate":"2026-11-20","invoiceUrl":"https://www.asaas.com/i/9"}`))
		case r.URL.Path == "/v3/payments/gone":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/v3/customers/cus_1":
			_, _ = w.Write([]byte(`{"name":"Maria","mobilePhone":"92991234567","phone":"9232334455"}`))
		case r.URL.Path == "/v3/customers/cus_2":
			_, _ = w.Write([]byte(`{"name":"José","phone":"9232334455"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	gw, err := newAsaas(GatewayConfig{APIKey: "k-123"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	gw.(*asaas).base = srv.URL + "/v3"
	list, err := gw.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || gotUA != "ChatHM" {
		t.Fatalf("list=%d ua=%q", len(list), gotUA)
	}
	if c := list[0]; c.ExternalID != "pay_1" || c.AmountCents != 15050 || c.Status != ChargeOpen || c.CustomerPhone != "92991234567" || c.CustomerName != "Maria" || c.PaymentURL != "https://www.asaas.com/i/1" {
		t.Fatalf("pay_1 = %+v", c)
	}
	if list[1].PaymentURL != "https://www.asaas.com/b/2" || list[1].Status != ChargeOpen {
		t.Fatalf("pay_2 = %+v", list[1])
	}
	if list[2].Status != ChargeOverdue || list[2].CustomerPhone != "9232334455" {
		t.Fatalf("pay_3 = %+v", list[2])
	}
	if c, err := gw.Get(context.Background(), "pay_9"); err != nil || c.Status != ChargePaid {
		t.Fatalf("pay_9 = %+v, %v", c, err)
	}
	if _, err := gw.Get(context.Background(), "gone"); !errors.Is(err, ErrChargeGone) {
		t.Fatalf("gone: %v", err)
	}
	bad, _ := newAsaas(GatewayConfig{APIKey: "errada"}, srv.Client())
	bad.(*asaas).base = srv.URL + "/v3"
	if _, err := bad.List(context.Background()); !errors.Is(err, ErrGatewayAuth) {
		t.Fatalf("chave errada: %v", err)
	}
	if _, err := newAsaas(GatewayConfig{}, nil); err == nil {
		t.Fatal("sem chave aceito")
	}
}

func TestAsaasStatusMapping(t *testing.T) {
	for in, want := range map[string]string{"PENDING": ChargeOpen, "OVERDUE": ChargeOverdue, "RECEIVED": ChargePaid, "CONFIRMED": ChargePaid, "REFUNDED": ChargeCancelled, "CHARGEBACK_REQUESTED": ChargeCancelled, "": ChargeCancelled} {
		if got := mapAsaasStatus(in); got != want {
			t.Errorf("mapAsaasStatus(%q) = %q; quer %q", in, got, want)
		}
	}
	p := asaasPayment{ID: "x", Status: "PENDING", DueDate: "2026-11-20", Deleted: true, Value: 1}
	if c, _ := p.toCharge(asaasCustomer{}); c.Status != ChargeCancelled {
		t.Errorf("cobrança excluída ficou %q", c.Status)
	}
}
