package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

const CSRFHeader = "X-CSRF-Token"

type ctxKey struct{}

// FromContext devolve a sessão autenticada da requisição.
func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(*Session)
	return s, ok
}

// MustFromContext é usado em handlers montados atrás de RequireSession.
func MustFromContext(ctx context.Context) *Session {
	s, ok := FromContext(ctx)
	if !ok {
		panic("auth: handler sem sessão no contexto (falta RequireSession)")
	}
	return s
}

func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

func ClientFrom(r *http.Request) Client {
	return Client{IP: httpx.ClientIP(r), UserAgent: r.UserAgent()}
}

// ---------------------------------------------------------------------------
// Middlewares

// LoadSession coloca no contexto a sessão do cookie, se válida.
func (s *Service) LoadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cfg.CookieName)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := s.Authenticate(r.Context(), c.Value)
		switch {
		case errors.Is(err, ErrNoSession):
			s.clearCookie(w)
			next.ServeHTTP(w, r)
		case err != nil:
			httpx.Internal(w, r, err)
		default:
			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		}
	})
}

// RequireSession exige uma sessão (mesmo que o MFA ainda esteja pendente).
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); !ok {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "Faça login para continuar.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMFA bloqueia sessões com segundo fator pendente e admins sem MFA
// configurado (quando exigido). Deve vir depois de RequireSession.
func (s *Service) RequireMFA(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := MustFromContext(r.Context())
		if sess.MFAPending() {
			httpx.Error(w, http.StatusForbidden, "mfa_required", "Informe o código de verificação.")
			return
		}
		if s.MFASetupRequired(sess) {
			httpx.Error(w, http.StatusForbidden, "mfa_setup_required", "Configure a verificação em duas etapas.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin exige perfil administrador. Deve vir depois de RequireSession.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !MustFromContext(r.Context()).IsAdmin() {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Acesso restrito a administradores.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// CSRF valida, em métodos que alteram estado, a origem da requisição e o token
// sincronizador da sessão enviado no cabeçalho X-CSRF-Token.
func (s *Service) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.originAllowed(r) {
			httpx.Error(w, http.StatusForbidden, "origin_not_allowed", "Origem não permitida.")
			return
		}
		sess := MustFromContext(r.Context())
		got := r.Header.Get(CSRFHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFToken)) != 1 {
			httpx.Error(w, http.StatusForbidden, "csrf_failed", "Token CSRF ausente ou inválido.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CheckOrigin protege rotas sem sessão (login) contra requisições cross-site.
func (s *Service) CheckOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSafeMethod(r.Method) && !s.originAllowed(r) {
			httpx.Error(w, http.StatusForbidden, "origin_not_allowed", "Origem não permitida.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) originAllowed(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		return slices.Contains(s.cfg.AllowedOrigins, origin)
	}
	// Sem Origin (clientes não-browser): rejeita apenas se o navegador declarar cross-site.
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

func (s *Service) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		Expires:  time.Now().Add(s.cfg.SessionTTL),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearCookie remove o cookie de sessão do navegador.
func (s *Service) ClearCookie(w http.ResponseWriter) { s.clearCookie(w) }

func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cfg.CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

// ---------------------------------------------------------------------------
// Handlers

type UserView struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Role       Role   `json:"role"`
	Theme      string `json:"theme"`
	MFAEnabled bool   `json:"mfaEnabled"`
}

type MeResponse struct {
	User             UserView `json:"user"`
	CSRFToken        string   `json:"csrfToken"`
	MFARequired      bool     `json:"mfaRequired"`
	MFASetupRequired bool     `json:"mfaSetupRequired"`
}

func (s *Service) me(sess *Session) MeResponse {
	return MeResponse{
		User: UserView{
			ID: sess.UserID, Email: sess.Email, Name: sess.Name, Role: sess.Role,
			Theme: sess.Theme, MFAEnabled: sess.MFAEnabled,
		},
		CSRFToken:        sess.CSRFToken,
		MFARequired:      sess.MFAPending(),
		MFASetupRequired: s.MFASetupRequired(sess),
	}
}

// PublicRoutes: rotas acessíveis sem sessão.
func (s *Service) PublicRoutes(r chi.Router) {
	r.With(s.CheckOrigin).Post("/auth/login", s.handleLogin)
	r.With(s.CheckOrigin).Post("/auth/register", s.handleRegister)
	r.Get("/auth/options", s.handleOptions)
	r.Get("/setup/status", s.handleSetupStatus)
	r.With(s.CheckOrigin).Post("/setup/start", s.handleSetupStart)
	r.With(s.CheckOrigin).Post("/setup/finish", s.handleSetupFinish)
	r.With(s.CheckOrigin).Post("/auth/join/start", s.handleJoinStart)
	r.With(s.CheckOrigin).Post("/auth/join/finish", s.handleJoinFinish)
	r.With(s.CheckOrigin).Post("/auth/enroll/start", s.handleEnrollStart)
	r.With(s.CheckOrigin).Post("/auth/enroll/finish", s.handleEnrollFinish)
}

// SessionRoutes: rotas disponíveis mesmo com MFA pendente / não configurado.
func (s *Service) SessionRoutes(r chi.Router) {
	r.Get("/auth/me", s.handleMe)
	r.Post("/auth/logout", s.handleLogout)
	r.Post("/auth/mfa/verify", s.handleMFAVerify)
	r.Post("/auth/mfa/setup", s.handleMFASetup)
	r.Post("/auth/mfa/enable", s.handleMFAEnable)
}

// ProtectedRoutes: exigem sessão completa (MFA concluído quando aplicável).
func (s *Service) ProtectedRoutes(r chi.Router) {
	r.Post("/auth/password", s.handleChangePassword)
	r.Post("/auth/recovery-codes", s.handleRecoveryCodes)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"` // código do autenticador (ou de recuperação): entra sem senha
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	var (
		token string
		sess  *Session
		err   error
	)
	byCode := in.Password == "" && in.Code != ""
	if byCode {
		token, sess, err = s.LoginWithCode(r.Context(), in.Email, in.Code, ClientFrom(r))
	} else {
		token, sess, err = s.Login(r.Context(), in.Email, in.Password, ClientFrom(r))
	}
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		msg := "E-mail ou senha inválidos."
		if byCode {
			msg = "E-mail ou código inválidos."
		}
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", msg)
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Tente novamente mais tarde.")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		s.setCookie(w, token)
		httpx.JSON(w, http.StatusOK, s.me(sess))
	}
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, s.me(MustFromContext(r.Context())))
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.Logout(r.Context(), MustFromContext(r.Context()), ClientFrom(r)); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	s.clearCookie(w)
	httpx.NoContent(w)
}

type codeInput struct {
	Code string `json:"code"`
}

func (s *Service) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	sess := MustFromContext(r.Context())
	secret, url, err := s.SetupMFA(r.Context(), sess)
	if errors.Is(err, ErrMFAAlreadyEnabled) {
		httpx.Error(w, http.StatusConflict, "mfa_already_enabled", "A verificação em duas etapas já está ativa.")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauthUrl": url})
}

func (s *Service) handleMFAEnable(w http.ResponseWriter, r *http.Request) {
	var in codeInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := MustFromContext(r.Context())
	if err := s.EnableMFA(r.Context(), sess, in.Code, ClientFrom(r)); err != nil {
		s.writeMFAError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s.me(sess))
}

func (s *Service) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	var in codeInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	token, err := s.VerifyMFA(r.Context(), MustFromContext(r.Context()), in.Code, ClientFrom(r))
	if err != nil {
		s.writeMFAError(w, r, err)
		return
	}
	sess, err := s.Authenticate(r.Context(), token)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	s.setCookie(w, token)
	httpx.JSON(w, http.StatusOK, s.me(sess))
}

func (s *Service) writeMFAError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCode):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_code", "Código inválido.")
	case errors.Is(err, ErrTooManyAttempts):
		s.clearCookie(w)
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Faça login novamente.")
	case errors.Is(err, ErrMFAAlreadyEnabled):
		httpx.Error(w, http.StatusConflict, "mfa_already_enabled", "A verificação em duas etapas já está ativa.")
	case errors.Is(err, ErrMFANotEnabled):
		httpx.Error(w, http.StatusConflict, "mfa_not_enabled", "A verificação em duas etapas não está ativa.")
	case errors.Is(err, ErrMFASetupNotStarted):
		httpx.Error(w, http.StatusConflict, "mfa_setup_not_started", "Inicie a configuração antes de confirmar.")
	default:
		httpx.Internal(w, r, err)
	}
}

func (s *Service) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	err := s.ChangePassword(r.Context(), MustFromContext(r.Context()), in.CurrentPassword, in.NewPassword, ClientFrom(r))
	switch {
	case err == nil:
		httpx.NoContent(w)
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Validation(w, httpx.FieldErrors{"currentPassword": "Senha atual incorreta."})
	case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooLong):
		httpx.Validation(w, httpx.FieldErrors{"newPassword": err.Error()})
	case errors.Is(err, ErrSamePassword):
		httpx.Validation(w, httpx.FieldErrors{"newPassword": "A nova senha deve ser diferente da atual."})
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Tente novamente mais tarde.")
	default:
		httpx.Internal(w, r, err)
	}
}
