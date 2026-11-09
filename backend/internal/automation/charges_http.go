package automation

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Rotas de cobranças (administrador). A chave de API do gateway entra por aqui e nunca sai.
func (h *Handler) chargeRoutes(r chi.Router) {
	r.Get("/automation/gateways", h.gateways)
	r.Put("/automation/gateways/{provider}", h.putGateway)
	r.Post("/automation/gateways/{provider}/sync", h.syncGateway)
	r.Get("/automation/charges", h.charges)
	r.Put("/automation/charges/{id}/contact", h.linkCharge)
	r.Post("/automation/charges/import", h.importCharges)
}

func (h *Handler) gateways(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Gateways(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"gateways": list})
}

func (h *Handler) putGateway(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey  string `json:"apiKey"`
		Sandbox bool   `json:"sandbox"`
		Enabled bool   `json:"enabled"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.SetGateway(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "provider"), in.APIKey, in.Sandbox, in.Enabled); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) syncGateway(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Sync(r.Context(), auth.MustFromContext(r.Context()).UserID, chi.URLParam(r, "provider"))
	switch {
	case err == nil:
		httpx.JSON(w, http.StatusOK, res)
	case errors.Is(err, ErrGatewayAuth):
		httpx.Error(w, http.StatusBadGateway, "gateway_auth", gatewayErrText(err))
	default:
		var fe httpx.FieldErrors
		if errors.As(err, &fe) {
			h.fail(w, r, err)
			return
		}
		httpx.Error(w, http.StatusBadGateway, "gateway_error", gatewayErrText(err))
	}
}

func (h *Handler) charges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := h.svc.Charges(r.Context(), ChargeFilter{Status: q.Get("status"), ContactID: q.Get("contactId"), Unlinked: q.Get("unlinked") == "1", Provider: q.Get("provider")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"charges": list})
}

func (h *Handler) linkCharge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ContactID string `json:"contactId"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.LinkCharge(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.ContactID); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// importCharges aceita a planilha CSV (multipart, campo "file") ou JSON {"charges":[...]} para quem integra outro gateway por script.
func (h *Handler) importCharges(w http.ResponseWriter, r *http.Request) {
	actor, ip := auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r)
	var rows []ImportRow
	var parseErrs []string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_multipart", "Envio inválido ou arquivo grande demais (máximo 4 MB).")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			httpx.Validation(w, httpx.FieldErrors{"file": "Escolha a planilha CSV."})
			return
		}
		defer f.Close()
		if rows, parseErrs, err = ParseChargesCSV(f); err != nil {
			httpx.Validation(w, httpx.FieldErrors{"file": err.Error()})
			return
		}
	} else {
		var in struct {
			Charges []struct {
				ID          string `json:"id"`
				ContactID   string `json:"contactId"`
				Phone       string `json:"phone"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Amount      string `json:"amount"` // "150,00" ou "150.00"
				DueDate     string `json:"dueDate"`
				Status      string `json:"status"`
				URL         string `json:"url"`
			} `json:"charges"`
		}
		if !httpx.Decode(w, r, &in) {
			return
		}
		for i, c := range in.Charges {
			row := ImportRow{Line: i + 1, ExternalID: c.ID, ContactID: c.ContactID, Phone: c.Phone, Name: c.Name, Description: c.Description, URL: c.URL}
			var ok bool
			if row.AmountCents, ok = ParseMoney(c.Amount); !ok {
				parseErrs = append(parseErrs, "item "+itoa(i+1)+": valor inválido")
				continue
			}
			if row.DueDate, ok = ParseDate(c.DueDate); !ok {
				parseErrs = append(parseErrs, "item "+itoa(i+1)+": vencimento inválido")
				continue
			}
			if row.Status, ok = ParseStatus(c.Status); !ok {
				parseErrs = append(parseErrs, "item "+itoa(i+1)+": situação desconhecida")
				continue
			}
			rows = append(rows, row)
		}
	}
	res, err := h.svc.ImportCharges(r.Context(), actor, ip, rows)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	res.Errors = append(parseErrs, res.Errors...)
	httpx.JSON(w, http.StatusOK, res)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
