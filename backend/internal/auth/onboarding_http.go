package auth

import (
	"errors"
	"net/http"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

func (s *Service) handleOptions(w http.ResponseWriter, r *http.Request) {
	needs, err := s.NeedsSetup(r.Context())
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"selfRegister": s.cfg.AllowSelfRegister,
		"needsSetup":   needs,
		"setupCode":    needs && s.cfg.SetupToken != "", // a tela pede o código de instalação
	})
}

func (s *Service) handleSetupStatus(w http.ResponseWriter, r *http.Request) { s.handleOptions(w, r) }

// writeOnboardingError traduz os erros comuns dos fluxos de convite/configuração.
func (s *Service) writeOnboardingError(w http.ResponseWriter, r *http.Request, err error) {
	var fields httpx.FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.Validation(w, fields)
	case errors.Is(err, ErrInvalidInvite):
		httpx.Error(w, http.StatusUnauthorized, "invalid_invite", "Convite inválido ou expirado. Peça um novo código ao administrador.")
	case errors.Is(err, ErrInvalidCode):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_code", "Código inválido. Confira o horário do celular e tente com o código atual.")
	case errors.Is(err, ErrSetupDone):
		httpx.Error(w, http.StatusConflict, "setup_done", "A configuração inicial já foi concluída. Entre com sua conta.")
	case errors.Is(err, ErrSetupForbidden):
		httpx.Error(w, http.StatusForbidden, "setup_forbidden", "Código de instalação inválido.")
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Tente novamente mais tarde.")
	default:
		httpx.Internal(w, r, err)
	}
}

type sessionWithCodes struct {
	MeResponse
	RecoveryCodes []string `json:"recoveryCodes"`
}

func (s *Service) handleSetupStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Email       string `json:"email"`
		InstallCode string `json:"installCode"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	e, err := s.SetupStart(r.Context(), in.Name, in.Email, in.InstallCode, ClientFrom(r))
	if err != nil {
		s.writeOnboardingError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (s *Service) handleSetupFinish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	tok, sess, codes, err := s.SetupFinish(r.Context(), in.Token, in.Code, ClientFrom(r))
	if err != nil {
		s.writeOnboardingError(w, r, err)
		return
	}
	s.setCookie(w, tok)
	httpx.JSON(w, http.StatusCreated, sessionWithCodes{s.me(sess), codes})
}

type enrollInput struct {
	Email      string `json:"email"`
	InviteCode string `json:"inviteCode"`
	Token      string `json:"token"`
	Code       string `json:"code"`
}

func (s *Service) handleEnrollStart(w http.ResponseWriter, r *http.Request) {
	var in enrollInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	e, err := s.EnrollStart(r.Context(), in.Email, in.InviteCode, ClientFrom(r))
	if err != nil {
		s.writeOnboardingError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (s *Service) handleEnrollFinish(w http.ResponseWriter, r *http.Request) {
	var in enrollInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	tok, sess, codes, err := s.EnrollFinish(r.Context(), in.Email, in.InviteCode, in.Token, in.Code, ClientFrom(r))
	if err != nil {
		s.writeOnboardingError(w, r, err)
		return
	}
	s.setCookie(w, tok)
	httpx.JSON(w, http.StatusCreated, sessionWithCodes{s.me(sess), codes})
}

func (s *Service) handleRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := s.RegenerateRecoveryCodes(r.Context(), MustFromContext(r.Context()), ClientFrom(r))
	if errors.Is(err, ErrMFANotEnabled) {
		httpx.Error(w, http.StatusConflict, "mfa_not_enabled", "Ative o autenticador antes de gerar códigos de recuperação.")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}
