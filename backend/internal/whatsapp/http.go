package whatsapp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type HTTPHandler struct {
	m *Manager
	// gate libera o pareamento da instalação inicial (sem administrador) e confere o código de instalação.
	gate func(ctx context.Context, installCode, ip string) error
	mu   sync.Mutex
	key  string // chave da tela que iniciou o pareamento: só ela vê o QR
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
		r.Post("/whatsapp/history", h.requestHistory)
		if h.sim != nil {
			r.Post("/dev/whatsapp/inbound", h.simulate)
			r.Post("/dev/whatsapp/typing", h.simulateTyping)
		}
	})
}

func (h *HTTPHandler) requestHistory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// Corpo opcional: {"days": 1-365}. Sem corpo, pede o padrão (30 dias).
	var in struct {
		Days int `json:"days"`
	}
	if r.ContentLength != 0 && !httpx.Decode(w, r, &in) {
		return
	}
	if in.Days < 0 || in.Days > MaxHistoryDays {
		httpx.Validation(w, httpx.FieldErrors{"days": "Escolha entre 1 e 365 dias."})
		return
	}
	err := h.m.RequestHistory(ctx, in.Days)
	switch {
	case errors.Is(err, ErrNotConnected):
		httpx.Error(w, 409, "whatsapp_unavailable", "Conecte o WhatsApp antes de solicitar o histórico.")
	case errors.Is(err, ErrBusy):
		w.Header().Set("Retry-After", "60")
		httpx.Error(w, 429, "too_many_attempts", "Aguarde um minuto antes de solicitar novamente.")
	case err != nil:
		httpx.Error(w, 502, "provider_error", "Não foi possível enviar a solicitação ao WhatsApp.")
	default:
		httpx.JSON(w, 202, map[string]string{"status": "requested"})
	}
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

func (h *HTTPHandler) simulateTyping(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
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
	if len(digits) < 10 || len(digits) > 15 {
		httpx.Validation(w, httpx.FieldErrors{"phone": "Informe um telefone com DDI e DDD."})
		return
	}
	if !h.m.Connected() {
		httpx.Error(w, http.StatusConflict, "not_connected", "Conecte o WhatsApp simulado para testar.")
		return
	}
	h.m.OnTyping(Typing{ChatJID: digits + "@s.whatsapp.net", ContactWAID: digits, SenderID: "contact", Active: true})
	httpx.NoContent(w)
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
		if err != nil || len(digits) < 10 || (in.Kind != "image" && in.Kind != "audio" && in.Kind != "document" && in.Kind != "sticker") {
			httpx.Validation(w, httpx.FieldErrors{"kind": "Informe kind (image|audio|document|sticker), telefone e fileBase64."})
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

// SetSetupGate liga a instalação inicial por QR (auth.Service.CheckSetupOpen).
func (h *HTTPHandler) SetSetupGate(g func(ctx context.Context, installCode, ip string) error) {
	h.gate = g
}

// PublicRoutes: pareamento da instalação inicial, sem sessão. Só funciona enquanto o escritório não tem
// administrador; depois disso responde 409 (o QR da tela de entrada some e o backend recusa).
func (h *HTTPHandler) PublicRoutes(r chi.Router, checkOrigin func(http.Handler) http.Handler) {
	if h.gate == nil {
		return
	}
	r.With(checkOrigin).Post("/setup/whatsapp/connect", h.setupConnect)
	r.With(checkOrigin).Post("/setup/whatsapp/status", h.setupStatus)
}

func (h *HTTPHandler) setupGateError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, auth.ErrSetupDone):
		httpx.Error(w, http.StatusConflict, "setup_done", "O escritório já tem um administrador. Entre com sua conta.")
	case errors.Is(err, auth.ErrSetupForbidden):
		httpx.Error(w, http.StatusForbidden, "setup_forbidden", "Código de instalação inválido.")
	case errors.Is(err, auth.ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Tente novamente mais tarde.")
	default:
		httpx.Internal(w, r, err)
	}
	return true
}

func (h *HTTPHandler) setupConnect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InstallCode string `json:"installCode"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if h.setupGateError(w, r, h.gate(r.Context(), in.InstallCode, httpx.ClientIP(r))) {
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	key := base64.RawURLEncoding.EncodeToString(b)
	h.mu.Lock()
	h.key = key
	h.mu.Unlock()
	if err := h.m.Connect(r.Context(), "", httpx.ClientIP(r)); err != nil && !errors.Is(err, ErrAlreadyConnected) {
		httpx.Error(w, http.StatusBadGateway, "provider_error", "Não foi possível iniciar a conexão com o WhatsApp.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"key": key})
}

// setupStatus devolve o estado e o QR a quem iniciou o pareamento (a chave sai no corpo, nunca na URL).
func (h *HTTPHandler) setupStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key         string `json:"key"`
		InstallCode string `json:"installCode"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if h.setupGateError(w, r, h.gate(r.Context(), in.InstallCode, httpx.ClientIP(r))) {
		return
	}
	h.mu.Lock()
	ok := h.key != "" && subtle.ConstantTimeCompare([]byte(in.Key), []byte(h.key)) == 1
	h.mu.Unlock()
	if !ok {
		httpx.Error(w, http.StatusForbidden, "setup_forbidden", "Reinicie a conexão.")
		return
	}
	v := connectionView{Status: h.m.Status(), QR: h.m.CurrentQR()}
	httpx.JSON(w, http.StatusOK, v)
}
