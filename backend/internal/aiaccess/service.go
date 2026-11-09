// Package aiaccess implementa o acesso por IA ao ChatHM: configuração do escritório, tokens pessoais
// e o servidor MCP (/mcp). Tudo roda como a pessoa dona do token, então a visibilidade por equipe vale igual ao app.
package aiaccess

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

const (
	tokenPrefix = "chm_"
	ScopeRead   = "read"
	ScopeWrite  = "write"
	ScopeSend   = "send"
	// ScopeAutomate: iniciar avisos sob a autonomia do assunto. Exige autorização explícita do usuário.
	ScopeAutomate    = "automate"
	maxTokensPerUser = 10
	defaultTTLDays   = 90
	maxTTLDays       = 365
)

var (
	ErrDisabled     = errors.New("aiaccess: acesso por IA desligado")
	ErrBadToken     = errors.New("aiaccess: token inválido")
	ErrTooManyCalls = errors.New("aiaccess: limite de chamadas")
	ErrNotFound     = errors.New("aiaccess: não encontrado")
)

// Settings é a configuração do escritório.
type Settings struct {
	Enabled       bool `json:"enabled"`
	MaskDocuments bool `json:"maskDocuments"`
	MaskPhones    bool `json:"maskPhones"`
	// AllowWrite libera tokens com escopo de escrita (etiquetar, anotar, adiar, agenda, transferir...).
	AllowWrite bool `json:"allowWrite"`
	// AllowSend libera tokens que PEDEM envios: a IA nunca envia; uma pessoa aprova cada mensagem na conversa.
	AllowSend bool `json:"allowSend"`
	// MaskEmails oculta e-mails (padrão ligado). StrictPrivacy reduz nomes a "Nome S." e remove observações internas.
	MaskEmails    bool `json:"maskEmails"`
	StrictPrivacy bool `json:"strictPrivacy"`
	// AllowOAuth libera conectar ChatGPT/claude.ai por login. OAuthRedirectHosts: hosts permitidos nos endereços de retorno.
	AllowOAuth         bool     `json:"allowOauth"`
	OAuthRedirectHosts []string `json:"oauthRedirectHosts"`
	// AllowAutomation libera pedidos com escopo "automate", sob as regras e autonomia de cada assunto.
	AllowAutomation bool `json:"allowAutomation"`
}

// Token é um token pessoal (nunca inclui o segredo).
type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	Revoked    bool       `json:"revoked"`
	Expired    bool       `json:"expired"`
	Provider   string     `json:"provider"`        // claude | chatgpt | grok | gemini | other | ""
	Owner      string     `json:"owner,omitempty"` // só na visão do administrador
	OwnerID    string     `json:"ownerId,omitempty"`
}

// Principal é quem está chamando o MCP.
type Principal struct {
	UserID  string
	Name    string
	Admin   bool
	Manager bool // gestor: age nas conversas de outras pessoas da equipe (como no app)
	TokenID string
	Prefix  string
	Scopes  []string
	// Write: o token tem escopo de escrita E o administrador liberou alterações.
	Write bool
	// Send: o token tem escopo "send" E o administrador liberou pedidos de envio (continuam dependendo de aprovação humana).
	Send bool
	// OAuth: token emitido a um aplicativo de IA hospedado por terceiros (ChatGPT, claude.ai): privacidade máxima, sem opção.
	OAuth      bool
	ClientName string
	// Automate: a conexão recebeu "automate" explicitamente e o administrador liberou envios automáticos.
	Automate bool
	// Provider e App identificam a IA (a tela mostra a logo do provedor nas mensagens automáticas).
	Provider string
	App      string
}

type Service struct {
	db    *pgxpool.Pool
	convs *conversations.Service
	pub   Publisher

	mu      sync.Mutex
	buckets map[string]*bucket // limite por token
	limit   int
}

type bucket struct {
	windowStart time.Time
	count       int
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db, buckets: map[string]*bucket{}, limit: 60}
}

// --- configuração ----------------------------------------------------------

