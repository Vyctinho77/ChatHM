package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// InviteInput: quem convidar. A pessoa define o próprio acesso (autenticador) com o código do convite;
// ninguém precisa conhecer ou repassar uma senha.
type InviteInput struct {
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Role    auth.Role `json:"role"`
	TeamIDs []string  `json:"teamIds"`
}

// InviteResult traz o código, mostrado uma única vez ao administrador.
type InviteResult struct {
	User       User      `json:"user"`
	InviteCode string    `json:"inviteCode"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// Invite cria a conta (inativa, sem senha utilizável) e emite o convite.
func (s *Service) Invite(ctx context.Context, actorID, ip string, in InviteInput) (InviteResult, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return InviteResult{}, err
	}
	u, err := s.Create(ctx, actorID, ip, CreateInput{
		Email: in.Email, Name: in.Name, Role: in.Role, TeamIDs: in.TeamIDs,
		Password: "Aa1!" + base64.RawURLEncoding.EncodeToString(b), // nunca revelada
	})
	if err != nil {
		return InviteResult{}, err
	}
	var code string
	var exp time.Time
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, u.ID); err != nil {
			return err
		}
		if code, exp, err = auth.IssueInvite(ctx, tx, u.ID, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "user.invite", TargetType: "user", TargetID: u.ID, IP: ip})
	})
	if err != nil {
		return InviteResult{}, err
	}
	u, err = s.Get(ctx, u.ID)
	return InviteResult{User: u, InviteCode: code, ExpiresAt: exp}, err
}

// Reinvite emite novo código para uma pessoa existente e REDEFINE o autenticador dela
// (celular perdido ou trocado): o segredo antigo, os códigos de recuperação e as sessões são encerrados.
func (s *Service) Reinvite(ctx context.Context, actorID, ip, id string) (InviteResult, error) {
	if !httpx.ValidUUID(id) {
		return InviteResult{}, ErrNotFound
	}
	var code string
	var exp time.Time
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users SET mfa_enabled = false, mfa_secret_enc = NULL, mfa_pending_secret_enc = NULL, mfa_last_step = 0
			 WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM recovery_codes WHERE user_id = $1`, id); err != nil {
			return err
		}
		if err := auth.RevokeUserSessions(ctx, tx, id, ""); err != nil {
			return err
		}
		// Autenticador redefinido: tokens de acesso por IA da pessoa também deixam de valer.
		if _, err := tx.Exec(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE oauth_grants SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
			return err
		}
		if code, exp, err = auth.IssueInvite(ctx, tx, id, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "user.reinvite", TargetType: "user", TargetID: id, IP: ip})
	})
	if err != nil {
		return InviteResult{}, err
	}
	u, err := s.Get(ctx, id)
	return InviteResult{User: u, InviteCode: code, ExpiresAt: exp}, err
}
