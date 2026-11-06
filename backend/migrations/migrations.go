// Package migrations embute as migrações SQL versionadas e as executa com goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

func provider(dsn string) (*goose.Provider, *sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := renameLegacyVersions(db); err != nil {
		db.Close()
		return nil, nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fs.FS(files))
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return p, db, nil
}

// Up aplica todas as migrações pendentes, silenciosamente.
func Up(ctx context.Context, dsn string) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = p.Up(ctx)
	return err
}

// Run executa um comando (up, down, status) imprimindo o resultado.
func Run(ctx context.Context, dsn, command string) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	switch command {
	case "up":
		res, err := p.Up(ctx)
		for _, r := range res {
			fmt.Println(r)
		}
		if err == nil && len(res) == 0 {
			fmt.Println("nenhuma migração pendente")
		}
		return err
	case "down":
		r, err := p.Down(ctx)
		if r != nil {
			fmt.Println(r)
		}
		return err
	case "status":
		st, err := p.Status(ctx)
		for _, s := range st {
			applied := "pendente"
			if s.State == goose.StateApplied {
				applied = s.AppliedAt.Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-40s %s\n", s.Source.Path, applied)
		}
		return err
	default:
		return fmt.Errorf("comando de migração desconhecido: %q", command)
	}
}

// Create gera um novo arquivo SQL vazio em dir (uso em desenvolvimento).
func Create(dir, name string) error {
	goose.SetSequential(true) // 00008_nome.sql, mantendo a ordem numérica das migrações existentes
	return goose.Create(nil, dir, name, "sql")
}

// renameLegacyVersions: a migração da agenda foi criada com versão por data (20261105213059) e depois renumerada
// para 7. Bancos que já a aplicaram com o número antigo seriam recusados pelo goose ("migração faltando");
// aqui o registro é corrigido uma única vez, sem reexecutar o SQL. Não faz nada em bancos novos ou já ajustados.
func renameLegacyVersions(db *sql.DB) error {
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('goose_db_version') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return err
	}
	_, err := db.Exec(`UPDATE goose_db_version SET version_id = 7
		WHERE version_id = 20261105213059
		  AND NOT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 7)`)
	return err
}
