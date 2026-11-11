package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

// Cobranças: a IA nunca escreve link, valor nem vencimento. Ela aponta uma cobrança (charge_id) e o servidor confere de quem é,
// se continua em aberto e de que domínio é o link, e só então preenche {link_cobranca}, {valor} e {vencimento} na hora de enviar.

// SetBilling liga a cifragem das chaves de API dos gateways. Sem isso, só a importação manual funciona.
func (s *Service) SetBilling(box *secretbox.Box) { s.box = box }

// SetGatewayFactory troca a criação dos adaptadores (testes).
func (s *Service) SetGatewayFactory(f func(provider string, cfg GatewayConfig) (Gateway, error)) {
	s.newGateway = f
}

func defaultGateway(provider string, cfg GatewayConfig) (Gateway, error) {
	p, ok := providers[provider]
	if !ok {
		return nil, ErrNotFound
	}
	return p.New(cfg, nil)
}

// ChargeVars: os valores que entram no texto no lugar das variáveis da cobrança.
type ChargeVars struct {
	Link   string
	Amount string // R$ 1.234,56
	Due    string // 20/11/2026
}

// Variáveis reservadas: só existem com charge_id. Campos fixos do escritório não podem usar estes nomes.
var chargeVarNames = []string{"link_cobranca", "valor", "vencimento"}

func isChargeVar(name string) bool {
	for _, n := range chargeVarNames {
		if n == name {
			return true
		}
	}
	return false
}

func formatBRL(cents int64) string {
	s := strconv.FormatInt(cents/100, 10)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return fmt.Sprintf("R$ %s,%02d", strings.Join(parts, "."), cents%100)
}

// --- cobrança no banco ------------------------------------------------------

type chargeRow struct {
	ID          string
	Provider    string
	ExternalID  string
	ContactID   string
	Description string
	AmountCents int64
	DueDate     time.Time
	Status      string
	PaymentURL  string
}

func (c chargeRow) vars() *ChargeVars {
	return &ChargeVars{Link: c.PaymentURL, Amount: formatBRL(c.AmountCents), Due: c.DueDate.Format("02/01/2006")}
}

const selectChargeRow = `SELECT id::text, provider, external_id, coalesce(contact_id::text, ''), description, amount_cents, due_date, status, payment_url FROM billing_charges`

func scanChargeRow(row pgx.Row) (chargeRow, error) {
	var c chargeRow
	err := row.Scan(&c.ID, &c.Provider, &c.ExternalID, &c.ContactID, &c.Description, &c.AmountCents, &c.DueDate, &c.Status, &c.PaymentURL)
	return c, err
}

