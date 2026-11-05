// Command migrate aplica ou reverte migrações versionadas do banco.
//
// Uso: go run ./cmd/migrate [up|down|status|create <nome>]
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vyctinho77/chathm/backend/internal/config"
	"github.com/vyctinho77/chathm/backend/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: migrate [up|down|status|create <nome>]")
		os.Exit(2)
	}

	cmd := os.Args[1]
	if cmd == "create" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "uso: migrate create <nome>")
			os.Exit(2)
		}
		if err := migrations.Create("migrations", os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := migrations.Run(context.Background(), cfg.DatabaseURL, cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
