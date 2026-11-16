package config

import (
	"testing"
	"time"
)

func TestPersistentSessionDefaults(t *testing.T) {
	t.Setenv("SESSION_TTL", "")
	t.Setenv("SESSION_IDLE_TIMEOUT", "")
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://localhost/chathm_test")
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("WHATSAPP_PROVIDER", "fake")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionTTL != 30*24*time.Hour || cfg.SessionIdle != 30*24*time.Hour {
		t.Fatalf("session defaults = %v / %v, want 30 days", cfg.SessionTTL, cfg.SessionIdle)
	}
}

func TestDatabasePoolSettings(t *testing.T) {
	base := func() {
		t.Setenv("APP_ENV", "development")
		t.Setenv("DATABASE_URL", "postgres://localhost/chathm_test")
		t.Setenv("ENCRYPTION_KEY", "test-key")
		t.Setenv("WHATSAPP_PROVIDER", "fake")
	}
	// Sem as variáveis, fica 0 e o pacote database aplica o padrão seguro (10 e 2): nada a configurar no servidor.
	base()
	t.Setenv("DB_MAX_CONNS", "")
	t.Setenv("DB_MIN_CONNS", "")
	cfg, err := Load()
	if err != nil || cfg.DBMaxConns != 0 || cfg.DBMinConns != 0 {
		t.Fatalf("defaults = %d/%d %v, want 0/0", cfg.DBMaxConns, cfg.DBMinConns, err)
	}
	t.Setenv("DB_MAX_CONNS", "12")
	t.Setenv("DB_MIN_CONNS", "3")
	if cfg, err = Load(); err != nil || cfg.DBMaxConns != 12 || cfg.DBMinConns != 3 {
		t.Fatalf("explicit values = %d/%d %v", cfg.DBMaxConns, cfg.DBMinConns, err)
	}
	for _, bad := range []string{"0", "-1", "abc", "201"} {
		t.Setenv("DB_MAX_CONNS", bad)
		if _, err := Load(); err == nil {
			t.Fatalf("DB_MAX_CONNS=%q should be rejected", bad)
		}
	}
}