func (s *Service) loadCharge(ctx context.Context, id string) (chargeRow, error) {
	if !httpx.ValidUUID(id) {
		return chargeRow{}, ErrNotFound
	}
	c, err := scanChargeRow(s.db.QueryRow(ctx, selectChargeRow+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// chargeLinkTrusted: o link é https e de um domínio do gateway da cobrança ou liberado pelo administrador.
func chargeLinkTrusted(c chargeRow, allowed []string) bool {
	if sanitizeURL(c.PaymentURL) == "" {
		return false
	}
	hosts := append([]string{}, allowed...)
	if p, ok := providers[c.Provider]; ok {
		hosts = append(hosts, p.TrustedHosts...)
	}
	return domainAllowed(c.PaymentURL, hosts)
}

// guardCharge: a cobrança existe, é do próprio destinatário, continua em aberto e o link é de um domínio confiável.
// Conferido ao enfileirar, ao aprovar e na hora de enviar.
func (s *Service) guardCharge(ctx context.Context, chargeID, contactID string, st Settings) (chargeRow, *Refusal, error) {
	if !httpx.ValidUUID(chargeID) {
		return chargeRow{}, refuse("charge", "charge_id inválido: use um id de list_charges."), nil
	}
	c, err := s.loadCharge(ctx, chargeID)
	if errors.Is(err, ErrNotFound) {
		return c, refuse("charge", "Essa cobrança não existe (list_charges)."), nil
	}
	if err != nil {
		return c, nil, err
	}
	switch {
	case c.ContactID == "":
		return c, refuse("charge_owner", "Esta cobrança ainda não está vinculada a um cliente do ChatHM. A equipe vincula em Envios automáticos → Cobranças."), nil
	case c.ContactID != contactID:
		return c, refuse("charge_owner", "Essa cobrança é de outro cliente. Só é possível usar a cobrança do próprio destinatário."), nil
	case c.Status == ChargePaid:
		return c, refuse("charge_closed", "Essa cobrança já foi paga: não há o que cobrar."), nil
	case c.Status == ChargeCancelled:
		return c, refuse("charge_closed", "Essa cobrança foi cancelada."), nil
	case !chargeLinkTrusted(c, st.AllowedDomains):
		return c, refuse("charge_link", "O link desta cobrança não é de um domínio confiável do gateway. O administrador pode liberar o domínio em Envios automáticos → Controles."), nil
	}
	return c, nil, nil
}

// sanitizeURL: só https com host; devolve "" para qualquer outra coisa.
func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(raw) > 500 {
		return ""
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return ""
		}
	}
	return raw
}

// refreshCharge consulta o gateway e atualiza a cobrança (valor, vencimento, situação, link). Cobrança importada não tem consulta ao vivo.
func (s *Service) refreshCharge(ctx context.Context, id string) (chargeRow, error) {
	c, err := s.loadCharge(ctx, id)
	if err != nil || c.Provider == ProviderManual {
		return c, err
	}
	gw, err := s.gatewayFor(ctx, c.Provider)
	if err != nil {
		return c, err
	}
	g, err := gw.Get(ctx, c.ExternalID)
	if errors.Is(err, ErrChargeGone) {
		g = GatewayCharge{ExternalID: c.ExternalID, AmountCents: c.AmountCents, DueDate: c.DueDate, Status: ChargeCancelled, PaymentURL: c.PaymentURL}
		err = nil
	}
	if err != nil {
		return c, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE billing_charges SET amount_cents = $2, due_date = $3, status = $4, payment_url = $5, description = $6, synced_at = now() WHERE id = $1`,
		id, positive(g.AmountCents, c.AmountCents), g.DueDate, g.Status, sanitizeURL(g.PaymentURL), trunc(g.Description, 300)); err != nil {
		return c, err
	}
	return s.loadCharge(ctx, id)
}

func positive(v, fallback int64) int64 {
	if v > 0 {
		return v
	}
	return fallback
}

func trunc(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

// --- gateways -------------------------------------------------------------------

type GatewayState struct {
	Provider    string     `json:"provider"`
	Label       string     `json:"label"`
	Configured  bool       `json:"configured"` // há chave salva (nunca devolvida)
	Enabled     bool       `json:"enabled"`
	Sandbox     bool       `json:"sandbox"`
	LastSyncAt  *time.Time `json:"lastSyncAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	TrustedHost []string   `json:"trustedHosts"`
}

func (s *Service) Gateways(ctx context.Context) ([]GatewayState, error) {
	out := []GatewayState{}
	for _, p := range Providers() {
		st := GatewayState{Provider: p.Key, Label: p.Label, TrustedHost: p.TrustedHosts}
		err := s.db.QueryRow(ctx, `SELECT config_enc IS NOT NULL, enabled, sandbox, last_sync_at, last_error FROM billing_gateways WHERE provider = $1`, p.Key).
			Scan(&st.Configured, &st.Enabled, &st.Sandbox, &st.LastSyncAt, &st.LastError)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Service) gatewayFor(ctx context.Context, provider string) (Gateway, error) {
	var enabled, sandbox bool
	var enc []byte
	err := s.db.QueryRow(ctx, `SELECT enabled, sandbox, config_enc FROM billing_gateways WHERE provider = $1`, provider).Scan(&enabled, &sandbox, &enc)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!enabled || len(enc) == 0)) {
		return nil, errors.New("o gateway não está conectado")
	}
	if err != nil {
		return nil, err
	}
	if s.box == nil {
		return nil, errors.New("criptografia indisponível")
	}
	plain, err := s.box.Open(enc, []byte("billing:"+provider))
	if err != nil {
		return nil, errors.New("não foi possível ler a chave salva (ENCRYPTION_KEY mudou?)")
	}
	var cfg GatewayConfig
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, errors.New("configuração do gateway corrompida")
	}
	cfg.Sandbox = sandbox
	f := s.newGateway
	if f == nil {
		f = defaultGateway
	}
	return f(provider, cfg)
}

