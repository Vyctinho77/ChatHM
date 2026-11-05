package conversations

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Routes: usuários autenticados (Fase 2: fila geral visível a todos).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/conversations", h.list)
	r.Get("/conversations/{id}", h.get)
	r.Get("/conversations/{id}/messages", h.messages)
	r.Post("/conversations/{id}/read", h.read)
	r.Patch("/contacts/{id}", h.updateContact)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"conversations": cs})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.Messages(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": ms})
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkRead(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) updateContact(w http.ResponseWriter, r *http.Request) {
	var in ContactInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.UpdateContact(r.Context(), chi.URLParam(r, "id"), in); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	default:
		httpx.Internal(w, r, err)
	}
}
