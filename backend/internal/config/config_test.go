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