// SetGateway salva a conexão. apiKey vazia mantém a chave atual. A chave é cifrada e nunca volta.
func (s *Service) SetGateway(ctx context.Context, actorID, ip, provider, apiKey string, sandbox, enabled bool) error {
	if _, ok := providers[provider]; !ok {
		return ErrNotFound
	}
	apiKey = strings.TrimSpace(apiKey)
	if len(apiKey) > 300 || strings.ContainsAny(apiKey, " \r\n\t") {
		return httpx.FieldErrors{"apiKey": "A chave de API não pode ter espaços e tem até 300 caracteres."}
	}
	var hasKey bool
	if err := s.db.QueryRow(ctx, `SELECT coalesce((SELECT config_enc IS NOT NULL FROM billing_gateways WHERE provider = $1), false)`, provider).Scan(&hasKey); err != nil {
		return err
	}
	if enabled && apiKey == "" && !hasKey {
		return httpx.FieldErrors{"apiKey": "Informe a chave de API para conectar."}
	}
	var enc []byte
	if apiKey != "" {
		if s.box == nil {
			return errors.New("criptografia indisponível")
		}
		raw, _ := json.Marshal(GatewayConfig{APIKey: apiKey})
		var err error
		if enc, err = s.box.Seal(raw, []byte("billing:"+provider)); err != nil {
			return err
		}
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing_gateways (provider, enabled, sandbox, config_enc, updated_by) VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid)
			ON CONFLICT (provider) DO UPDATE SET enabled = $2, sandbox = $3, config_enc = coalesce($4, billing_gateways.config_enc),
			       updated_at = now(), updated_by = NULLIF($5, '')::uuid, last_error = '', last_attempt_at = NULL`,
			provider, enabled, sandbox, enc, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "billing.gateway.update", TargetType: "billing_gateway", TargetID: provider, IP: ip,
			Metadata: map[string]any{"enabled": enabled, "sandbox": sandbox, "keyChanged": apiKey != ""}})
	})
}

type SyncResult struct {
	Seen     int `json:"seen"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Closed   int `json:"closed"` // pagas ou canceladas desde a última sincronização
	Linked   int `json:"linked"` // vinculadas a um cliente nesta rodada
	Unlinked int `json:"unlinked"`
}

func gatewayErrText(err error) string {
	if errors.Is(err, ErrGatewayAuth) {
		return "O gateway recusou a chave de API. Confira a chave e o ambiente (produção ou teste)."
	}
	return "Não foi possível consultar o gateway agora."
}

