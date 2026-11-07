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
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

const (
	tokenPrefix      = "chm_"
	ScopeRead        = "read"
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
	Owner      string     `json:"owner,omitempty"` // só na visão do administrador
	OwnerID    string     `json:"ownerId,omitempty"`
}

// Principal é quem está chamando o MCP.
type Principal struct {
	UserID  string
	Name    string
	Admin   bool
	TokenID string
	Prefix  string
	Scopes  []string
}

type Service struct {
	db *pgxpool.Pool

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

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var st Settings
	err := s.db.QueryRow(ctx, `SELECT enabled, mask_documents, mask_phones FROM ai_settings WHERE id`).
		Scan(&st.Enabled, &st.MaskDocuments, &st.MaskPhones)
	return st, err
}

// SetSettings (administrador). Desligar o acesso revoga nada: os tokens só deixam de funcionar enquanto estiver desligado.
func (s *Service) SetSettings(ctx context.Context, actorID, ip string, in Settings) (Settings, error) {
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE ai_settings SET enabled = $1, mask_documents = $2, mask_phones = $3,
			updated_at = now(), updated_by = $4 WHERE id`, in.Enabled, in.MaskDocuments, in.MaskPhones, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.settings.update", TargetType: "ai_settings", IP: ip,
			Metadata: map[string]any{"enabled": in.Enabled, "maskDocuments": in.MaskDocuments, "maskPhones": in.MaskPhones}})
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
func (s *Service) Create(ctx context.Context, userID, ip, name string, ttlDays int) (string, Token, error) {
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
	if st, err := s.Settings(ctx); err != nil {
		return "", Token{}, err
	} else if !st.Enabled {
		return "", Token{}, ErrDisabled
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
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO api_tokens (user_id, name, prefix, token_hash, expires_at) VALUES ($1, $2, $3, $4, $5)
			RETURNING id::text, name, prefix, scopes, expires_at, created_at`,
			userID, name, prefix, hashToken(raw), exp).Scan(&t.ID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "ai.token.create", TargetType: "api_token", TargetID: t.ID, IP: ip,
			Metadata: map[string]any{"prefix": prefix, "expiresInDays": ttlDays}})
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
		       t.expires_at <= now(), u.name, u.id::text
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
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.CreatedAt, &t.LastUsedAt, &t.Revoked, &t.Expired, &t.Owner, &t.OwnerID); err != nil {
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
		p    Principal
		hash []byte
		last *time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT t.id::text, t.prefix, t.scopes, t.token_hash, t.last_used_at, u.id::text, u.name, u.role = 'admin'
		  FROM api_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND t.expires_at > now() AND u.is_active`, hashToken(raw)).
		Scan(&p.TokenID, &p.Prefix, &p.Scopes, &hash, &last, &p.UserID, &p.Name, &p.Admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrBadToken
	}
	if err != nil {
		return Principal{}, err
	}
	if subtle.ConstantTimeCompare(hash, hashToken(raw)) != 1 {
		return Principal{}, ErrBadToken
	}
	if st, err := s.Settings(ctx); err != nil {
		return Principal{}, err
	} else if !st.Enabled {
		return Principal{}, ErrDisabled
	}
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
func (s *Service) Allow(tokenID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	b := s.buckets[tokenID]
	if b == nil || now.Sub(b.windowStart) >= time.Minute {
		s.buckets[tokenID] = &bucket{windowStart: now, count: 1}
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
	return b.count <= s.limit
}

// Record grava uma chamada de ferramenta na auditoria (sem conteúdo de mensagens).
func (s *Service) Record(ctx context.Context, p Principal, tool string, meta map[string]any, ok bool) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["tool"], meta["tokenPrefix"], meta["ok"] = tool, p.Prefix, ok
	_ = audit.Record(context.WithoutCancel(ctx), s.db, audit.Entry{ActorID: p.UserID, Action: "mcp.call", TargetType: "api_token", TargetID: p.TokenID, Metadata: meta})
}
