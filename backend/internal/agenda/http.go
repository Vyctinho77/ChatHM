package agenda

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"net/http"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }
func (h *Handler) Routes(r chi.Router) {
	r.Get("/agenda", h.list)
	r.Put("/agenda/{id}", h.save)
	r.Patch("/agenda/{id}/done", h.done)
	r.Delete("/agenda/{id}", h.delete)
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fields httpx.FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.Validation(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, 404, "not_found", "Item ou equipe não encontrado.")
	default:
		httpx.Internal(w, r, err)
	}
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), auth.MustFromContext(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items})
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !httpx.Decode(w, r, &in) {
		return
	}
	item, err := h.svc.Save(r.Context(), auth.MustFromContext(r.Context()), chi.URLParam(r, "id"), httpx.ClientIP(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, 200, item)
}
func (h *Handler) done(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Done *bool `json:"done"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Done == nil {
		httpx.Validation(w, httpx.FieldErrors{"done": "Informe se o item foi concluído."})
		return
	}
	if err := h.svc.Mutate(r.Context(), auth.MustFromContext(r.Context()), chi.URLParam(r, "id"), httpx.ClientIP(r), in.Done); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Mutate(r.Context(), auth.MustFromContext(r.Context()), chi.URLParam(r, "id"), httpx.ClientIP(r), nil); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}