// Sync traz as cobranças em aberto do gateway, vincula cada uma ao cliente (pelo telefone) e fecha as que foram pagas ou canceladas.
func (s *Service) Sync(ctx context.Context, actorID, provider string) (SyncResult, error) {
	var res SyncResult
	gw, err := s.gatewayFor(ctx, provider)
	if err != nil {
		return res, httpx.FieldErrors{"provider": err.Error()}
	}
	_, _ = s.db.Exec(ctx, `UPDATE billing_gateways SET last_attempt_at = now() WHERE provider = $1`, provider)
	list, err := gw.List(ctx)
	if err != nil {
		_, _ = s.db.Exec(ctx, `UPDATE billing_gateways SET last_error = $2 WHERE provider = $1`, provider, gatewayErrText(err))
		s.log.Warn("sincronização de cobranças falhou", "provider", provider, "err", err)
		return res, err
	}
	seen := make([]string, 0, len(list))
	for _, g := range list {
		seen = append(seen, g.ExternalID)
		created, linked, err := s.upsertCharge(ctx, provider, g, "")
		if err != nil {
			return res, err
		}
		res.Seen++
		if created {
			res.Created++
		} else {
			res.Updated++
		}
		if linked {
			res.Linked++
		}
	}
	// O que estava em aberto aqui e sumiu da lista foi pago ou cancelado: confere uma a uma (no máximo 100 por rodada).
	rows, err := s.db.Query(ctx, `SELECT id::text FROM billing_charges WHERE provider = $1 AND status IN ('open', 'overdue') AND NOT (external_id = ANY($2)) LIMIT 100`, provider, seen)
	if err != nil {
		return res, err
	}
	var missing []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			missing = append(missing, id)
		}
	}
	rows.Close()
	for _, id := range missing {
		before, _ := s.loadCharge(ctx, id)
		after, err := s.refreshCharge(ctx, id)
		if err == nil && after.Status != before.Status {
			res.Closed++
		}
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM billing_charges WHERE provider = $1 AND status IN ('open', 'overdue') AND contact_id IS NULL`, provider).Scan(&res.Unlinked)
	_, _ = s.db.Exec(ctx, `UPDATE billing_gateways SET last_sync_at = now(), last_error = '' WHERE provider = $1`, provider)
	_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "billing.sync", TargetType: "billing_gateway", TargetID: provider,
			Metadata: map[string]any{"seen": res.Seen, "created": res.Created, "closed": res.Closed, "linked": res.Linked, "unlinked": res.Unlinked}})
	})
	return res, nil
}

// SyncDue: a cada 15 minutos sincroniza os gateways conectados (e tenta de novo 15 minutos depois de uma falha).
func (s *Service) SyncDue(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT provider FROM billing_gateways WHERE enabled AND config_enc IS NOT NULL AND (last_attempt_at IS NULL OR last_attempt_at < now() - interval '15 minutes')`)
	if err != nil {
		return
	}
	var due []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			due = append(due, p)
		}
	}
	rows.Close()
	for _, p := range due {
		_, _ = s.Sync(ctx, "", p)
	}
}

