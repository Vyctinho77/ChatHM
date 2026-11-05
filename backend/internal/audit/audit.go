// Package audit registra a trilha de auditoria (audit_logs).
//
// Regra: metadata nunca deve conter senhas, tokens, segredos ou conteúdo de mensagens.
package audit

import (
	"context"

	"github.com/vyctinho77/chathm/backend/internal/platform/database"
)

type Entry struct {
	ActorID    string // vazio = sistema/anônimo
	Action     string
	TargetType string
	TargetID   string
	IP         string
	Metadata   map[string]any
}

// Ações registradas.
const (
	LoginSuccess       = "auth.login.success"
	LoginFailure       = "auth.login.failure"
	LoginThrottled     = "auth.login.throttled"
	Logout             = "auth.logout"
	PasswordChange     = "auth.password.change"
	MFAEnable          = "auth.mfa.enable"
	MFAVerifyFailure   = "auth.mfa.verify.failure"
	MFAReset           = "auth.mfa.reset"
	UserCreate         = "user.create"
	UserUpdate         = "user.update"
	UserPasswordReset  = "user.password.reset"
	TeamCreate         = "team.create"
	TeamUpdate         = "team.update"
	TeamDelete         = "team.delete"
	TeamMemberAdd      = "team.member.add"
	TeamMemberRemove   = "team.member.remove"
	WhatsAppConnect    = "whatsapp.connect"
	WhatsAppDisconnect = "whatsapp.disconnect"
)

func Record(ctx context.Context, db database.DBTX, e Entry) error {
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	_, err := db.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, ip, metadata)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, NULLIF($5, '')::inet, $6)`,
		e.ActorID, e.Action, e.TargetType, e.TargetID, e.IP, e.Metadata)
	return err
}
