package auth

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/platform/database"
)

// IssueInstallCode gera o código de instalação de uso único (invalida os anteriores). O texto só existe aqui.
func IssueInstallCode(ctx context.Context, db database.DBTX) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", err
	}
	if _, err := db.Exec(ctx, `DELETE FROM install_codes`); err != nil {
		return "", err
	}
	if _, err := db.Exec(ctx, `INSERT INTO install_codes (code_hash) VALUES ($1)`, codeHash(code)); err != nil {
		return "", err
	}
	return code, nil
}

// installCodePending: há um código de entrega ainda não usado.
func (s *Service) installCodePending(ctx context.Context) (bool, error) {
	var pending bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM install_codes WHERE used_at IS NULL)`).Scan(&pending)
	return pending, err
}

// installCodeRequired: a tela de instalação precisa pedir um código (de entrega ou o SETUP_TOKEN do servidor).
func (s *Service) installCodeRequired(ctx context.Context) (bool, error) {
	if s.cfg.SetupToken != "" {
		return true, nil
	}
	return s.installCodePending(ctx)
}

// checkInstallCode confere o código de instalação. Com um código de entrega pendente, só ele vale; sem ele,
// vale o SETUP_TOKEN (se definido); sem nenhum dos dois a instalação é livre. Erros contam para o limite por IP.
func (s *Service) checkInstallCode(ctx context.Context, code, ip string) error {
	pending, err := s.installCodePending(ctx)
	if err != nil {
		return err
	}
	if !pending && s.cfg.SetupToken == "" {
		return nil
	}
	throttled, err := s.throttled(ctx, "install-code", ip)
	if err != nil {
		return err
	}
	if throttled {
		return ErrTooManyAttempts
	}
	code = strings.TrimSpace(code)
	ok := false
	if len(code) <= 64 {
		if pending {
			var stored []byte
			err := s.db.QueryRow(ctx, `SELECT code_hash FROM install_codes WHERE used_at IS NULL ORDER BY created_at DESC LIMIT 1`).Scan(&stored)
			ok = err == nil && subtle.ConstantTimeCompare(stored, codeHash(code)) == 1
		} else {
			ok = subtle.ConstantTimeCompare([]byte(code), []byte(s.cfg.SetupToken)) == 1
		}
	}
	if !ok {
		s.recordFailure(ctx, "install-code", "", Client{IP: ip})
		return ErrSetupForbidden
	}
	return nil
}
