package database

import (
	"testing"
	"time"
)

const url = "postgres://u:p@localhost:5432/db?sslmode=disable"

func TestPoolStaysBelowTheSupabaseSessionPoolerLimit(t *testing.T) {
	// O pooler "Session" do Supabase aceita 15 conexões ao mesmo tempo; o padrão do app precisa ficar bem abaixo disso.
	cfg, err := poolConfig(url, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 10 || cfg.MinConns != 2 {
		t.Fatalf("defaults = %d/%d, want 10/2", cfg.MaxConns, cfg.MinConns)
	}
	if DefaultMaxConns >= 15 {
		t.Fatalf("DefaultMaxConns = %d must stay below the session pooler limit (15)", DefaultMaxConns)
	}
	if cfg.ConnConfig.ConnectTimeout != 10*time.Second {
		t.Fatalf("a connection that cannot open must fail fast, got %v", cfg.ConnConfig.ConnectTimeout)
	}
}

func TestPoolHonoursExplicitLimitsAndNeverKeepsMoreIdleThanItAllows(t *testing.T) {
	cfg, err := poolConfig(url, Options{MaxConns: 30, MinConns: 5})
	if err != nil || cfg.MaxConns != 30 || cfg.MinConns != 5 {
		t.Fatalf("explicit limits ignored: %v %v", cfg, err)
	}
	cfg, err = poolConfig(url, Options{MaxConns: 3, MinConns: 8})
	if err != nil || cfg.MaxConns != 3 || cfg.MinConns != 3 {
		t.Fatalf("MinConns must not exceed MaxConns: %d/%d %v", cfg.MaxConns, cfg.MinConns, err)
	}
}

func TestPoolRejectsAnInvalidURL(t *testing.T) {
	if _, err := poolConfig("::não é uma url::", Options{}); err == nil {
		t.Fatal("expected an error for an invalid DATABASE_URL")
	}
}
