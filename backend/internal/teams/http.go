package teams

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes: leitura para usuários autenticados (filtrada por visibilidade);
// escrita apenas para admin.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/teams", h.list)
	r.Get("/teams/{id}", h.get)
	r.Get("/teams/{id}/members", h.members)

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Post("/teams", h.create)
		r.Patch("/teams/{id}", h.update)
		r.Delete("/teams/{id}", h.delete)
	})
	// Gestor: monta a própria equipe (só as equipes de que participa; o serviço confere).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireSupervisor)
		r.Put("/teams/{id}/members/{userId}", h.addMember)
		r.Delete("/teams/{id}/members/{userId}", h.removeMember)
	})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ts, err := h.svc.List(r.Context(), auth.MustFromContext(r.Context()))
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"teams": ts})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), auth.MustFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.Members(r.Context(), auth.MustFromContext(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": ms})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !httpx.Decode(w, r, &in) {
		return
	}
	t, err := h.svc.Create(r.Context(), auth.MustFromContext(r.Context()), httpx.ClientIP(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in Input
	if !httpx.Decode(w, r, &in) {
		return
	}
	t, err := h.svc.Update(r.Context(), auth.MustFromContext(r.Context()), httpx.ClientIP(r), chi.URLParam(r, "id"), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), auth.MustFromContext(r.Context()), httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	err := h.svc.AddMember(r.Context(), auth.MustFromContext(r.Context()), httpx.ClientIP(r),
		chi.URLParam(r, "id"), chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	err := h.svc.RemoveMember(r.Context(), auth.MustFromContext(r.Context()), httpx.ClientIP(r),
		chi.URLParam(r, "id"), chi.URLParam(r, "userId"))
	if err != nil {
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
		httpx.Error(w, http.StatusNotFound, "not_found", "Equipe não encontrada.")
	case errors.Is(err, ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "user_not_found", "Usuário não encontrado.")
	case errors.Is(err, ErrInUse):
		httpx.Error(w, http.StatusConflict, "team_in_use", "Esta equipe está vinculada a filas ou itens da agenda. Remova ou transfira esses vínculos antes de excluir.")
	case errors.Is(err, ErrNameTaken):
		httpx.Validation(w, httpx.FieldErrors{"name": "Já existe uma equipe com esse nome."})
	default:
		httpx.Internal(w, r, err)
	}
}
