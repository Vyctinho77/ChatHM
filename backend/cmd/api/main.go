// Command api inicia o monólito modular do ChatHM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/app"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/httpapi"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("encerrando com erro", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Reserve the HTTP port before touching the shared WhatsApp session. A
	// duplicate process must fail before it can restore or replace that session.
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := migrations.Up(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
	}

	svcs, err := app.NewServices(ctx, cfg, pool, app.Options{Hasher: auth.NewHasher(), Logger: logger})
	if err != nil {
		return err
	}
	go svcs.Auth.RunJanitor(ctx, time.Hour)
	go svcs.Conversations.RunSnoozeWorker(ctx)
	go svcs.Automation.Run(ctx) // fila persistente: aprovação humana ou autonomia do assunto, no ritmo do escritório
	if err := svcs.WhatsApp.Start(ctx); err != nil {
		return err
	}
	defer svcs.WhatsApp.Shutdown()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Config: cfg, Logger: logger, DB: pool,
			Auth: svcs.Auth, Users: svcs.Users, Teams: svcs.Teams,
			Hub: svcs.Hub, Conversations: svcs.Conversations, Segments: svcs.Segments, Tags: svcs.Tags, AI: svcs.AI, Automation: svcs.Automation,
			WhatsApp: svcs.WhatsApp, Instance: svcs.Instance, Simulate: svcs.Simulate,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // não afeta o WebSocket (prazos removidos no Hijack)

		IdleTimeout: 120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api ouvindo", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("desligando servidor")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsDevelopment() {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
