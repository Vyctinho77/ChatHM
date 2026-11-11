package automation

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"
)

// Cobranças de gateways de pagamento (docs/mcp-automacao.md, seção 14).
//
// Cada gateway é um adaptador pequeno que implementa Gateway. O resto do sistema só conhece GatewayCharge, então trocar ou somar
// gateways (Asaas, Mercado Pago, Pagar.me, Iugu...) é escrever um adaptador e registrá-lo em providers. Quem não tem adaptador
// usa a importação (CSV/JSON), que alimenta as mesmas tabelas com o provedor "manual".

// Status normalizados de uma cobrança.
const (
	ChargeOpen      = "open"
	ChargeOverdue   = "overdue"
	ChargePaid      = "paid"
	ChargeCancelled = "cancelled"
)

// ProviderManual: cobranças que entram por importação, sem gateway conectado.
const ProviderManual = "manual"

// GatewayCharge: uma cobrança como o gateway a descreve, já normalizada.
type GatewayCharge struct {
	ExternalID    string
	CustomerRef   string
	CustomerName  string
	CustomerPhone string // como o gateway informa; o sistema reduz a dígitos
	Description   string
	AmountCents   int64
	DueDate       time.Time
	Status        string // ChargeOpen | ChargeOverdue | ChargePaid | ChargeCancelled
	PaymentURL    string
	BillingType   string
}

// GatewayConfig: o que o administrador informa para conectar um gateway.
type GatewayConfig struct {
	APIKey  string `json:"apiKey"`
	Sandbox bool   `json:"sandbox"`
}

// Gateway é o contrato de um adaptador.
type Gateway interface {
	// List devolve TODAS as cobranças em aberto ou vencidas (a sincronização compara com o que já se conhece).
	List(ctx context.Context) ([]GatewayCharge, error)
	// Get devolve o estado atual de uma cobrança (conferido na hora de enviar, para nunca cobrar quem já pagou).
	Get(ctx context.Context, externalID string) (GatewayCharge, error)
}

// ProviderInfo registra um gateway disponível.
type ProviderInfo struct {
	Key   string
	Label string
	// TrustedHosts: domínios onde os links de pagamento deste gateway ficam. O servidor só envia link desses domínios
	// (ou dos que o administrador liberou em Controles).
	TrustedHosts []string
	New          func(cfg GatewayConfig, hc *http.Client) (Gateway, error)
}

var providers = map[string]ProviderInfo{
	"asaas": {Key: "asaas", Label: "Asaas", TrustedHosts: []string{"asaas.com"}, New: newAsaas},
}

// Providers lista os gateways disponíveis, em ordem estável.
func Providers() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

var (
	// ErrGatewayAuth: a chave de API foi recusada pelo gateway.
	ErrGatewayAuth = errors.New("gateway: chave de API recusada")
	// ErrChargeGone: a cobrança não existe mais no gateway.
	ErrChargeGone = errors.New("gateway: cobrança não encontrada")
)
