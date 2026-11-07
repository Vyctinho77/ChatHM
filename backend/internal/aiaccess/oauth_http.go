package aiaccess

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// BaseURLFunc devolve o endereço público do servidor (https://dominio) para os metadados e o /mcp.
type BaseURLFunc func(*http.Request) string

// PublicBase escolhe a origem configurada que corresponde ao Host da requisição; sem correspondência, usa o esquema
// (X-Forwarded-Proto/TLS) e o Host recebidos.
func PublicBase(allowedOrigins []string) BaseURLFunc {
	return func(r *http.Request) string {
		for _, o := range allowedOrigins {
			if u, err := url.Parse(o); err == nil && strings.EqualFold(u.Host, r.Host) {
				return strings.TrimRight(o, "/")
			}
		}
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		return scheme + "://" + r.Host
	}
}

// OAuthHandler: endpoints públicos do OAuth (metadados, registro, token, revogação).
type OAuthHandler struct {
	svc  *Service
	base BaseURLFunc

	mu    sync.Mutex
	tries map[string][]time.Time // limite por IP
}

func NewOAuthHandler(svc *Service, base BaseURLFunc) *OAuthHandler {
	return &OAuthHandler{svc: svc, base: base, tries: map[string][]time.Time{}}
}

func (h *OAuthHandler) allow(kind, ip string, max int, window time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	k := kind + ":" + ip
	now := time.Now()
	kept := h.tries[k][:0]
	for _, t := range h.tries[k] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= max {
		h.tries[k] = kept
		return false
	}
	h.tries[k] = append(kept, now)
	if len(h.tries) > 5000 {
		for key, v := range h.tries {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > window {
				delete(h.tries, key)
			}
		}
	}
	return true
}

func (h *OAuthHandler) enabled(r *http.Request) bool {
	st, err := h.svc.Settings(r.Context())
	return err == nil && st.Enabled && st.AllowOAuth
}

// Routes: na raiz do servidor (sem /api/v1 e sem sessão).
func (h *OAuthHandler) Routes(r chi.Router) {
	r.Get("/.well-known/oauth-protected-resource", h.protectedResource)
	r.Get("/.well-known/oauth-protected-resource/mcp", h.protectedResource)
	r.Get("/.well-known/oauth-authorization-server", h.authServer)
	r.Get("/.well-known/oauth-authorization-server/mcp", h.authServer)
	r.Get("/.well-known/openid-configuration", h.authServer)
	r.Post("/oauth/register", h.register)
	r.Get("/oauth/authorize", h.authorize)
	r.Post("/oauth/token", h.token)
	r.Post("/oauth/revoke", h.revoke)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthFail(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func (h *OAuthHandler) protectedResource(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(r) {
		http.NotFound(w, r)
		return
	}
	b := h.base(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource": b + "/mcp", "authorization_servers": []string{b}, "scopes_supported": levelOrder,
		"bearer_methods_supported": []string{"header"}, "resource_name": "ChatHM",
	})
}

func (h *OAuthHandler) authServer(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(r) {
		http.NotFound(w, r)
		return
	}
	b := h.base(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer": b, "authorization_endpoint": b + "/oauth/authorize", "token_endpoint": b + "/oauth/token",
		"registration_endpoint": b + "/oauth/register", "revocation_endpoint": b + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		"revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": levelOrder,
	})
}

// register: registro dinâmico de cliente (RFC 7591), clientes públicos com PKCE.
func (h *OAuthHandler) register(w http.ResponseWriter, r *http.Request) {
	if !h.allow("register", httpx.ClientIP(r), 20, time.Hour) {
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Muitos registros; tente mais tarde.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
		AuthMethod   string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "JSON inválido.")
		return
	}
	if in.AuthMethod != "" && in.AuthMethod != "none" {
		oauthFail(w, http.StatusBadRequest, "invalid_client_metadata", "Só clientes públicos (token_endpoint_auth_method: none) com PKCE.")
		return
	}
	c, err := h.svc.RegisterClient(r.Context(), httpx.ClientIP(r), in.ClientName, in.RedirectURIs)
	var oe *OAuthError
	switch {
	case errors.Is(err, ErrOAuthDisabled):
		oauthFail(w, http.StatusForbidden, "access_denied", "A conexão por login (OAuth) está desligada neste escritório.")
		return
	case errors.As(err, &oe):
		oauthFail(w, http.StatusBadRequest, oe.Code, oe.Description)
		return
	case err != nil:
		httpx.Internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ClientID, "client_name": c.Name, "redirect_uris": c.RedirectURIs,
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
}