// upsertCharge grava uma cobrança do gateway. O vínculo com o cliente só é decidido uma vez (telefone) e nunca sobrescreve o da equipe.
func (s *Service) upsertCharge(ctx context.Context, provider string, g GatewayCharge, forcedContact string) (created, linked bool, err error) {
	phone := onlyDigits(g.CustomerPhone)
	status := g.Status
	switch status {
	case ChargeOpen, ChargeOverdue, ChargePaid, ChargeCancelled:
	default:
		status = ChargeCancelled // desconhecido nunca vira cobrança em aberto
	}
	amount := g.AmountCents
	if amount <= 0 {
		amount, status = 1, ChargeCancelled
	}
	var id, contactID string
	var inserted bool
	err = s.db.QueryRow(ctx, `
		INSERT INTO billing_charges (provider, external_id, contact_id, link_manual, customer_ref, customer_name, customer_phone, description, amount_cents, due_date, status, payment_url, billing_type)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $3 <> '', $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (provider, external_id) DO UPDATE SET customer_ref = $4, customer_name = $5, customer_phone = $6, description = $7, amount_cents = $8,
		       due_date = $9, status = $10, payment_url = $11, billing_type = $12, synced_at = now(),
		       contact_id = CASE WHEN $3 <> '' THEN NULLIF($3, '')::uuid ELSE billing_charges.contact_id END,
		       link_manual = billing_charges.link_manual OR $3 <> ''
		RETURNING id::text, coalesce(contact_id::text, ''), (xmax = 0)`,
		provider, trunc(g.ExternalID, 120), forcedContact, trunc(g.CustomerRef, 120), trunc(g.CustomerName, 150), phone, trunc(g.Description, 300),
		amount, g.DueDate, status, sanitizeURL(g.PaymentURL), trunc(g.BillingType, 40)).Scan(&id, &contactID, &inserted)
	if err != nil {
		return false, false, err
	}
	if contactID == "" && phone != "" {
		if m := s.matchContact(ctx, phone); m != "" {
			if _, err := s.db.Exec(ctx, `UPDATE billing_charges SET contact_id = $2 WHERE id = $1 AND contact_id IS NULL`, id, m); err != nil {
				return inserted, false, err
			}
			linked = true
		}
	}
	return inserted, linked, nil
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// phoneKey: DDD + 8 últimos dígitos (ignora o código do país e o nono dígito). Vazio se não parecer um telefone brasileiro.
func phoneKey(digits string) (ddd, last8 string) {
	d := strings.TrimPrefix(digits, "55")
	if len(digits) < 12 { // sem o 55 na frente, o número já vem só com DDD
		d = digits
	}
	if len(d) < 10 || len(d) > 11 {
		return "", ""
	}
	return d[:2], d[len(d)-8:]
}

// matchContact: o único contato (não grupo) com o mesmo DDD e final de telefone. Zero ou mais de um: sem vínculo (a equipe decide).
func (s *Service) matchContact(ctx context.Context, phoneDigits string) string {
	ddd, last8 := phoneKey(phoneDigits)
	if last8 == "" {
		return ""
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, regexp_replace(CASE WHEN phone <> '' THEN phone ELSE split_part(wa_id, '@', 1) END, '\D', '', 'g')
		  FROM contacts WHERE kind <> 'group'
		   AND right(regexp_replace(CASE WHEN phone <> '' THEN phone ELSE split_part(wa_id, '@', 1) END, '\D', '', 'g'), 8) = $1`, last8)
	if err != nil {
		return ""
	}
	defer rows.Close()
	found := ""
	for rows.Next() {
		var id, digits string
		if rows.Scan(&id, &digits) != nil {
			continue
		}
		if d, _ := phoneKey(digits); d != ddd {
			continue
		}
		if found != "" {
			return "" // ambíguo
		}
		found = id
	}
	return found
}

// --- consulta e vínculo ------------------------------------------------------------

// Charge: a visão de uma cobrança para a tela e para a IA. Nunca inclui o link de pagamento nem o telefone do cliente.
type Charge struct {
	ID           string    `json:"id"`
	Provider     string    `json:"provider"`
	ContactID    string    `json:"contactId,omitempty"`
	ContactName  string    `json:"contactName,omitempty"`
	CustomerName string    `json:"customerName"`
	Description  string    `json:"description"`
	AmountCents  int64     `json:"amountCents"`
	Amount       string    `json:"amount"`
	DueDate      string    `json:"dueDate"` // AAAA-MM-DD
	DaysOverdue  int       `json:"daysOverdue"`
	Status       string    `json:"status"`
	BillingType  string    `json:"billingType,omitempty"`
	HasLink      bool      `json:"hasLink"`
	SyncedAt     time.Time `json:"syncedAt"`
}

type ChargeFilter struct {
	Status    string // vazio = todas; "open" = em aberto + vencidas
	ContactID string
	Unlinked  bool
	Provider  string
	Limit     int
}

func (s *Service) Charges(ctx context.Context, f ChargeFilter) ([]Charge, error) {
	if f.Limit < 1 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT b.id::text, b.provider, coalesce(b.contact_id::text, ''), coalesce(nullif(ct.name, ''), ct.phone, ''), b.customer_name, b.description,
		       b.amount_cents, b.due_date, b.status, b.billing_type, b.payment_url <> '', b.synced_at
		  FROM billing_charges b LEFT JOIN contacts ct ON ct.id = b.contact_id
		 WHERE ($1 = '' OR ($1 = 'open' AND b.status IN ('open', 'overdue')) OR b.status = $1)
		   AND ($2 = '' OR b.contact_id::text = $2) AND (NOT $3 OR b.contact_id IS NULL) AND ($4 = '' OR b.provider = $4)
		 ORDER BY b.due_date, b.created_at LIMIT $5`, f.Status, f.ContactID, f.Unlinked, f.Provider, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Charge{}
	today := s.now().UTC().Truncate(24 * time.Hour)
	for rows.Next() {
		var c Charge
		var due time.Time
		if err := rows.Scan(&c.ID, &c.Provider, &c.ContactID, &c.ContactName, &c.CustomerName, &c.Description, &c.AmountCents, &due, &c.Status, &c.BillingType, &c.HasLink, &c.SyncedAt); err != nil {
			return nil, err
		}
		c.Amount = formatBRL(c.AmountCents)
		c.DueDate = due.Format("2006-01-02")
		if (c.Status == ChargeOpen || c.Status == ChargeOverdue) && due.Before(today) {
			c.DaysOverdue = int(today.Sub(due).Hours() / 24)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LinkCharge: a equipe escolhe a quem pertence uma cobrança (contactID vazio desfaz o vínculo).
func (s *Service) LinkCharge(ctx context.Context, actorID, ip, chargeID, contactID string) error {
	if !httpx.ValidUUID(chargeID) || (contactID != "" && !httpx.ValidUUID(contactID)) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if contactID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contacts WHERE id = $1 AND kind <> 'group')`, contactID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return httpx.FieldErrors{"contactId": "Cliente não encontrado."}
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE billing_charges SET contact_id = NULLIF($2, '')::uuid, link_manual = ($2 <> '') WHERE id = $1`, chargeID, contactID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		// O que já estava na fila com o vínculo antigo deixa de valer.
		if _, err := tx.Exec(ctx, `UPDATE automation_sends SET status = 'cancelled', status_detail = 'A cobrança foi vinculada a outro cliente.', updated_at = now()
			 WHERE charge_id = $1 AND status IN ('pending_approval', 'approved') AND contact_id::text <> $2`, chargeID, contactID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "billing.charge.link", TargetType: "billing_charge", TargetID: chargeID, IP: ip, Metadata: map[string]any{"contactId": contactID}})
	})
}

// --- importação (qualquer gateway) -------------------------------------------------

var errTooMany = errors.New("no máximo 2000 cobranças por importação")

type ImportRow struct {
	Line        int
	ExternalID  string
	ContactID   string
	Phone       string
	Name        string
	Description string
	AmountCents int64
	DueDate     time.Time
	Status      string
	URL         string
}

type ImportResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Linked   int      `json:"linked"`
	Unlinked int      `json:"unlinked"`
	Errors   []string `json:"errors"`
}

