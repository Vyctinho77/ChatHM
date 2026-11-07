package aiaccess

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vyctinho77/chathm/backend/internal/agenda"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/segments"
	"github.com/vyctinho77/chathm/backend/internal/tags"
	"github.com/vyctinho77/chathm/backend/internal/teams"
	"github.com/vyctinho77/chathm/backend/internal/users"
)

type ctxKey struct{}

// MCP é o servidor MCP do ChatHM (POST /mcp, Streamable HTTP sem sessão).
type MCP struct {
	svc            *Service
	convs          *conversations.Service
	segs           *segments.Service
	tags           *tags.Service
	agenda         *agenda.Service
	teams          *teams.Service
	users          *users.Service
	allowedOrigins map[string]bool
	log            *slog.Logger
	handler        http.Handler
	base           BaseURLFunc
}

// SetBaseURL informa o endereço público (para anunciar os metadados OAuth nos 401).
func (m *MCP) SetBaseURL(b BaseURLFunc) { m.base = b }

// challenge monta o WWW-Authenticate; com OAuth ligado aponta para os metadados (RFC 9728).
func (m *MCP) challenge(r *http.Request, invalid bool) string {
	h := `Bearer realm="chathm"`
	if invalid {
		h += `, error="invalid_token"`
	}
	if m.base != nil {
		if st, err := m.svc.Settings(r.Context()); err == nil && st.Enabled && st.AllowOAuth {
			h += `, resource_metadata="` + m.base(r) + `/.well-known/oauth-protected-resource"`
		}
	}
	return h
}

func NewMCP(svc *Service, convs *conversations.Service, segs *segments.Service, tg *tags.Service, ag *agenda.Service, tm *teams.Service, us *users.Service, allowedOrigins []string, log *slog.Logger) *MCP {
	m := &MCP{svc: svc, convs: convs, segs: segs, tags: tg, agenda: ag, teams: tm, users: us, allowedOrigins: map[string]bool{}, log: log}
	for _, o := range allowedOrigins {
		m.allowedOrigins[strings.TrimRight(o, "/")] = true
	}
	m.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		p, _ := r.Context().Value(ctxKey{}).(Principal)
		return m.server(p)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return m
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// ServeHTTP autentica o token Bearer e entrega a requisição ao servidor MCP.
func (m *MCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// Contra DNS rebinding: navegadores enviam Origin; clientes de IA em servidor não.
	if o := r.Header.Get("Origin"); o != "" && !m.allowedOrigins[strings.TrimRight(o, "/")] {
		writeJSONError(w, http.StatusForbidden, "origin_not_allowed", "Origem não permitida.")
		return
	}
	authz := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(authz, "Bearer ")
	if !ok || strings.TrimSpace(raw) == "" {
		w.Header().Set("WWW-Authenticate", m.challenge(r, false))
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Envie o token de acesso no cabeçalho Authorization: Bearer.")
		return
	}
	p, err := m.svc.Authenticate(r.Context(), strings.TrimSpace(raw))
	switch {
	case errors.Is(err, ErrDisabled):
		writeJSONError(w, http.StatusForbidden, "ai_disabled", "O acesso por IA está desligado neste escritório.")
		return
	case errors.Is(err, ErrBadToken):
		w.Header().Set("WWW-Authenticate", m.challenge(r, true))
		writeJSONError(w, http.StatusUnauthorized, "invalid_token", "Token inválido, expirado ou revogado.")
		return
	case err != nil:
		m.log.Error("mcp: autenticando token", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal", "Erro interno.")
		return
	}
	if !m.svc.Allow(p.TokenID) {
		w.Header().Set("Retry-After", "30")
		writeJSONError(w, http.StatusTooManyRequests, "rate_limited", "Muitas chamadas. Tente de novo em instantes.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	m.handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
}
