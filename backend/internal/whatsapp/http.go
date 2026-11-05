package whatsapp

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type HTTPHandler struct {
	m *Manager
	// sim permite simular mensagens recebidas (somente provedor fake em desenvolvimento).
	sim func(InboundMessage)
}

func NewHTTPHandler(m *Manager, sim func(InboundMessage)) *HTTPHandler {
	return &HTTPHandler{m: m, sim: sim}
}

type connectionView struct {
	Status
	QR *QR `json:"qr,omitempty"`
	// Simulator indica o provedor simulado (desenvolvimento) — habilita o formulário de teste.
	Simulator bool `json:"simulator,omitempty"`
}

// Routes: status para qualquer usuário autenticado; gestão e QR só para admin.
func (h *HTTPHandler) Routes(r chi.Router) {
	r.Get("/whatsapp/connection", h.get)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Post("/whatsapp/connect", h.connect)
		r.Post("/whatsapp/disconnect", h.disconnect)
		if h.sim != nil {
			r.Post("/dev/whatsapp/inbound", h.simulate)
		}
	})
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	v := connectionView{Status: h.m.Status()}
	if auth.MustFromContext(r.Context()).IsAdmin() {
		v.QR = h.m.CurrentQR()
		v.Simulator = h.sim != nil
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (h *HTTPHandler) connect(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	err := h.m.Connect(r.Context(), sess.UserID, httpx.ClientIP(r))
	switch {
	case errors.Is(err, ErrAlreadyConnected):
		httpx.Error(w, http.StatusConflict, "already_connected", "O WhatsApp já está conectado.")
	case err != nil:
		httpx.Error(w, http.StatusBadGateway, "provider_error", "Não foi possível iniciar a conexão com o WhatsApp.")
	default:
		httpx.JSON(w, http.StatusAccepted, connectionView{Status: h.m.Status(), QR: h.m.CurrentQR()})
	}
}

func (h *HTTPHandler) disconnect(w http.ResponseWriter, r *http.Request) {
	sess := auth.MustFromContext(r.Context())
	if err := h.m.Disconnect(r.Context(), sess.UserID, httpx.ClientIP(r)); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, connectionView{Status: h.m.Status()})
}

func (h *HTTPHandler) simulate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
		Name  string `json:"name"`
		Text  string `json:"text"`
		// Mídia simulada (somente desenvolvimento): kind image|audio|document + arquivo em base64.
		Kind     string `json:"kind"`
		File     string `json:"fileBase64"`
		FileName string `json:"fileName"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, in.Phone)
	if in.Kind != "" && in.Kind != "text" {
		data, err := base64.StdEncoding.DecodeString(in.File)
		if err != nil || len(digits) < 10 || (in.Kind != "image" && in.Kind != "audio" && in.Kind != "document") {
			httpx.Validation(w, httpx.FieldErrors{"kind": "Informe kind (image|audio|document), telefone e fileBase64."})
			return
		}
		h.sim(InboundMessage{
			ExternalID: "sim-" + time.Now().Format("20060102150405.000000000"),
			ChatJID:    digits + "@s.whatsapp.net", ContactWAID: digits, ContactPhone: "+" + digits,
			PushName: in.Name, Timestamp: time.Now(), Kind: Kind(in.Kind), Body: in.Text,
			Payload:  map[string]any{"fileName": in.FileName, "sizeBytes": len(data)},
			Download: func(context.Context) ([]byte, error) { return data, nil },
		})
		httpx.NoContent(w)
		return
	}
	if len(digits) < 10 || strings.TrimSpace(in.Text) == "" {
		httpx.Validation(w, httpx.FieldErrors{"phone": "Informe telefone e texto."})
		return
	}
	h.sim(InboundMessage{
		ExternalID: "sim-" + time.Now().Format("20060102150405.000000000"),
		ChatJID:    digits + "@s.whatsapp.net", ContactWAID: digits, ContactPhone: "+" + digits,
		PushName: in.Name, Timestamp: time.Now(), Kind: KindText, Body: in.Text,
	})
	httpx.NoContent(w)
}