// ImportCharges grava cobranças de qualquer origem (planilha, outro gateway) como provedor "manual". O link precisa ser https de um domínio
// liberado em Controles: sem isso o servidor não o enviaria de qualquer jeito.
func (s *Service) ImportCharges(ctx context.Context, actorID, ip string, rows []ImportRow) (ImportResult, error) {
	res := ImportResult{Errors: []string{}}
	if len(rows) == 0 {
		return res, httpx.FieldErrors{"file": "Nenhuma cobrança encontrada."}
	}
	if len(rows) > 2000 {
		return res, httpx.FieldErrors{"file": errTooMany.Error()}
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return res, err
	}
	fail := func(r ImportRow, format string, a ...any) {
		if len(res.Errors) < 50 {
			res.Errors = append(res.Errors, fmt.Sprintf("linha %d: %s", r.Line, fmt.Sprintf(format, a...)))
		}
	}
	for _, r := range rows {
		status := r.Status
		if status == "" {
			status = ChargeOpen
		}
		link := sanitizeURL(r.URL)
		switch {
		case strings.TrimSpace(r.ExternalID) == "" || len(r.ExternalID) > 120:
			fail(r, "informe um código da cobrança (até 120 caracteres)")
			continue
		case r.AmountCents <= 0:
			fail(r, "valor inválido")
			continue
		case r.DueDate.IsZero():
			fail(r, "vencimento inválido")
			continue
		case (status == ChargeOpen || status == ChargeOverdue) && link == "":
			fail(r, "o link de pagamento precisa ser https")
			continue
		case link != "" && !domainAllowed(link, st.AllowedDomains):
			fail(r, "o domínio do link não está liberado em Controles → domínios permitidos")
			continue
		}
		contact := ""
		if r.ContactID != "" {
			if !httpx.ValidUUID(r.ContactID) {
				fail(r, "id de contato inválido")
				continue
			}
			var ok bool
			if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contacts WHERE id = $1 AND kind <> 'group')`, r.ContactID).Scan(&ok); err != nil {
				return res, err
			}
			if !ok {
				fail(r, "contato não encontrado")
				continue
			}
			contact = r.ContactID
		}
		created, linked, err := s.upsertCharge(ctx, ProviderManual, GatewayCharge{
			ExternalID: r.ExternalID, CustomerName: r.Name, CustomerPhone: r.Phone, Description: r.Description,
			AmountCents: r.AmountCents, DueDate: r.DueDate, Status: status, PaymentURL: link,
		}, contact)
		if err != nil {
			return res, err
		}
		if created {
			res.Created++
		} else {
			res.Updated++
		}
		if linked || contact != "" {
			res.Linked++
		}
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM billing_charges WHERE provider = $1 AND status IN ('open', 'overdue') AND contact_id IS NULL`, ProviderManual).Scan(&res.Unlinked)
	_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "billing.import", TargetType: "billing_charge", IP: ip,
			Metadata: map[string]any{"created": res.Created, "updated": res.Updated, "errors": len(res.Errors)}})
	})
	return res, nil
}

