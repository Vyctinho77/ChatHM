package automation

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct {
	svc           *Service
	whatsappReady func() bool
}

func NewHandler(svc *Service, whatsappReady func() bool) *Handler {
	return &Handler{svc: svc, whatsappReady: whatsappReady}
}

// Routes: tudo é do administrador (a aprovação do que a IA quer mandar ao cliente é decisão dele), exceto marcar o opt-out do cliente.
func (h *Handler) Routes(r chi.Router) {
	r.With(auth.RequireSupervisor).Get("/automation/contacts/{id}", h.contactState)
	r.With(auth.RequireSupervisor).Put("/automation/contacts/{id}/opt-out", h.setOptOut)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/automation/overview", h.overview)
		r.Get("/automation/settings", h.settings)
		r.Put("/automation/settings", h.putSettings)
		r.Post("/automation/pause", h.pause)
		r.Get("/automation/categories", h.categories)
		r.Put("/automation/categories/{key}", h.putCategory)
		r.Get("/automation/sends", h.sends)
		r.Post("/automation/sends/approve", h.approve)
		r.Post("/automation/sends/reject", h.reject)
		r.Post("/automation/sends/cancel", h.cancel)
		r.Post("/automation/sends/{id}/retry", h.retry)
		r.Get("/automation/documents", h.documents)
		r.Post("/automation/documents", h.upload)
		r.Delete("/automation/documents/{id}", h.deleteDocument)
		h.chargeRoutes(r)
	})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	var ref *Refusal
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.As(err, &ref):
		httpx.Error(w, http.StatusUnprocessableEntity, ref.Code, ref.Message)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	case errors.Is(err, ErrState):
		httpx.Error(w, http.StatusConflict, "invalid_state", "Isso não é possível no estado atual (o item pode estar em uso ou já ter saído).")
	default:
		httpx.Internal(w, r, err)
	}
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.svc.Overview(r.Context(), h.whatsappReady())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
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
	st, err := h.svc.SetSettings(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paused bool   `json:"paused"`
		Reason string `json:"reason"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.SetPaused(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), in.Paused, in.Reason); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Categories(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"categories": list})
}

func (h *Handler) putCategory(w http.ResponseWriter, r *http.Request) {
	var in Category
	if !httpx.Decode(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "key"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) sends(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := h.svc.List(r.Context(), ListFilter{Status: q.Get("status"), ContactID: q.Get("contactId"), Limit: atoi(q.Get("limit"))})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sends": list})
}

func atoi(s string) int {
	n := 0
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs   []string          `json:"ids"`
		Edits map[string]string `json:"edits"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	ok, refused, err := h.svc.Approve(r.Context(), sess.UserID, httpx.ClientIP(r), in.IDs, in.Edits)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"approved": ok, "refused": refused})
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Reason string   `json:"reason"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	n, err := h.svc.Reject(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), in.IDs, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"rejected": n})
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	n, err := h.svc.Cancel(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), in.IDs, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"cancelled": n})
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Retry(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) documents(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Documents(r.Context(), r.URL.Query().Get("contactId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"documents": list})
}

// upload: multipart com os campos name, ownerContactId, shareable e o arquivo em "file".
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	limit := h.svc.media.Max
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido.")
		return
	}
	var name, owner, fileName string
	var shareable bool
	var data []byte
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				httpx.Error(w, http.StatusRequestEntityTooLarge, "attachment_too_large", fmt.Sprintf("O arquivo passa do limite de %d MB.", limit>>20))
			} else {
				httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido.")
			}
			return
		}
		switch part.FormName() {
		case "file":
			fileName = part.FileName()
			data, err = io.ReadAll(io.LimitReader(part, limit+1))
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido.")
				return
			}
		default:
			val, _ := io.ReadAll(io.LimitReader(part, 4<<10))
			switch part.FormName() {
			case "name":
				name = string(val)
			case "ownerContactId":
				owner = string(val)
			case "shareable":
				shareable = string(val) == "true" || string(val) == "1"
			}
		}
	}
	if int64(len(data)) > limit {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "attachment_too_large", fmt.Sprintf("O arquivo passa do limite de %d MB.", limit>>20))
		return
	}
	if name == "" {
		name = fileName
	}
	d, err := h.svc.UploadDocument(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), name, data, owner, shareable)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteDocument(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// --- opt-out do cliente (equipe) ---------------------------------------------

func (h *Handler) contactState(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ContactState(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) setOptOut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OptOut bool `json:"optOut"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.SetOptOut(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.OptOut); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
