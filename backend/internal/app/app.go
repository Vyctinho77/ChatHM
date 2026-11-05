// Package app monta os serviços a partir da configuração (usado pela API e pela CLI).
package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
	"github.com/vyctinho77/chathm/backend/internal/teams"
	"github.com/vyctinho77/chathm/backend/internal/users"
)

type Services struct {
	Auth  *auth.Service
	Users *users.Service
	Teams *teams.Service
}

func NewServices(cfg config.Config, db *pgxpool.Pool, hasher auth.Hasher) (*Services, error) {
	box, err := secretbox.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	authSvc, err := auth.NewService(db, auth.Config{
		CookieName:      cfg.SessionCookieName,
		CookieSecure:    cfg.CookieSecure,
		SessionTTL:      cfg.SessionTTL,
		IdleTimeout:     cfg.SessionIdle,
		RequireAdminMFA: cfg.RequireAdminMFA,
		AllowedOrigins:  cfg.AllowedOrigins,
	}, hasher, box)
	if err != nil {
		return nil, err
	}
	return &Services{
		Auth:  authSvc,
		Users: users.NewService(db, hasher),
		Teams: teams.NewService(db),
	}, nil
}