var (
	reMoneyJunk = regexp.MustCompile(`[^\d.,]`)
)

// ParseMoney entende "150", "150.00", "150,00" e "1.234,56" e devolve centavos.
func ParseMoney(s string) (int64, bool) {
	s = reMoneyJunk.ReplaceAllString(strings.TrimSpace(s), "")
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	}
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if strings.Contains(frac, ".") || len(frac) > 2 || len(whole) > 12 {
		return 0, false
	}
	frac = (frac + "00")[:2]
	w, err1 := strconv.ParseInt(whole, 10, 64)
	f, err2 := strconv.ParseInt(frac, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	cents := w*100 + f
	return cents, cents > 0
}

// ParseDate entende AAAA-MM-DD e DD/MM/AAAA.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ParseStatus entende os nomes em português e inglês usados em planilhas.
func ParseStatus(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "aberto", "aberta", "open", "pendente", "pending":
		return ChargeOpen, true
	case "vencido", "vencida", "overdue", "atrasado", "atrasada":
		return ChargeOverdue, true
	case "pago", "paga", "paid", "recebido", "recebida":
		return ChargePaid, true
	case "cancelado", "cancelada", "cancelled", "canceled":
		return ChargeCancelled, true
	}
	return "", false
}

// refreshOrLoad: tenta atualizar do gateway; se o gateway não responde, usa o que a última sincronização trouxe.
func (s *Service) refreshOrLoad(ctx context.Context, id string) (chargeRow, error) {
	c, err := s.refreshCharge(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return s.loadCharge(ctx, id)
	}
	return c, err
}
