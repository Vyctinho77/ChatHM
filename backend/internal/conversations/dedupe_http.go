package conversations

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// AdminRoutes: manutenção dos dados (só administrador).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/maintenance/duplicates", h.dedupePreview)
	r.Post("/maintenance/duplicates", h.dedupeApply)
}

// dedupePreview conta o que seria corrigido, sem gravar nada.
func (h *Handler) dedupePreview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	rep, err := h.svc.Dedupe(ctx, false)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

// dedupeApply corrige de verdade; exige a confirmação digitada.
func (h *Handler) dedupeApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(strings.ToUpper(in.Confirm)) != "CORRIGIR" {
		httpx.Validation(w, httpx.FieldErrors{"confirm": "Digite CORRIGIR para confirmar."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	rep, err := h.svc.Dedupe(ctx, true)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}
