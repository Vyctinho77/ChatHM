// Package config carrega a configuração da aplicação a partir de variáveis de ambiente.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env         string
	HTTPAddr    string
	LogLevel    slog.Level
	DatabaseURL string
	AutoMigrate bool

	// Origens do frontend autorizadas (CORS com credenciais).
	AllowedOrigins []string

	// Sessão
	SessionCookieName string
	SessionTTL        time.Duration // validade absoluta da sessão
	SessionIdle       time.Duration // expira após inatividade
	CookieSecure      bool
	RequireAdminMFA   bool

	// Confiar em X-Forwarded-For/X-Real-IP (somente atrás de proxy reverso controlado).
	TrustProxyHeaders bool

	// Chave AES-256 (base64, 32 bytes) para segredos em repouso (MFA, sessão do WhatsApp).
	// Guardada fora do banco.
	EncryptionKey string

	// Provedor do WhatsApp: "whatsmeow" (QR Code) ou "fake" (simulado, testes/dev).
	WhatsAppProvider string

	// Armazenamento S3 compatível (anexos).
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
}

func (c Config) IsDevelopment() bool { return c.Env == "development" }

func Load() (Config, error) {
	cfg := Config{
		Env:               getenv("APP_ENV", "development"),
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		AutoMigrate:       getbool("AUTO_MIGRATE", false),
		AllowedOrigins:    splitList(getenv("ALLOWED_ORIGINS", "http://localhost:5173")),
		SessionCookieName: getenv("SESSION_COOKIE_NAME", "chathm_session"),
		CookieSecure:      getbool("COOKIE_SECURE", true),
		RequireAdminMFA:   getbool("REQUIRE_ADMIN_MFA", true),
		TrustProxyHeaders: getbool("TRUST_PROXY_HEADERS", false),
		EncryptionKey:     os.Getenv("ENCRYPTION_KEY"),
		WhatsAppProvider:  getenv("WHATSAPP_PROVIDER", "whatsmeow"),
		S3Endpoint:        os.Getenv("S3_ENDPOINT"),
		S3Region:          getenv("S3_REGION", "us-east-1"),
		S3Bucket:          os.Getenv("S3_BUCKET"),
		S3AccessKey:       os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:       os.Getenv("S3_SECRET_KEY"),
		S3UseSSL:          getbool("S3_USE_SSL", true),
	}

	var errs []error

	ttl, err := time.ParseDuration(getenv("SESSION_TTL", "12h"))
	if err != nil {
		errs = append(errs, fmt.Errorf("SESSION_TTL inválido: %w", err))
	}
	cfg.SessionTTL = ttl

	idle, err := time.ParseDuration(getenv("SESSION_IDLE_TIMEOUT", "2h"))
	if err != nil {
		errs = append(errs, fmt.Errorf("SESSION_IDLE_TIMEOUT inválido: %w", err))
	}
	cfg.SessionIdle = idle

	if err := cfg.LogLevel.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL inválido: %w", err))
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL é obrigatório"))
	}

	if cfg.WhatsAppProvider != "whatsmeow" && cfg.WhatsAppProvider != "fake" {
		errs = append(errs, errors.New("WHATSAPP_PROVIDER deve ser whatsmeow ou fake"))
	}
	if cfg.EncryptionKey == "" {
		errs = append(errs, errors.New("ENCRYPTION_KEY é obrigatório (gere com: openssl rand -base64 32)"))
	}

	if !cfg.IsDevelopment() {
		if !cfg.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE deve ser true fora de development"))
		}
		if cfg.WhatsAppProvider == "fake" {
			errs = append(errs, errors.New("WHATSAPP_PROVIDER=fake só é permitido em development"))
		}
		if !cfg.RequireAdminMFA {
			errs = append(errs, errors.New("REQUIRE_ADMIN_MFA deve ser true fora de development"))
		}
	}

	return cfg, errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getbool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