const selectSettings = `SELECT enabled, mask_documents, mask_phones, allow_write, allow_send, mask_emails, strict_privacy, allow_oauth, oauth_redirect_hosts, allow_automation FROM ai_settings WHERE id`

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var st Settings
	scan := func() error {
		return s.db.QueryRow(ctx, selectSettings).
			Scan(&st.Enabled, &st.MaskDocuments, &st.MaskPhones, &st.AllowWrite, &st.AllowSend, &st.MaskEmails, &st.StrictPrivacy, &st.AllowOAuth, &st.OAuthRedirectHosts, &st.AllowAutomation)
	}
	err := scan()
	if errors.Is(err, pgx.ErrNoRows) {
		// A migração cria a linha única; se ela faltar (banco restaurado, linha apagada), recria com os padrões (tudo desligado).
		if _, ierr := s.db.Exec(ctx, `INSERT INTO ai_settings (id) VALUES (true) ON CONFLICT (id) DO NOTHING`); ierr != nil {
			return st, ierr
		}
		err = scan()
	}
	return st, err
}

// SetSettings (administrador). Desligar o acesso revoga nada: os tokens só deixam de funcionar enquanto estiver desligado.
func (s *Service) SetSettings(ctx context.Context, actorID, ip string, in Settings) (Settings, error) {
	hosts, err := cleanHosts(in.OAuthRedirectHosts)
	if err != nil {
		return Settings{}, err
	}
	in.OAuthRedirectHosts = hosts
	if _, err := s.Settings(ctx); err != nil { // garante a linha única antes de atualizar
		return Settings{}, err
	}
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE ai_settings SET enabled = $1, mask_documents = $2, mask_phones = $3,
			allow_write = $5, allow_send = $6, mask_emails = $7, strict_privacy = $8, allow_oauth = $9, oauth_redirect_hosts = $10, allow_automation = $11,
			updated_at = now(), updated_by = $4 WHERE id`, in.Enabled, in.MaskDocuments, in.MaskPhones, actorID, in.AllowWrite, in.AllowSend, in.MaskEmails, in.StrictPrivacy, in.AllowOAuth, hosts, in.AllowAutomation); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.settings.update", TargetType: "ai_settings", IP: ip,
			Metadata: map[string]any{"enabled": in.Enabled, "maskDocuments": in.MaskDocuments, "maskPhones": in.MaskPhones, "allowWrite": in.AllowWrite, "allowSend": in.AllowSend, "maskEmails": in.MaskEmails, "strictPrivacy": in.StrictPrivacy, "allowOauth": in.AllowOAuth, "oauthRedirectHosts": hosts}})
	})
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

// --- tokens ----------------------------------------------------------------

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// Create cria um token pessoal. O segredo é devolvido só aqui.
func (s *Service) Create(ctx context.Context, userID, ip, name string, ttlDays int, scopes []string, provider string) (string, Token, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return "", Token{}, httpx.FieldErrors{"name": "Dê um nome de até 80 caracteres (ex.: Claude no meu notebook)."}
	}
	if ttlDays <= 0 {
		ttlDays = defaultTTLDays
	}
	if ttlDays > maxTTLDays {
		return "", Token{}, httpx.FieldErrors{"expiresInDays": "A validade máxima é de 365 dias."}
	}
	granted := []string{ScopeRead}
	for _, sc := range scopes {
		switch sc {
		case ScopeRead:
		case ScopeWrite:
			if !contains(granted, ScopeWrite) {
				granted = append(granted, ScopeWrite)
			}
		case ScopeSend:
			// Pedir envio inclui alterar (rascunhos e pedidos ficam na conversa).
			for _, x := range []string{ScopeWrite, ScopeSend} {
				if !contains(granted, x) {
					granted = append(granted, x)
				}
			}
		case ScopeAutomate:
			if !contains(granted, ScopeAutomate) {
				granted = append(granted, ScopeAutomate)
			}
		default:
			return "", Token{}, httpx.FieldErrors{"scopes": "Escopo inválido."}
		}
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !validProvider(provider) {
		return "", Token{}, httpx.FieldErrors{"provider": "Escolha o provedor da IA: Claude, ChatGPT, Grok, Gemini ou Outro."}
	}
	if contains(granted, ScopeAutomate) && provider == "" {
		return "", Token{}, httpx.FieldErrors{"provider": "Para envios automáticos, informe o provedor da IA (a logo dele aparece nas mensagens)."}
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return "", Token{}, err
	}
	if !st.Enabled {
		return "", Token{}, ErrDisabled
	}
	if contains(granted, ScopeSend) && !st.AllowSend {
		return "", Token{}, httpx.FieldErrors{"scopes": "O administrador ainda não liberou pedidos de envio pela IA (aba MCP)."}
	}
	if contains(granted, ScopeAutomate) && !st.AllowAutomation {
		return "", Token{}, httpx.FieldErrors{"scopes": "O administrador ainda não liberou envios automáticos pela IA (aba MCP)."}
	}
	if n := len(granted); n > 1 && !st.AllowWrite && !(n == 2 && contains(granted, ScopeAutomate)) {
		return "", Token{}, httpx.FieldErrors{"scopes": "O administrador ainda não liberou alterações pela IA (aba MCP)."}
	}
	var active int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()`, userID).Scan(&active); err != nil {
		return "", Token{}, err
	}
	if active >= maxTokensPerUser {
		return "", Token{}, httpx.FieldErrors{"name": "Você já tem 10 tokens ativos. Revogue algum antes de criar outro."}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Token{}, err
	}
	raw := tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	prefix := raw[:len(tokenPrefix)+6]
	exp := time.Now().Add(time.Duration(ttlDays) * 24 * time.Hour)
	var t Token
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO api_tokens (user_id, name, prefix, token_hash, expires_at, scopes, provider) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id::text, name, prefix, scopes, expires_at, created_at, provider`,
			userID, name, prefix, hashToken(raw), exp, granted, provider).Scan(&t.ID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.CreatedAt, &t.Provider); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "ai.token.create", TargetType: "api_token", TargetID: t.ID, IP: ip,
			Metadata: map[string]any{"prefix": prefix, "expiresInDays": ttlDays, "scopes": granted, "provider": provider}})
	})
	if err != nil {
		return "", Token{}, err
	}
	return raw, t, nil
}

// List: a própria pessoa vê os seus; o administrador pode ver todos (all=true).
func (s *Service) List(ctx context.Context, userID string, all bool) ([]Token, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.name, t.prefix, t.scopes, t.expires_at, t.created_at, t.last_used_at, t.revoked_at IS NOT NULL,
		       t.expires_at <= now(), u.name, u.id::text, t.provider
		  FROM api_tokens t JOIN users u ON u.id = t.user_id
		 WHERE ($2::boolean OR t.user_id = $1)
		 ORDER BY t.created_at DESC LIMIT 200`, userID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.CreatedAt, &t.LastUsedAt, &t.Revoked, &t.Expired, &t.Owner, &t.OwnerID, &t.Provider); err != nil {
			return nil, err
		}
		if !all {
			t.Owner, t.OwnerID = "", ""
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Revoke revoga um token (dono ou administrador).
func (s *Service) Revoke(ctx context.Context, actorID string, admin bool, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var prefix string
		err := tx.QueryRow(ctx, `UPDATE api_tokens SET revoked_at = now()
			 WHERE id = $1 AND revoked_at IS NULL AND ($3::boolean OR user_id = $2) RETURNING prefix`, id, actorID, admin).Scan(&prefix)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.token.revoke", TargetType: "api_token", TargetID: id, IP: ip,
			Metadata: map[string]any{"prefix": prefix}})
	})
}

