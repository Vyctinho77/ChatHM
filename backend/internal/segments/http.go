package segments

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Routes: qualquer pessoa vê os segmentos e testa uma mensagem; as palavras-chave são do administrador e dos integrantes da equipe
// do segmento (cada um mexe nas do seu); estrutura, vínculo de equipe e prioridade são só do administrador.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/segments", h.list)
	r.Post("/segments/classify", h.classify)
	r.Get("/segments/{id}/suggestions", h.suggestions)
	r.Post("/segments/{id}/keywords", h.addKeyword)
	r.Post("/segments/{id}/keywords/bulk", h.addKeywords)
	r.Delete("/segments/{id}/keywords/{kid}", h.deleteKeyword)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Post("/segments", h.create)
		r.Post("/segments/examples", h.examples)
		r.Patch("/segments/{id}", h.update)
		r.Put("/segments/{id}/team", h.setTeam)
		r.Delete("/segments/{id}", h.delete)
		r.Patch("/segments/{id}/keywords/{kid}", h.updateKeyword)
	})
}

// canEdit confere se quem pede pode mexer nas palavras-chave do segmento; responde 403/404 quando não pode.
func (h *Handler) canEdit(w http.ResponseWriter, r *http.Request) (sess *auth.Session, ok bool) {
	sess = auth.MustFromContext(r.Context())
	allowed, err := h.svc.CanEditKeywords(r.Context(), sess.UserID, sess.IsAdmin(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return sess, false
	}
	if !allowed {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Só o administrador ou quem é da equipe deste segmento pode mexer nas palavras-chave.")
		return sess, false
	}
	return sess, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	segs, err := h.svc.ListForUser(r.Context(), sess.UserID, sess.IsAdmin())
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"segments": segs})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string  `json:"name"`
		TeamID *string `json:"teamId"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	sg, err := h.svc.Create(r.Context(), sess.UserID, httpx.ClientIP(r), in.Name, in.TeamID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sg)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        *string `json:"name"`
		IsActive    *bool   `json:"isActive"`
		AutoInclude *bool   `json:"autoInclude"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	sg, err := h.svc.Update(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Name, in.IsActive, in.AutoInclude)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sg)
}

func (h *Handler) setTeam(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TeamID *string `json:"teamId"` // null desvincula
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	sg, err := h.svc.SetTeam(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.TeamID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sg)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.Delete(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) addKeyword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phrase   string `json:"phrase"`
		Priority int    `json:"priority"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess, ok := h.canEdit(w, r)
	if !ok {
		return
	}
	if !sess.IsAdmin() {
		in.Priority = DefaultPriority // só o administrador escolhe a prioridade
	}
	k, err := h.svc.AddKeyword(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Phrase, in.Priority)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, k)
}

func (h *Handler) addKeywords(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phrases  []string `json:"phrases"`
		Priority int      `json:"priority"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess, ok := h.canEdit(w, r)
	if !ok {
		return
	}
	if !sess.IsAdmin() || in.Priority == 0 {
		in.Priority = DefaultPriority
	}
	res, err := h.svc.AddKeywords(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Phrases, in.Priority)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.canEdit(w, r); !ok {
		return
	}
	list, err := h.svc.Suggestions(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suggestions": list})
}

func (h *Handler) updateKeyword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Priority *int `json:"priority"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Priority == nil {
		httpx.Validation(w, httpx.FieldErrors{"priority": "Informe a prioridade."})
		return
	}
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.UpdateKeywordPriority(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), chi.URLParam(r, "kid"), *in.Priority); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) deleteKeyword(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.canEdit(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteKeyword(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), chi.URLParam(r, "kid")); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// classify permite testar uma frase contra as palavras-chave atuais (qualquer pessoa: ajuda a equipe a conferir o que cadastrou).
func (h *Handler) classify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	res, err := h.svc.ClassifyText(r.Context(), in.Text)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Segmento não encontrado.")
	case errors.Is(err, ErrKeywordGone):
		httpx.Error(w, http.StatusNotFound, "keyword_not_found", "Palavra-chave não encontrada.")
	case errors.Is(err, ErrNameTaken):
		httpx.Validation(w, httpx.FieldErrors{"name": "Já existe um segmento com esse nome."})
	case errors.Is(err, ErrTeamNotFound):
		httpx.Validation(w, httpx.FieldErrors{"teamId": "Equipe não encontrada."})
	case errors.Is(err, ErrDuplicate):
		httpx.Validation(w, httpx.FieldErrors{"phrase": "Essa palavra já está cadastrada neste segmento."})
	default:
		httpx.Internal(w, r, err)
	}
}

// examples importa os segmentos de exemplo (somente admin); nunca sobrescreve o que já existe.
func (h *Handler) examples(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.ApplyExamples(r.Context(), sess.UserID, httpx.ClientIP(r)); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.NoContent(w)
}