// authorize: leva o navegador para a tela de consentimento do ChatHM (que exige login e autenticador).
func (h *OAuthHandler) authorize(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(r) {
		http.Error(w, "Conexão por login desligada neste escritório.", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/oauth/consent?"+r.URL.RawQuery, http.StatusFound)
}

func (h *OAuthHandler) token(w http.ResponseWriter, r *http.Request) {
	ip := httpx.ClientIP(r)
	if !h.allow("token", ip, 120, time.Minute) {
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Muitas tentativas.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		oauthFail(w, http.StatusBadRequest, "invalid_request", "Corpo inválido (use application/x-www-form-urlencoded).")
		return
	}
	f := r.PostForm
	if !h.enabled(r) {
		oauthFail(w, http.StatusForbidden, "access_denied", "A conexão por login (OAuth) está desligada neste escritório.")
		return
	}
	var (
		out TokenResponse
		err error
	)
	switch f.Get("grant_type") {
	case "authorization_code":
		out, err = h.svc.ExchangeCode(r.Context(), f.Get("client_id"), f.Get("code"), f.Get("redirect_uri"), f.Get("code_verifier"), f.Get("resource"))
	case "refresh_token":
		out, err = h.svc.ExchangeRefresh(r.Context(), f.Get("client_id"), f.Get("refresh_token"))
	default:
		oauthFail(w, http.StatusBadRequest, "unsupported_grant_type", "Use authorization_code ou refresh_token.")
		return
	}
	var oe *OAuthError
	switch {
	case errors.As(err, &oe):
		status := http.StatusBadRequest
		if oe.Code == "invalid_client" {
			status = http.StatusUnauthorized
		}
		oauthFail(w, status, oe.Code, oe.Description)
		return
	case err != nil:
		httpx.Internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *OAuthHandler) revoke(w http.ResponseWriter, r *http.Request) {
	if !h.allow("revoke", httpx.ClientIP(r), 60, time.Minute) {
		oauthFail(w, http.StatusTooManyRequests, "temporarily_unavailable", "Muitas tentativas.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err == nil {
		h.svc.RevokeToken(r.Context(), r.PostForm.Get("client_id"), r.PostForm.Get("token"))
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK) // RFC 7009: sempre 200
}

// --- sessão do app (tela de consentimento e "Aplicativos conectados") --------------------------------------------------

func authorizeParams(q url.Values, expectedResource string) AuthorizeParams {
	return AuthorizeParams{
		ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"), CodeChallenge: q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"), State: q.Get("state"), Scope: q.Get("scope"),
		Resource: q.Get("resource"), ExpectedResource: expectedResource,
	}
}

func (h *Handler) oauthFail(w http.ResponseWriter, r *http.Request, err error) bool {
	var oe *OAuthError
	switch {
	case errors.Is(err, ErrOAuthDisabled):
		httpx.Error(w, http.StatusConflict, "oauth_disabled", "A conexão de aplicativos por login está desligada neste escritório. Peça a um administrador para ligar em Configurações → Integrações.")
	case errors.As(err, &oe):
		httpx.Error(w, http.StatusBadRequest, oe.Code, oe.Description)
	default:
		h.fail(w, r, err)
	}
	return true
}

// oauthPreview: valida o pedido e informa o que a pessoa pode conceder (a tela só mostra um pedido válido).
func (h *Handler) oauthPreview(w http.ResponseWriter, r *http.Request) {
	p := authorizeParams(r.URL.Query(), h.base(r)+"/mcp")
	c, levels, err := h.svc.Preview(r.Context(), p)
	if err != nil {
		h.oauthFail(w, r, err)
		return
	}
	host := ""
	if u, err := url.Parse(p.RedirectURI); err == nil {
		host = u.Host
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"client": map[string]string{"id": c.ClientID, "name": c.Name}, "redirectHost": host, "levels": levels})
}

// oauthAuthorize: a pessoa aprova (com o nível escolhido) ou nega; devolve a URL de retorno ao aplicativo.
func (h *Handler) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ClientID            string `json:"clientId"`
		RedirectURI         string `json:"redirectUri"`
		CodeChallenge       string `json:"codeChallenge"`
		CodeChallengeMethod string `json:"codeChallengeMethod"`
		State               string `json:"state"`
		Scope               string `json:"scope"`
		Resource            string `json:"resource"`
		Level               string `json:"level"`
		Approve             bool   `json:"approve"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	p := AuthorizeParams{ClientID: in.ClientID, RedirectURI: in.RedirectURI, CodeChallenge: in.CodeChallenge, CodeChallengeMethod: in.CodeChallengeMethod,
		State: in.State, Scope: in.Scope, Resource: in.Resource, ExpectedResource: h.base(r) + "/mcp"}
	to, err := h.svc.Authorize(r.Context(), sess.UserID, httpx.ClientIP(r), p, in.Level, in.Approve)
	if err != nil {
		h.oauthFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"redirectTo": to})
}

func (h *Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	list, err := h.svc.Grants(r.Context(), sess.UserID, sess.IsAdmin() && r.URL.Query().Get("all") == "1")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"grants": list})
}

func (h *Handler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.RevokeGrant(r.Context(), sess.UserID, sess.IsAdmin(), httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