// Authenticate valida um token Bearer. Falhas devolvem sempre ErrBadToken (sem dizer o motivo).
func (s *Service) Authenticate(ctx context.Context, raw string) (Principal, error) {
	if !strings.HasPrefix(raw, tokenPrefix) || len(raw) > 128 {
		return Principal{}, ErrBadToken
	}
	var (
		p       Principal
		hash    []byte
		last    *time.Time
		kind    string
		grantID *string
		client  *string
	)
	err := s.db.QueryRow(ctx, `
		SELECT t.id::text, t.prefix, t.scopes, t.token_hash, t.last_used_at, u.id::text, u.name, u.role = 'admin', u.role = 'manager',
		       t.kind, t.grant_id::text, c.name, t.provider
		  FROM api_tokens t JOIN users u ON u.id = t.user_id
		  LEFT JOIN oauth_grants g ON g.id = t.grant_id AND g.revoked_at IS NULL
		  LEFT JOIN oauth_clients c ON c.client_id = g.client_id
		 WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND t.expires_at > now() AND u.is_active
		   AND (t.kind = 'personal' OR g.id IS NOT NULL)`, hashToken(raw)).
		Scan(&p.TokenID, &p.Prefix, &p.Scopes, &hash, &last, &p.UserID, &p.Name, &p.Admin, &p.Manager, &kind, &grantID, &client, &p.Provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrBadToken
	}
	if err != nil {
		return Principal{}, err
	}
	if subtle.ConstantTimeCompare(hash, hashToken(raw)) != 1 {
		return Principal{}, ErrBadToken
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return Principal{}, err
	}
	if !st.Enabled {
		return Principal{}, ErrDisabled
	}
	if kind == "oauth" {
		if !st.AllowOAuth {
			return Principal{}, ErrDisabled
		}
		p.OAuth = true
		if client != nil {
			p.ClientName = *client
		}
		if grantID != nil && (last == nil || time.Since(*last) > time.Minute) {
			_, _ = s.db.Exec(ctx, `UPDATE oauth_grants SET last_used_at = now() WHERE id = $1`, *grantID)
		}
	}
	for _, sc := range p.Scopes {
		if sc == ScopeWrite && st.AllowWrite {
			p.Write = true
		}
		if sc == ScopeSend && st.AllowSend && st.AllowWrite {
			p.Send = true
		}
		if sc == ScopeAutomate && st.AllowAutomation && st.AllowWrite && st.AllowSend {
			p.Automate = true
		}
	}
	if p.Provider == "" && p.OAuth {
		p.Provider = providerOf(p.ClientName)
	}
	p.App = appName(p.Provider, p.ClientName)
	if last == nil || time.Since(*last) > time.Minute {
		_, _ = s.db.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE id = $1`, p.TokenID)
	}
	return p, nil
}

// SetRateLimit muda o teto de chamadas por minuto de cada token (testes).
func (s *Service) SetRateLimit(n int) {
	s.mu.Lock()
	s.limit = n
	s.mu.Unlock()
}

// Allow aplica o limite de chamadas por token (janela de 1 minuto).
func (s *Service) Allow(tokenID string) bool { return s.allow(tokenID, s.rateLimit()) }

// AllowWriteCall limita as alterações por token (20 por minuto, bem abaixo das leituras).
func (s *Service) AllowWriteCall(tokenID string) bool { return s.allow("w:"+tokenID, 20) }

func (s *Service) rateLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

func (s *Service) allow(key string, limit int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	b := s.buckets[key]
	if b == nil || now.Sub(b.windowStart) >= time.Minute {
		s.buckets[key] = &bucket{windowStart: now, count: 1}
		if len(s.buckets) > 5000 { // não crescer sem limite
			for k, v := range s.buckets {
				if now.Sub(v.windowStart) >= time.Minute {
					delete(s.buckets, k)
				}
			}
		}
		return true
	}
	b.count++
	return b.count <= limit
}

// Record grava uma chamada de ferramenta na auditoria (sem conteúdo de mensagens).
func (s *Service) Record(ctx context.Context, p Principal, tool string, meta map[string]any, ok bool) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["tool"], meta["tokenPrefix"], meta["ok"] = tool, p.Prefix, ok
	_ = audit.Record(context.WithoutCancel(ctx), s.db, audit.Entry{ActorID: p.UserID, Action: "mcp.call", TargetType: "api_token", TargetID: p.TokenID, Metadata: meta})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// MaskerFor monta o filtro de privacidade de uma chamada. Clientes OAuth recebem tudo ocultado, sempre.
func MaskerFor(st Settings, p Principal) Masker {
	if p.OAuth {
		return Masker{Documents: true, Phones: true, Emails: true, Addresses: true, Strict: st.StrictPrivacy}
	}
	return Masker{Documents: st.MaskDocuments, Phones: st.MaskPhones, Emails: st.MaskEmails, Addresses: st.MaskDocuments, Strict: st.StrictPrivacy}
}

// validProvider: provedores de IA reconhecidos (cada um tem a sua logo na tela).
func validProvider(p string) bool {
	switch p {
	case "", "claude", "chatgpt", "grok", "gemini", "other":
		return true
	}
	return false
}

// providerOf deduz o provedor pelo nome do aplicativo OAuth ("Claude", "ChatGPT"...).
func providerOf(client string) string {
	c := strings.ToLower(client)
	switch {
	case strings.Contains(c, "claude") || strings.Contains(c, "anthropic"):
		return "claude"
	case strings.Contains(c, "chatgpt") || strings.Contains(c, "openai"):
		return "chatgpt"
	case strings.Contains(c, "grok") || strings.Contains(c, "xai") || strings.Contains(c, "x.ai"):
		return "grok"
	case strings.Contains(c, "gemini") || strings.Contains(c, "google"):
		return "gemini"
	}
	return "other"
}

var providerNames = map[string]string{"claude": "Claude", "chatgpt": "ChatGPT", "grok": "Grok", "gemini": "Gemini", "other": "IA"}

// appName: o nome mostrado ao lado da logo.
func appName(provider, client string) string {
	if strings.TrimSpace(client) != "" {
		return client
	}
	if n, ok := providerNames[provider]; ok {
		return n
	}
	return "IA"
}
