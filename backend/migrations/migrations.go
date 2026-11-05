// Package migrations embute as migrações SQL versionadas e as executa com goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var fs embed.FS

func Up(ctx context.Context, dsn string) error { return Run(ctx, dsn, "up") }

func Run(ctx context.Context, dsn, command string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(fs)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	switch command {
	case "up":
		return goose.UpContext(ctx, db, ".")
	case "down":
		return goose.DownContext(ctx, db, ".")
	case "status":
		return goose.StatusContext(ctx, db, ".")
	default:
		return fmt.Errorf("comando de migração desconhecido: %q", command)
	}
}

// Create gera um novo arquivo SQL vazio em dir (uso em desenvolvimento).
func Create(dir, name string) error {
	goose.SetBaseFS(nil)
	return goose.Create(nil, dir, name, "sql")
}
