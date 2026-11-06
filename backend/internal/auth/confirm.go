package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/database"
)

// ConfirmWithCode reconfirma uma ação sensível com o código ATUAL do autenticador (cada código vale uma vez).
// Erros contam para o limite da sessão, que é encerrada ao esgotá-lo.
func (s *Service) ConfirmWithCode(ctx context.Context, sess *Session, code string, c Client) error {
	if !sess.MFAEnabled {
		return ErrMFANotEnabled
	}
	valid := false
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			enc      []byte
			lastStep int64
		)
		if err := tx.QueryRow(ctx, `SELECT mfa_secret_enc, mfa_last_step FROM users WHERE id = $1 FOR UPDATE`, sess.UserID).Scan(&enc, &lastStep); err != nil {
			return err
		}
		secret, err := s.box.Open(enc, mfaAAD(sess.UserID))
		if err != nil {
			return err
		}
		step, ok := validateTOTP(string(secret), code, time.Now(), lastStep)
		if !ok {
			return nil
		}
		valid = true
		_, err = tx.Exec(ctx, `UPDATE users SET mfa_last_step = $2 WHERE id = $1`, sess.UserID, step)
		return err
	})
	if err != nil {
		return err
	}
	if !valid {
		return s.mfaFailure(ctx, sess, c)
	}
	return nil
}
