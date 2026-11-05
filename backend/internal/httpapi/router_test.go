package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/config"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestHealthAndReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{AllowedOrigins: []string{"http://localhost:5173"}}

	cases := []struct {
		name   string
		path   string
		dbErr  error
		status int
	}{
		{"healthz", "/healthz", nil, http.StatusOK},
		{"readyz ok", "/readyz", nil, http.StatusOK},
		{"readyz db fora", "/readyz", errors.New("down"), http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(cfg, logger, fakeDB{err: tc.dbErr})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, esperado %d", rec.Code, tc.status)
			}
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("cabeçalho de segurança ausente")
			}
		})
	}
}
