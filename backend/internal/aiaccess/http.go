package aiaccess

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Handler expõe a gestão do acesso por IA (sessão do app): configuração e tokens.
type Handler struct {
	svc  *Service
	base BaseURLFunc
}

func NewHandler(svc *Service, base BaseURLFunc) *Handler { return &Handler{svc: svc, base: base} }

// Routes: usuários autenticados (MFA completo). Só o administrador altera a configuração.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/ai/settings", h.getSettings)
	r.With(auth.RequireAdmin).Put("/ai/settings", h.putSettings)
	r.With(auth.RequireAdmin).Get("/ai/webhook", h.getWebhook)
	r.With(auth.RequireAdmin).Put("/ai/webhook", h.putWebhook)
	r.With(auth.RequireAdmin).Post("/ai/webhook/secret", h.rotateWebhookSecret)
	r.With(auth.RequireAdmin).Post("/ai/webhook/test", h.testWebhook)
	r.Get("/ai/tokens", h.listTokens)
	r.Post("/ai/tokens", h.createToken)
	r.Delete("/ai/tokens/{id}", h.revokeToken)
	r.Get("/ai/oauth/preview", h.oauthPreview)
	r.Post("/ai/oauth/authorize", h.oauthAuthorize)
	r.Get("/ai/grants", h.listGrants)
	r.Delete("/ai/grants/{id}", h.revokeGrant)
	r.Get("/ai/suggestions", h.listSuggestions)
	r.Post("/ai/suggestions/{id}/send", h.sendSuggestion)
	r.Post("/ai/suggestions/{id}/use", h.useSuggestion)
	r.Post("/ai/suggestions/{id}/discard", h.discardSuggestion)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	var taken *conversations.TakenError
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.As(err, &taken):
		httpx.Error(w, http.StatusConflict, "conversation_taken", "Outra pessoa já assumiu este atendimento.")
	case errors.Is(err, ErrSuggestionGone):
		httpx.Error(w, http.StatusNotFound, "suggestion_gone", "Esta sugestão não existe mais (já foi decidida ou expirou).")
	case errors.Is(err, conversations.ErrClosed):
		httpx.Error(w, http.StatusConflict, "conversation_closed", "Este atendimento está encerrado.")
	case errors.Is(err, conversations.ErrUnavailable):
		httpx.Error(w, http.StatusServiceUnavailable, "whatsapp_unavailable", "O WhatsApp está desconectado.")
	case errors.Is(err, conversations.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Registro não encontrado.")
	case errors.Is(err, ErrDisabled):
		httpx.Error(w, http.StatusConflict, "ai_disabled", "O acesso por IA está desligado. Peça a um administrador para ligar na aba MCP.")
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
		Provider      string   `json:"provider"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	for _, sc := range in.Scopes {
		if sc == ScopeAttend && sess.Role != auth.RoleAdmin {
			httpx.Error(w, http.StatusForbidden, "forbidden", "Só administradores criam tokens de atendente autônomo.")
			return
		}
	}
	raw, t, err := h.svc.Create(r.Context(), sess.UserID, clientIP(r), in.Name, in.ExpiresInDays, in.Scopes, in.Provider)
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

func (h *Handler) listSuggestions(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	list, err := h.svc.Suggestions(r.Context(), sess.UserID, r.URL.Query().Get("conversationId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suggestions": list})
}

// sendSuggestion: a pessoa aprova (opcionalmente com o texto editado) e a mensagem sai como dela.
func (h *Handler) sendSuggestion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	msg, err := h.svc.Send(r.Context(), conversations.Viewer{ID: sess.UserID, Name: sess.AuthorName(), Admin: sess.IsAdmin(), Supervisor: sess.IsManager()}, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Text)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msg)
}

// useSuggestion: devolve o texto para a pessoa editar no campo de mensagem.
func (h *Handler) useSuggestion(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	sg, err := h.svc.Use(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sg)
}

func (h *Handler) discardSuggestion(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.svc.Discard(r.Context(), sess.UserID, httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// --- aviso automático (webhook): só administrador --------------------------------------

func (h *Handler) getWebhook(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Webhook(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

// putWebhook salva a configuração. Na primeira vez devolve também o segredo (uma única vez).
func (h *Handler) putWebhook(w http.ResponseWriter, r *http.Request) {
	var in WebhookInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	sess := auth.MustFromContext(r.Context())
	c, secret, err := h.svc.SetWebhook(r.Context(), sess.UserID, httpx.ClientIP(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := map[string]any{"webhook": c}
	if secret != "" {
		out["secret"] = secret
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) rotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	secret, err := h.svc.RotateWebhookSecret(r.Context(), sess.UserID, httpx.ClientIP(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"secret": secret})
}

func (h *Handler) testWebhook(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.TestWebhook(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
