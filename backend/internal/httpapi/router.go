// Package httpapi monta o roteador HTTP, os middlewares transversais e os módulos.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/agenda"
	"github.com/vyctinho77/chathm/backend/internal/aiaccess"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/automation"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/instance"
	"github.com/vyctinho77/chathm/backend/internal/internalchat"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
	"github.com/vyctinho77/chathm/backend/internal/segments"
	"github.com/vyctinho77/chathm/backend/internal/tags"
	"github.com/vyctinho77/chathm/backend/internal/teams"
	"github.com/vyctinho77/chathm/backend/internal/users"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

type Deps struct {
	Config config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Auth   *auth.Service
	Users  *users.Service
	Teams  *teams.Service

	Hub           *realtime.Hub
	Conversations *conversations.Service
	Segments      *segments.Service
	Tags          *tags.Service
	WhatsApp      *whatsapp.Manager
	Instance      *instance.Service
	AI            *aiaccess.Service
	Automation    *automation.Service
	InternalChat  *internalchat.Service
	// Simulate habilita POST /dev/whatsapp/inbound (provedor fake em development).
	Simulate func(whatsapp.InboundMessage)
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	if d.Config.TrustProxyHeaders {
		r.Use(middleware.RealIP)
	}
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   d.Config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", auth.CSRFHeader},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := d.DB.Ping(ctx); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	usersH := users.NewHandler(d.Users)
	teamsH := teams.NewHandler(d.Teams)
	convsH := conversations.NewHandler(d.Conversations)
	segmentsH := segments.NewHandler(d.Segments)
	tagsH := tags.NewHandler(d.Tags)
	waH := whatsapp.NewHTTPHandler(d.WhatsApp, d.Simulate)
	waH.SetSetupGate(d.Auth.CheckSetupOpen)
	agendaSvc := agenda.NewService(d.DB, d.Conversations)
	agendaH := agenda.NewHandler(agendaSvc)
	aiSvc := d.AI
	if aiSvc == nil {
		aiSvc = aiaccess.NewService(d.DB)
	}
	baseURL := aiaccess.PublicBase(d.Config.AllowedOrigins)
	aiH := aiaccess.NewHandler(aiSvc, baseURL)
	autoSvc := d.Automation
	if autoSvc == nil {
		autoSvc = automation.NewService(d.DB, d.Conversations, nil, nil, d.Logger)
	}
	autoH := automation.NewHandler(autoSvc, d.WhatsApp.Connected)
	chatSvc := d.InternalChat
	if chatSvc == nil {
		chatSvc = internalchat.NewService(d.DB, d.Hub) // sem armazenamento: anexos indisponíveis
	}
	chatH := internalchat.NewHandler(chatSvc)
	oauthH := aiaccess.NewOAuthHandler(aiSvc, baseURL)
	mcpH := aiaccess.NewMCP(aiSvc, d.Conversations, d.Segments, d.Tags, agendaSvc, d.Teams, d.Users, autoSvc, d.Config.AllowedOrigins, d.Logger)

	// O WebSocket revalida a sessão periodicamente (logout/desativação encerram a conexão).
	validateWS := func(ctx context.Context, r *http.Request) bool {
		c, err := r.Cookie(d.Config.SessionCookieName)
		if err != nil {
			return false
		}
		_, err = d.Auth.Authenticate(ctx, c.Value)
		return err == nil
	}

	// MCP (acesso por IA): só token Bearer, sem cookie; independe da sessão do app.
	mcpH.SetBaseURL(baseURL)
	r.Handle("/mcp", mcpH)
	oauthH.Routes(r) // /.well-known/oauth-*, /oauth/register|authorize|token|revoke

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(d.Auth.LoadSession)

		// Sem sessão.
		d.Auth.PublicRoutes(r)
		waH.PublicRoutes(r, d.Auth.CheckOrigin)

		// Sessão (MFA pode estar pendente).
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireSession, d.Auth.CSRF)
			d.Auth.SessionRoutes(r)
		})

		// Sessão completa: autorização por perfil/equipe dentro de cada módulo.
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireSession, d.Auth.RequireMFA, d.Auth.CSRF)
			d.Auth.ProtectedRoutes(r)
			usersH.SelfRoutes(r)
			teamsH.Routes(r)
			convsH.Routes(r)
			segmentsH.Routes(r)
			tagsH.Routes(r)
			waH.Routes(r)
			agendaH.Routes(r)
			aiH.Routes(r)
			autoH.Routes(r)
			chatH.Routes(r)
			r.Get("/ws", d.Hub.Handler(validateWS))

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				usersH.AdminRoutes(r)
				d.Auth.AdminRoutes(r)
				if d.Instance != nil {
					instance.NewHandler(d.Instance, d.Auth).Routes(r)
				}
			})
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpx.Error(w, http.StatusNotFound, "not_found", "Rota não encontrada.")
		})
	})

	if d.Config.WebDir != "" {
		web, err := webHandler(d.Config.WebDir)
		if err != nil {
			d.Logger.Error("frontend indisponível (WEB_DIR)", "dir", d.Config.WebDir, "err", err)
		} else {
			r.NotFound(web.ServeHTTP) // tudo fora de /api, /healthz e /readyz
		}
	}

	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// requestLogger registra método, rota, status e duração — nunca corpo, query ou cabeçalhos.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
