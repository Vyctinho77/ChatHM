package aiaccess

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

const (
	accessTokenTTL   = time.Hour
	refreshTokenTTL  = 30 * 24 * time.Hour
	authCodeTTL      = time.Minute
	maxOAuthClients  = 200
	maxGrantsPerUser = 20
	refreshPrefix    = "chmr_"
	codePrefix       = "chmc_"
	clientPrefix     = "chc_"
)

var (
	ErrOAuthDisabled = errors.New("aiaccess: conexão por login (OAuth) desligada")
	hostRe           = regexp.MustCompile(`^[a-z0-9]([a-z0-9.\-]{0,251}[a-z0-9])?$`)
	verifierRe       = regexp.MustCompile(`^[A-Za-z0-9\-._~]{43,128}$`)
)

// OAuthError segue o formato de erro do RFC 6749 (error + error_description).
type OAuthError struct{ Code, Description string }

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }

func oerr(code, desc string) error { return &OAuthError{Code: code, Description: desc} }

// cleanHosts valida a lista de hosts permitidos nos endereços de retorno (sem esquema, caminho, porta ou curinga).
func cleanHosts(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, h := range in {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if !hostRe.MatchString(h) || !strings.Contains(h, ".") {
			return nil, httpx.FieldErrors{"oauthRedirectHosts": "Use só nomes de site, como chatgpt.com (sem https://, caminho ou *)."}
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if len(out) > 10 {
		return nil, httpx.FieldErrors{"oauthRedirectHosts": "No máximo 10 sites."}
	}
	return out, nil
}

func randToken(prefix string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Client é um aplicativo de IA registrado (registro dinâmico, RFC 7591).
type Client struct {
	ClientID     string   `json:"client_id"`
	Name         string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1"
}

// validRedirect aceita https em hosts da lista do administrador, ou http em loopback (aplicativos de computador).
func validRedirect(raw string, hosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Host == "" || len(raw) > 512 {
		return false
	}
	switch u.Scheme {
	case "https":
		h := strings.ToLower(u.Hostname())
		for _, allowed := range hosts {
			if h == allowed {
				return true
			}
		}
	case "http":
		return isLoopback(strings.ToLower(u.Hostname()))
	}
	return false
}

func cleanName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "Aplicativo de IA"
	}
	if utf8.RuneCountInString(out) > 80 {
		out = string([]rune(out)[:80])
	}
	return out
}

// RegisterClient implementa o registro dinâmico. Só endereços de retorno da lista do administrador passam.
func (s *Service) RegisterClient(ctx context.Context, ip, name string, redirectURIs []string) (Client, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return Client{}, err
	}
	if !st.Enabled || !st.AllowOAuth {
		return Client{}, ErrOAuthDisabled
	}
	if len(redirectURIs) == 0 || len(redirectURIs) > 5 {
		return Client{}, oerr("invalid_redirect_uri", "Informe de 1 a 5 redirect_uris.")
	}
	for _, u := range redirectURIs {
		if !validRedirect(u, st.OAuthRedirectHosts) {
			return Client{}, oerr("invalid_redirect_uri", "Endereço de retorno não permitido neste escritório: "+truncate(u, 80))
		}
	}
	c := Client{Name: cleanName(name), RedirectURIs: redirectURIs}
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		// Limpeza: registros antigos sem nenhuma autorização.
		if _, err := tx.Exec(ctx, `DELETE FROM oauth_clients c WHERE c.created_at < now() - interval '30 days'
			AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.client_id)`); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM oauth_clients`).Scan(&n); err != nil {
			return err
		}
		if n >= maxOAuthClients {
			return oerr("temporarily_unavailable", "Há aplicativos demais registrados; tente mais tarde.")
		}
		id, err := randToken(clientPrefix, 16)
		if err != nil {
			return err
		}
		c.ClientID = id
		if _, err := tx.Exec(ctx, `INSERT INTO oauth_clients (client_id, name, redirect_uris) VALUES ($1, $2, $3)`, c.ClientID, c.Name, c.RedirectURIs); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "oauth.client.register", TargetType: "oauth_client", TargetID: c.ClientID, IP: ip,
			Metadata: map[string]any{"name": c.Name, "redirectUris": c.RedirectURIs}})
	})
	return c, err
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func (s *Service) client(ctx context.Context, id string) (Client, error) {
	var c Client
	err := s.db.QueryRow(ctx, `SELECT client_id, name, redirect_uris FROM oauth_clients WHERE client_id = $1`, id).Scan(&c.ClientID, &c.Name, &c.RedirectURIs)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, oerr("invalid_client", "Aplicativo não registrado.")
	}
	return c, err
}

// AuthorizeParams são os parâmetros do pedido de autorização (RFC 6749 + PKCE + RFC 8707).
type AuthorizeParams struct {
	ClientID, RedirectURI, CodeChallenge, CodeChallengeMethod, State, Scope, Resource, ExpectedResource string
}

var levelOrder = []string{ScopeRead, ScopeWrite, ScopeSend}

func levelScopes(level string) []string {
	switch level {
	case ScopeSend:
		return []string{ScopeRead, ScopeWrite, ScopeSend}
	case ScopeWrite:
		return []string{ScopeRead, ScopeWrite}
	}
	return []string{ScopeRead}
}

// Preview valida o pedido e devolve o aplicativo e os níveis que a pessoa pode conceder.
func (s *Service) Preview(ctx context.Context, p AuthorizeParams) (Client, []string, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return Client{}, nil, err
	}
	if !st.Enabled || !st.AllowOAuth {
		return Client{}, nil, ErrOAuthDisabled
	}
	c, err := s.client(ctx, p.ClientID)
	if err != nil {
		return c, nil, err
	}
	ok := false
	for _, u := range c.RedirectURIs {
		if subtle.ConstantTimeCompare([]byte(u), []byte(p.RedirectURI)) == 1 {
			ok = true
		}
	}
	if !ok {
		return c, nil, oerr("invalid_request", "redirect_uri não corresponde ao registrado.")
	}
	if p.CodeChallengeMethod != "S256" || !verifierRe.MatchString(p.CodeChallenge) {
		return c, nil, oerr("invalid_request", "PKCE (code_challenge S256) é obrigatório.")
	}
	if p.Resource != "" && p.Resource != p.ExpectedResource {
		return c, nil, oerr("invalid_target", "resource não corresponde a este servidor.")
	}
	// Níveis possíveis: o que o administrador liberou, limitado ao que o aplicativo pediu (scope vazio = sem limite).
	max := 0
	if st.AllowWrite {
		max = 1
		if st.AllowSend {
			max = 2
		}
	}
	if sc := strings.Fields(p.Scope); len(sc) > 0 {
		asked := 0
		for _, x := range sc {
			for i, l := range levelOrder {
				if x == l && i > asked {
					asked = i
				}
			}
		}
		if asked < max {
			max = asked
		}
	}
	return c, levelOrder[:max+1], nil
}

// Authorize registra a decisão da pessoa: devolve a URL de retorno com code (ou error=access_denied).
func (s *Service) Authorize(ctx context.Context, userID, ip string, p AuthorizeParams, level string, approve bool) (string, error) {
	c, levels, err := s.Preview(ctx, p)
	if err != nil {
		return "", err
	}
	back, err := url.Parse(p.RedirectURI)
	if err != nil {
		return "", oerr("invalid_request", "redirect_uri inválido.")
	}
	q := back.Query()
	if p.State != "" {
		q.Set("state", p.State)
	}
	if !approve {
		q.Set("error", "access_denied")
		back.RawQuery = q.Encode()
		_ = audit.Record(ctx, s.db, audit.Entry{ActorID: userID, Action: "oauth.grant.deny", TargetType: "oauth_client", TargetID: c.ClientID, IP: ip})
		return back.String(), nil
	}
	if !contains(levels, level) {
		return "", httpx.FieldErrors{"level": "Nível não permitido para este aplicativo."}
	}
	code, err := randToken(codePrefix, 32)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM oauth_grants WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&active); err != nil {
			return err
		}
		if active >= maxGrantsPerUser {
			return httpx.FieldErrors{"level": "Você já tem 20 aplicativos conectados; desconecte algum antes."}
		}
		var gid string
		scopes := levelScopes(level)
		if err := tx.QueryRow(ctx, `INSERT INTO oauth_grants (user_id, client_id, scopes) VALUES ($1, $2, $3) RETURNING id::text`, userID, c.ClientID, scopes).Scan(&gid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO oauth_codes (code_hash, grant_id, redirect_uri, code_challenge, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, now() + $6::interval)`, hashToken(code), gid, p.RedirectURI, p.CodeChallenge, p.Resource, "60 seconds"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "oauth.grant.create", TargetType: "oauth_grant", TargetID: gid, IP: ip,
			Metadata: map[string]any{"client": c.Name, "level": level}})
	})
	if err != nil {
		return "", err
	}
	q.Set("code", code)
	back.RawQuery = q.Encode()
	return back.String(), nil
}

