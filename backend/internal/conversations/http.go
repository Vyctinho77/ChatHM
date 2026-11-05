package conversations

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

func viewer(r *http.Request) Viewer {
	sess := auth.MustFromContext(r.Context())
	return Viewer{ID: sess.UserID, Name: sess.Name, Admin: sess.IsAdmin()}
}

// Routes: usuários autenticados. A visibilidade por equipe é aplicada em cada consulta.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/conversations", h.list)
	r.Get("/conversations/{id}", h.get)
	r.Get("/conversations/{id}/messages", h.messages)
	r.Post("/conversations/{id}/messages", h.send)
	r.Post("/conversations/{id}/read", h.read)
	r.Post("/conversations/{id}/close", h.close)
	r.Post("/conversations/{id}/transfer", h.transfer)
	r.Put("/conversations/{id}/tags/{tagId}", h.addTag)
	r.Delete("/conversations/{id}/tags/{tagId}", h.removeTag)
	r.Patch("/contacts/{id}", h.updateContact)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.List(r.Context(), viewer(r))
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"conversations": cs})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	ms, err := h.svc.Messages(r.Context(), viewer(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": ms})
}

// send: 201 mensagem nova; 200 repetição de um envio já feito (idempotente).
// A mensagem devolvida pode estar com status "failed" se o WhatsApp recusou o envio:
// reenviar com o mesmo id tenta de novo sem duplicar.
func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	m, created, err := h.svc.SendText(r.Context(), viewer(r), chi.URLParam(r, "id"), SendInput{ID: in.ID, Text: in.Text})
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, m)
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.MarkRead(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Close(r.Context(), viewer(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// transfer passa o atendimento para outra pessoa do escritório (corpo: {userId}).
func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"userId"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	err := h.svc.Transfer(r.Context(), viewer(r), chi.URLParam(r, "id"), in.UserID)
	var taken *TakenError
	if errors.As(err, &taken) {
		msg := "Só quem está atendendo (ou um administrador) pode transferir esta conversa."
		if taken.Assignee != "" {
			msg = taken.Assignee + " está atendendo; só ela (ou um administrador) pode transferir esta conversa."
		}
		httpx.Error(w, http.StatusConflict, "conversation_taken", msg)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) addTag(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.AddTag(r.Context(), viewer(r), chi.URLParam(r, "id"), chi.URLParam(r, "tagId")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) removeTag(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveTag(r.Context(), viewer(r), chi.URLParam(r, "id"), chi.URLParam(r, "tagId")); err != nil {
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
	if err := h.svc.UpdateContact(r.Context(), viewer(r), chi.URLParam(r, "id"), in); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	var taken *TakenError
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.As(err, &taken):
		msg := "Outra pessoa já assumiu este atendimento."
		if taken.Assignee != "" {
			msg = taken.Assignee + " já assumiu este atendimento."
		}
		httpx.Error(w, http.StatusConflict, "conversation_taken", msg)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	case errors.Is(err, ErrClosed):
		httpx.Error(w, http.StatusConflict, "conversation_closed", "Este atendimento está encerrado. Aguarde uma nova mensagem do cliente.")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden", "Somente o responsável pelo atendimento pode fazer isso.")
	case errors.Is(err, ErrUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, "whatsapp_unavailable", "O WhatsApp está desconectado. Peça a um administrador para reconectar.")
	case errors.Is(err, ErrIDConflict):
		httpx.Error(w, http.StatusConflict, "id_conflict", "Identificador de mensagem já utilizado.")
	case errors.Is(err, ErrTagNotFound):
		httpx.Error(w, http.StatusNotFound, "tag_not_found", "Etiqueta não encontrada.")
	default:
		httpx.Internal(w, r, err)
	}
}
