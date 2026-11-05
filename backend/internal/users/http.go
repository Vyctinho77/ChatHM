package users

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SelfRoutes: qualquer usuário autenticado.
func (h *Handler) SelfRoutes(r chi.Router) {
	r.Patch("/me/preferences", h.updatePreferences)
}

// AdminRoutes: montar atrás de auth.RequireAdmin.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/users", h.list)
	r.Post("/users", h.create)
	r.Get("/users/{id}", h.get)
	r.Patch("/users/{id}", h.update)
	r.Post("/users/{id}/password", h.resetPassword)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	us, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": us})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	u, err := h.svc.Create(r.Context(), sess.UserID, httpx.ClientIP(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, u)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	u, err := h.svc.Update(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.ResetPassword(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Password); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Theme string `json:"theme"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.SetTheme(r.Context(), auth.MustFromContext(r.Context()).UserID, in.Theme); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"theme": in.Theme})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Usuário não encontrado.")
	case errors.Is(err, ErrEmailTaken):
		httpx.Validation(w, httpx.FieldErrors{"email": "E-mail já cadastrado."})
	case errors.Is(err, ErrTeamNotFound):
		httpx.Validation(w, httpx.FieldErrors{"teamIds": "Equipe não encontrada."})
	case errors.Is(err, ErrLastAdmin):
		httpx.Error(w, http.StatusConflict, "last_admin", "Deve existir ao menos um administrador ativo.")
	case errors.Is(err, ErrSelfDeactivate):
		httpx.Error(w, http.StatusConflict, "self_deactivate", "Você não pode desativar o próprio usuário.")
	default:
		httpx.Internal(w, r, err)
	}
}