// TokenResponse é a resposta do endpoint /oauth/token.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type grantRow struct {
	id, userID, clientID, clientName string
	scopes                           []string
	revoked                          bool
}

func (s *Service) grant(ctx context.Context, db database.DBTX, id string) (grantRow, error) {
	var g grantRow
	var revokedAt *time.Time
	err := db.QueryRow(ctx, `SELECT g.id::text, g.user_id::text, g.client_id, c.name, g.scopes, g.revoked_at
		FROM oauth_grants g JOIN oauth_clients c ON c.client_id = g.client_id JOIN users u ON u.id = g.user_id AND u.is_active
		WHERE g.id = $1`, id).Scan(&g.id, &g.userID, &g.clientID, &g.clientName, &g.scopes, &revokedAt)
	g.revoked = revokedAt != nil
	return g, err
}

func (s *Service) revokeGrantTx(ctx context.Context, db database.DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE oauth_grants SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE grant_id = $1 AND revoked_at IS NULL`, id)
	return err
}

// issue emite o par de tokens da concessão e revoga os tokens de acesso anteriores dela.
func (s *Service) issue(ctx context.Context, tx pgx.Tx, g grantRow) (TokenResponse, error) {
	access, err := randToken(tokenPrefix, 32)
	if err != nil {
		return TokenResponse{}, err
	}
	refresh, err := randToken(refreshPrefix, 32)
	if err != nil {
		return TokenResponse{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE grant_id = $1 AND revoked_at IS NULL`, g.id); err != nil {
		return TokenResponse{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO api_tokens (user_id, name, prefix, token_hash, scopes, expires_at, kind, grant_id)
		VALUES ($1, $2, $3, $4, $5, now() + $6::interval, 'oauth', $7)`,
		g.userID, g.clientName, access[:len(tokenPrefix)+6], hashToken(access), g.scopes, "1 hour", g.id); err != nil {
		return TokenResponse{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO oauth_refresh_tokens (token_hash, grant_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		hashToken(refresh), g.id, "30 days"); err != nil {
		return TokenResponse{}, err
	}
	return TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(accessTokenTTL.Seconds()), RefreshToken: refresh, Scope: strings.Join(g.scopes, " ")}, nil
}

// ExchangeCode troca o código (com PKCE) por tokens.
func (s *Service) ExchangeCode(ctx context.Context, clientID, code, redirectURI, verifier, resource string) (TokenResponse, error) {
	if !verifierRe.MatchString(verifier) {
		return TokenResponse{}, oerr("invalid_grant", "code_verifier inválido.")
	}
	var out TokenResponse
	var reuse string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var gid, uri, challenge, res string
		err := tx.QueryRow(ctx, `UPDATE oauth_codes SET used_at = now() WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
			RETURNING grant_id::text, redirect_uri, code_challenge, resource`, hashToken(code)).Scan(&gid, &uri, &challenge, &res)
		if errors.Is(err, pgx.ErrNoRows) {
			// Código já usado = possível roubo: a concessão inteira é revogada.
			_ = tx.QueryRow(ctx, `SELECT grant_id::text FROM oauth_codes WHERE code_hash = $1 AND used_at IS NOT NULL`, hashToken(code)).Scan(&reuse)
			return oerr("invalid_grant", "Código inválido, expirado ou já usado.")
		}
		if err != nil {
			return err
		}
		g, err := s.grant(ctx, tx, gid)
		if err != nil || g.revoked {
			return oerr("invalid_grant", "Concessão revogada.")
		}
		if g.clientID != clientID || uri != redirectURI {
			return oerr("invalid_grant", "Código emitido para outro aplicativo ou endereço de retorno.")
		}
		sum := sha256.Sum256([]byte(verifier))
		if subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) != 1 {
			return oerr("invalid_grant", "code_verifier não confere (PKCE).")
		}
		if resource != "" && res != "" && resource != res {
			return oerr("invalid_target", "resource não confere.")
		}
		out, err = s.issue(ctx, tx, g)
		return err
	})
	if err != nil && reuse != "" {
		_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error { return s.revokeGrantTx(ctx, tx, reuse) })
	}
	return out, err
}

// ExchangeRefresh renova os tokens (o token de renovação é de uso único; reapresentar um usado revoga a concessão).
func (s *Service) ExchangeRefresh(ctx context.Context, clientID, refresh string) (TokenResponse, error) {
	if !strings.HasPrefix(refresh, refreshPrefix) || len(refresh) > 128 {
		return TokenResponse{}, oerr("invalid_grant", "Token de renovação inválido.")
	}
	var out TokenResponse
	var reuse string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var gid string
		err := tx.QueryRow(ctx, `UPDATE oauth_refresh_tokens SET used_at = now() WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
			RETURNING grant_id::text`, hashToken(refresh)).Scan(&gid)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.QueryRow(ctx, `SELECT grant_id::text FROM oauth_refresh_tokens WHERE token_hash = $1 AND used_at IS NOT NULL`, hashToken(refresh)).Scan(&reuse)
			return oerr("invalid_grant", "Token de renovação inválido, expirado ou já usado.")
		}
		if err != nil {
			return err
		}
		g, err := s.grant(ctx, tx, gid)
		if err != nil || g.revoked || g.clientID != clientID {
			return oerr("invalid_grant", "Concessão revogada ou de outro aplicativo.")
		}
		st, err := s.Settings(ctx)
		if err != nil {
			return err
		}
		if !st.Enabled || !st.AllowOAuth {
			return oerr("invalid_grant", "Conexão por login desligada neste escritório.")
		}
		out, err = s.issue(ctx, tx, g)
		return err
	})
	if err != nil && reuse != "" {
		_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error { return s.revokeGrantTx(ctx, tx, reuse) })
	}
	return out, err
}

// RevokeToken (RFC 7009): token de renovação revoga a concessão toda; token de acesso revoga só ele. Sempre "sucesso".
func (s *Service) RevokeToken(ctx context.Context, clientID, token string) {
	switch {
	case strings.HasPrefix(token, refreshPrefix):
		var gid string
		if s.db.QueryRow(ctx, `SELECT g.id::text FROM oauth_refresh_tokens r JOIN oauth_grants g ON g.id = r.grant_id WHERE r.token_hash = $1 AND g.client_id = $2`,
			hashToken(token), clientID).Scan(&gid) == nil {
			_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error { return s.revokeGrantTx(ctx, tx, gid) })
		}
	case strings.HasPrefix(token, tokenPrefix):
		_, _ = s.db.Exec(ctx, `UPDATE api_tokens t SET revoked_at = now() FROM oauth_grants g
			WHERE t.token_hash = $1 AND t.grant_id = g.id AND g.client_id = $2 AND t.revoked_at IS NULL`, hashToken(token), clientID)
	}
}

// Grant é um aplicativo autorizado por uma pessoa (para a lista "Aplicativos conectados").
type Grant struct {
	ID         string     `json:"id"`
	Client     string     `json:"client"`
	Level      string     `json:"level"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	Owner      string     `json:"owner,omitempty"`
}

func (s *Service) Grants(ctx context.Context, userID string, all bool) ([]Grant, error) {
	rows, err := s.db.Query(ctx, `
		SELECT g.id::text, c.name, g.scopes, g.created_at, g.last_used_at, u.name
		  FROM oauth_grants g JOIN oauth_clients c ON c.client_id = g.client_id JOIN users u ON u.id = g.user_id
		 WHERE g.revoked_at IS NULL AND ($2::boolean OR g.user_id = $1) ORDER BY g.created_at DESC LIMIT 200`, userID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Grant{}
	for rows.Next() {
		var g Grant
		var scopes []string
		if err := rows.Scan(&g.ID, &g.Client, &scopes, &g.CreatedAt, &g.LastUsedAt, &g.Owner); err != nil {
			return nil, err
		}
		g.Level = ScopeRead
		if contains(scopes, ScopeSend) {
			g.Level = ScopeSend
		} else if contains(scopes, ScopeWrite) {
			g.Level = ScopeWrite
		}
		if !all {
			g.Owner = ""
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// RevokeGrant desconecta um aplicativo (a própria pessoa ou um administrador).
func (s *Service) RevokeGrant(ctx context.Context, actorID string, admin bool, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var client string
		err := tx.QueryRow(ctx, `SELECT c.name FROM oauth_grants g JOIN oauth_clients c ON c.client_id = g.client_id
			WHERE g.id = $1 AND g.revoked_at IS NULL AND ($3::boolean OR g.user_id = $2) FOR UPDATE OF g`, id, actorID, admin).Scan(&client)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := s.revokeGrantTx(ctx, tx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "oauth.grant.revoke", TargetType: "oauth_grant", TargetID: id, IP: ip, Metadata: map[string]any{"client": client}})
	})
}
