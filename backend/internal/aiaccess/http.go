package aiaccess

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Handler expõe a gestão do acesso por IA (sessão do app): configuração e tokens.
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes: usuários autenticados (MFA completo). Só o administrador altera a configuração.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/ai/settings", h.getSettings)
	r.With(auth.RequireAdmin).Put("/ai/settings", h.putSettings)
	r.Get("/ai/tokens", h.listTokens)
	r.Post("/ai/tokens", h.createToken)
	r.Delete("/ai/tokens/{id}", h.revokeToken)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusConflict, "ai_disabled", "O acesso por IA está desligado. Peça a um administrador para ligar em Configurações → Integrações.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	default:
		httpx.Internal(w, r, err)
	}
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) putSettings(w http.ResponseWriter, r *http.Request) {
	var in Settings
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	st, err := h.svc.SetSettings(r.Context(), sess.UserID, clientIP(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	all := sess.IsAdmin() && r.URL.Query().Get("all") == "1"
	list, err := h.svc.List(r.Context(), sess.UserID, all)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tokens": list})
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string   `json:"name"`
		ExpiresInDays int      `json:"expiresInDays"`
		Scopes        []string `json:"scopes"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	raw, t, err := h.svc.Create(r.Context(), sess.UserID, clientIP(r), in.Name, in.ExpiresInDays, in.Scopes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// O token completo só aparece nesta resposta.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusCreated, map[string]any{"token": raw, "info": t})
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.Revoke(r.Context(), sess.UserID, sess.IsAdmin(), clientIP(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func clientIP(r *http.Request) string {
	return httpx.ClientIP(r)
}
