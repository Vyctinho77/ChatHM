package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Adaptador do Asaas (API v3). Só leitura: lista e consulta cobranças. Nunca cria, altera nem cancela nada no Asaas.

const (
	asaasProdBase    = "https://api.asaas.com/v3"
	asaasSandboxBase = "https://api-sandbox.asaas.com/v3"
	asaasPageSize    = 100
	asaasMaxPages    = 30 // até 3000 cobranças por status por sincronização
	asaasMaxBody     = 8 << 20
)

type asaas struct {
	key  string
	base string
	hc   *http.Client
}

func newAsaas(cfg GatewayConfig, hc *http.Client) (Gateway, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("informe a chave de API do Asaas")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 25 * time.Second}
	}
	base := asaasProdBase
	if cfg.Sandbox {
		base = asaasSandboxBase
	}
	return &asaas{key: strings.TrimSpace(cfg.APIKey), base: base, hc: hc}, nil
}

type asaasPayment struct {
	ID          string  `json:"id"`
	Customer    string  `json:"customer"`
	Value       float64 `json:"value"`
	BillingType string  `json:"billingType"`
	Status      string  `json:"status"`
	DueDate     string  `json:"dueDate"`
	Description string  `json:"description"`
	InvoiceURL  string  `json:"invoiceUrl"`
	BankSlipURL string  `json:"bankSlipUrl"`
	Deleted     bool    `json:"deleted"`
}

type asaasCustomer struct {
	Name        string `json:"name"`
	MobilePhone string `json:"mobilePhone"`
	Phone       string `json:"phone"`
}

func (a *asaas) get(ctx context.Context, path string, q url.Values, out any) error {
	u := a.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("access_token", a.key)
	req.Header.Set("User-Agent", "ChatHM")
	req.Header.Set("Accept", "application/json")
	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("asaas: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, asaasMaxBody))
	if err != nil {
		return fmt.Errorf("asaas: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrGatewayAuth
	case resp.StatusCode == http.StatusNotFound:
		return ErrChargeGone
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("asaas respondeu %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("asaas: resposta inválida: %w", err)
	}
	return nil
}

// mapAsaasStatus: PENDING e AWAITING_RISK_ANALYSIS esperam pagamento; OVERDUE venceu; RECEIVED/CONFIRMED/RECEIVED_IN_CASH foram pagas;
// o resto (reembolso, estorno, chargeback...) nunca é tratado como cobrança em aberto.
func mapAsaasStatus(s string) string {
	switch s {
	case "PENDING", "AWAITING_RISK_ANALYSIS":
		return ChargeOpen
	case "OVERDUE":
		return ChargeOverdue
	case "RECEIVED", "CONFIRMED", "RECEIVED_IN_CASH", "DUNNING_RECEIVED":
		return ChargePaid
	default:
		return ChargeCancelled
	}
}

func (p asaasPayment) toCharge(c asaasCustomer) (GatewayCharge, error) {
	due, err := time.Parse("2006-01-02", p.DueDate)
	if err != nil {
		return GatewayCharge{}, fmt.Errorf("asaas: vencimento inválido em %s", p.ID)
	}
	link := p.InvoiceURL
	if link == "" {
		link = p.BankSlipURL
	}
	phone := c.MobilePhone
	if phone == "" {
		phone = c.Phone
	}
	status := mapAsaasStatus(p.Status)
	if p.Deleted {
		status = ChargeCancelled
	}
	return GatewayCharge{
		ExternalID: p.ID, CustomerRef: p.Customer, CustomerName: c.Name, CustomerPhone: phone,
		Description: p.Description, AmountCents: int64(math.Round(p.Value * 100)), DueDate: due, Status: status,
		PaymentURL: link, BillingType: p.BillingType,
	}, nil
}

func (a *asaas) customer(ctx context.Context, cache map[string]asaasCustomer, id string) asaasCustomer {
	if c, ok := cache[id]; ok || id == "" {
		return c
	}
	var c asaasCustomer
	// Sem os dados do cliente a cobrança entra sem vínculo; a equipe vincula na tela.
	_ = a.get(ctx, "/customers/"+url.PathEscape(id), nil, &c)
	cache[id] = c
	return c
}

func (a *asaas) List(ctx context.Context) ([]GatewayCharge, error) {
	cache := map[string]asaasCustomer{}
	var out []GatewayCharge
	for _, status := range []string{"PENDING", "OVERDUE"} {
		for page := 0; page < asaasMaxPages; page++ {
			var resp struct {
				HasMore bool           `json:"hasMore"`
				Data    []asaasPayment `json:"data"`
			}
			q := url.Values{"status": {status}, "limit": {fmt.Sprint(asaasPageSize)}, "offset": {fmt.Sprint(page * asaasPageSize)}}
			if err := a.get(ctx, "/payments", q, &resp); err != nil {
				return nil, err
			}
			for _, p := range resp.Data {
				ch, err := p.toCharge(a.customer(ctx, cache, p.Customer))
				if err != nil {
					continue
				}
				out = append(out, ch)
			}
			if !resp.HasMore {
				break
			}
		}
	}
	return out, nil
}

func (a *asaas) Get(ctx context.Context, externalID string) (GatewayCharge, error) {
	var p asaasPayment
	if err := a.get(ctx, "/payments/"+url.PathEscape(externalID), nil, &p); err != nil {
		return GatewayCharge{}, err
	}
	return p.toCharge(a.customer(ctx, map[string]asaasCustomer{}, p.Customer))
}
