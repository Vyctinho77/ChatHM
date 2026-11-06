// Package auth implementa login por e-mail/senha, sessões server-side em cookie
// HttpOnly, proteção CSRF, limitação de força bruta e MFA (TOTP).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleAgent Role = "agent" // atendente
)

func (r Role) Valid() bool { return r == RoleAdmin || r == RoleAgent }

type Config struct {
	CookieName      string
	CookieSecure    bool
	SessionTTL      time.Duration // validade absoluta
	IdleTimeout     time.Duration // expira sem atividade
	RequireAdminMFA bool
	AllowedOrigins  []string
	// AllowSelfRegister habilita POST /auth/register (contas nascem inativas, aguardando o administrador).
	AllowSelfRegister bool
}

const (
	failureWindow       = 15 * time.Minute
	maxFailuresPerEmail = 5
	maxFailuresPerIP    = 30
	maxMFAFailures      = 5
	lastSeenResolution  = time.Minute
	mfaIssuer           = "ChatHM"
)

var (
	ErrInvalidCredentials = errors.New("auth: credenciais inválidas")
	ErrTooManyAttempts    = errors.New("auth: muitas tentativas")
	ErrNoSession          = errors.New("auth: sessão inexistente ou expirada")
	ErrInvalidCode        = errors.New("auth: código MFA inválido")
	ErrMFANotEnabled      = errors.New("auth: MFA não habilitado")
	ErrMFAAlreadyEnabled  = errors.New("auth: MFA já habilitado")
	ErrMFASetupNotStarted = errors.New("auth: configuração de MFA não iniciada")
	ErrSamePassword       = errors.New("auth: nova senha igual à atual")
)

// Session é a sessão autenticada corrente, com os dados do usuário necessários
// para autorização. Papel e status são lidos do banco a cada requisição, então
// mudanças feitas pelo admin valem imediatamente.
type Session struct {
	ID          string
	UserID      string
	Email       string
	Name        string
	Role        Role
	Theme       string
	MFAEnabled  bool
	MFAVerified bool
	CSRFToken   string
	ExpiresAt   time.Time
}

func (s *Session) IsAdmin() bool { return s.Role == RoleAdmin }

// MFAPending indica que o usuário passou da senha mas ainda não do segundo fator.
func (s *Session) MFAPending() bool { return s.MFAEnabled && !s.MFAVerified }

// Client identifica a origem de uma requisição para auditoria e rate limit.
type Client struct {
	IP        string
	UserAgent string
}

type Service struct {
	db        *pgxpool.Pool
	cfg       Config
	hasher    Hasher
	box       *secretbox.Box
	dummyHash string
}

func NewService(db *pgxpool.Pool, cfg Config, hasher Hasher, box *secretbox.Box) (*Service, error) {
	// Hash fictício para que e-mails inexistentes levem o mesmo tempo que senhas erradas.
	dummy, err := hasher.Hash("dummy-password-for-timing-equalization")
	if err != nil {
		return nil, err
	}
	return &Service{db: db, cfg: cfg, hasher: hasher, box: box, dummyHash: dummy}, nil
}

func (s *Service) Hasher() Hasher { return s.hasher }

// MFASetupRequired: admin sem MFA quando a política exige.
func (s *Service) MFASetupRequired(sess *Session) bool {
	return s.cfg.RequireAdminMFA && sess.IsAdmin() && !sess.MFAEnabled
}

// ---------------------------------------------------------------------------
// Login / sessão

// Login valida as credenciais e cria uma nova sessão (novo token a cada login,
// evitando fixação de sessão).
func (s *Service) Login(ctx context.Context, email, password string, c Client) (string, *Session, error) {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 254 || len(password) > 4*MaxPasswordLen {
		return "", nil, ErrInvalidCredentials
	}

	throttled, err := s.throttled(ctx, email, c.IP)
	if err != nil {
		return "", nil, err
	}
	if throttled {
		s.audit(ctx, audit.Entry{Action: audit.LoginThrottled, IP: c.IP, Metadata: map[string]any{"email": email}})
		return "", nil, ErrTooManyAttempts
	}

	var (
		userID, hash string
		active, mfa  bool
	)
	err = s.db.QueryRow(ctx,
		`SELECT id, password_hash, is_active, mfa_enabled FROM users WHERE email = $1`, email,
	).Scan(&userID, &hash, &active, &mfa)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _, _ = s.hasher.Verify(password, s.dummyHash)
		s.recordFailure(ctx, email, "", c)
		return "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}

	ok, needsRehash, err := s.hasher.Verify(password, hash)
	if err != nil {
		return "", nil, err
	}
	if !ok || !active {
		s.recordFailure(ctx, email, userID, c)
		return "", nil, ErrInvalidCredentials
	}

	var token string
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO login_attempts (email, ip, success) VALUES ($1, NULLIF($2, '')::inet, true)`,
			email, c.IP); err != nil {
			return err
		}
		if needsRehash {
			if newHash, err := s.hasher.Hash(password); err == nil {
				if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, newHash); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID); err != nil {
			return err
		}
		token, err = s.createSession(ctx, tx, userID, !mfa, c)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: userID, Action: audit.LoginSuccess, TargetType: "user", TargetID: userID, IP: c.IP,
		})
	})
	if err != nil {
		return "", nil, err
	}

	sess, err := s.Authenticate(ctx, token)
	if err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

// throttled conta falhas recentes por e-mail (desde o último sucesso) e por IP.
func (s *Service) throttled(ctx context.Context, email, ip string) (bool, error) {
	var byEmail, byIP int
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM login_attempts
			  WHERE email = $1 AND NOT success
			    AND created_at > greatest(
			        now() - make_interval(secs => $3),
			        coalesce((SELECT max(created_at) FROM login_attempts WHERE email = $1 AND success), '-infinity'))),
			(SELECT count(*) FROM login_attempts
			  WHERE $2 <> '' AND ip = NULLIF($2, '')::inet AND NOT success
			    AND created_at > now() - make_interval(secs => $3))`,
		email, ip, failureWindow.Seconds(),
	).Scan(&byEmail, &byIP)
	if err != nil {
		return false, err
	}
	return byEmail >= maxFailuresPerEmail || byIP >= maxFailuresPerIP, nil
}

func (s *Service) recordFailure(ctx context.Context, email, userID string, c Client) {
	if _, err := s.db.Exec(ctx,
		`INSERT INTO login_attempts (email, ip, success) VALUES ($1, NULLIF($2, '')::inet, false)`,
		email, c.IP); err != nil {
		slog.ErrorContext(ctx, "registrando tentativa de login", "err", err)
	}
	s.audit(ctx, audit.Entry{
		Action: audit.LoginFailure, TargetType: "user", TargetID: userID, IP: c.IP,
		Metadata: map[string]any{"email": email},
	})
}

func (s *Service) createSession(ctx context.Context, db database.DBTX, userID string, mfaVerified bool, c Client) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", err
	}
	ua := c.UserAgent
	if len(ua) > 512 {
		ua = ua[:512]
	}
	_, err = db.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, csrf_token, mfa_verified, ip, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::inet, $6, now() + make_interval(secs => $7))`,
		userID, hashToken(token), csrf, mfaVerified, c.IP, ua, s.cfg.SessionTTL.Seconds())
	return token, err
}

// Authenticate resolve o token do cookie para uma sessão válida (não revogada,
// não expirada, dentro do tempo de inatividade e com usuário ativo).
func (s *Service) Authenticate(ctx context.Context, token string) (*Session, error) {
	if token == "" || len(token) > 128 {
		return nil, ErrNoSession
	}
	var (
		sess     Session
		role     string
		lastSeen time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.email, u.name, u.role, u.theme, u.mfa_enabled,
		       s.mfa_verified, s.csrf_token, s.expires_at, s.last_seen_at
		  FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1
		   AND s.revoked_at IS NULL
		   AND s.expires_at > now()
		   AND s.last_seen_at > now() - make_interval(secs => $2)
		   AND u.is_active`,
		hashToken(token), s.cfg.IdleTimeout.Seconds(),
	).Scan(&sess.ID, &sess.UserID, &sess.Email, &sess.Name, &role, &sess.Theme, &sess.MFAEnabled,
		&sess.MFAVerified, &sess.CSRFToken, &sess.ExpiresAt, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	sess.Role = Role(role)

	if time.Since(lastSeen) > lastSeenResolution {
		if _, err := s.db.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, sess.ID); err != nil {
			return nil, err
		}
	}
	return &sess, nil
}

func (s *Service) Logout(ctx context.Context, sess *Session, c Client) error {
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sess.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: sess.UserID, Action: audit.Logout, IP: c.IP})
	})
}

// RevokeUserSessions revoga as sessões ativas do usuário, exceto exceptID (pode ser "").
func RevokeUserSessions(ctx context.Context, db database.DBTX, userID, exceptID string) error {
	_, err := db.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		 WHERE user_id = $1 AND revoked_at IS NULL AND ($2 = '' OR id <> NULLIF($2, '')::uuid)`,
		userID, exceptID)
	return err
}

// ChangePassword troca a senha do próprio usuário e encerra suas outras sessões.
func (s *Service) ChangePassword(ctx context.Context, sess *Session, current, next string, c Client) error {
	throttled, err := s.throttled(ctx, sess.Email, c.IP)
	if err != nil {
		return err
	}
	if throttled {
		return ErrTooManyAttempts
	}

	var hash string
	if err := s.db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, sess.UserID).Scan(&hash); err != nil {
		return err
	}
	ok, _, err := s.hasher.Verify(current, hash)
	if err != nil {
		return err
	}
	if !ok {
		s.recordFailure(ctx, sess.Email, sess.UserID, c)
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(next); err != nil {
		return err
	}
	if current == next {
		return ErrSamePassword
	}
	newHash, err := s.hasher.Hash(next)
	if err != nil {
		return err
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE users SET password_hash = $2, password_changed_at = now() WHERE id = $1`,
			sess.UserID, newHash); err != nil {
			return err
		}
		if err := RevokeUserSessions(ctx, tx, sess.UserID, sess.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.PasswordChange, TargetType: "user", TargetID: sess.UserID, IP: c.IP,
		})
	})
}

// ---------------------------------------------------------------------------
// MFA (TOTP)

func mfaAAD(userID string) []byte { return []byte("user-mfa:" + userID) }

// SetupMFA gera um novo segredo pendente; só passa a valer após EnableMFA.
func (s *Service) SetupMFA(ctx context.Context, sess *Session) (secret, url string, err error) {
	if sess.MFAEnabled {
		return "", "", ErrMFAAlreadyEnabled
	}
	secret, err = newTOTPSecret()
	if err != nil {
		return "", "", err
	}
	enc, err := s.box.Seal([]byte(secret), mfaAAD(sess.UserID))
	if err != nil {
		return "", "", err
	}
	tag, err := s.db.Exec(ctx,
		`UPDATE users SET mfa_pending_secret_enc = $2 WHERE id = $1 AND NOT mfa_enabled`, sess.UserID, enc)
	if err != nil {
		return "", "", err
	}
	if tag.RowsAffected() == 0 {
		return "", "", ErrMFAAlreadyEnabled
	}
	return secret, otpauthURL(mfaIssuer, sess.Email, secret), nil
}

// EnableMFA confirma o segredo pendente com um código válido.
func (s *Service) EnableMFA(ctx context.Context, sess *Session, code string, c Client) error {
	if sess.MFAEnabled {
		return ErrMFAAlreadyEnabled
	}
	valid := false
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			pending  []byte
			lastStep int64
		)
		if err := tx.QueryRow(ctx,
			`SELECT mfa_pending_secret_enc, mfa_last_step FROM users WHERE id = $1 FOR UPDATE`, sess.UserID,
		).Scan(&pending, &lastStep); err != nil {
			return err
		}
		if pending == nil {
			return ErrMFASetupNotStarted
		}
		secret, err := s.box.Open(pending, mfaAAD(sess.UserID))
		if err != nil {
			return err
		}
		step, ok := validateTOTP(string(secret), code, time.Now(), lastStep)
		if !ok {
			return nil
		}
		valid = true
		if _, err := tx.Exec(ctx, `
			UPDATE users SET mfa_enabled = true, mfa_secret_enc = mfa_pending_secret_enc,
			       mfa_pending_secret_enc = NULL, mfa_last_step = $2
			 WHERE id = $1`, sess.UserID, step); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET mfa_verified = true WHERE id = $1`, sess.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.MFAEnable, TargetType: "user", TargetID: sess.UserID, IP: c.IP,
		})
	})
	if err != nil {
		return err
	}
	if !valid {
		return s.mfaFailure(ctx, sess, c)
	}
	sess.MFAEnabled, sess.MFAVerified = true, true
	return nil
}

// VerifyMFA conclui o login com o segundo fator. O token de sessão é rotacionado
// (elevação de privilégio) e o novo token é devolvido para o cookie.
func (s *Service) VerifyMFA(ctx context.Context, sess *Session, code string, c Client) (string, error) {
	if !sess.MFAEnabled {
		return "", ErrMFANotEnabled
	}
	var newToken string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			enc      []byte
			lastStep int64
		)
		if err := tx.QueryRow(ctx,
			`SELECT mfa_secret_enc, mfa_last_step FROM users WHERE id = $1 FOR UPDATE`, sess.UserID,
		).Scan(&enc, &lastStep); err != nil {
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
		if _, err := tx.Exec(ctx, `UPDATE users SET mfa_last_step = $2 WHERE id = $1`, sess.UserID, step); err != nil {
			return err
		}
		token, err := randomToken()
		if err != nil {
			return err
		}
		csrf, err := randomToken()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE sessions SET token_hash = $2, csrf_token = $3, mfa_verified = true, mfa_failures = 0
			 WHERE id = $1`, sess.ID, hashToken(token), csrf); err != nil {
			return err
		}
		newToken = token
		return nil
	})
	if err != nil {
		return "", err
	}
	if newToken == "" {
		return "", s.mfaFailure(ctx, sess, c)
	}
	return newToken, nil
}

// mfaFailure conta falhas por sessão; ao atingir o limite, a sessão é revogada.
func (s *Service) mfaFailure(ctx context.Context, sess *Session, c Client) error {
	var failures int
	if err := s.db.QueryRow(ctx,
		`UPDATE sessions SET mfa_failures = mfa_failures + 1 WHERE id = $1 RETURNING mfa_failures`, sess.ID,
	).Scan(&failures); err != nil {
		return err
	}
	s.audit(ctx, audit.Entry{ActorID: sess.UserID, Action: audit.MFAVerifyFailure, IP: c.IP})
	if failures >= maxMFAFailures {
		if _, err := s.db.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1`, sess.ID); err != nil {
			return err
		}
		return ErrTooManyAttempts
	}
	return ErrInvalidCode
}

// ResetMFA desabilita o MFA de um usuário (recuperação via CLI de administração).
func ResetMFA(ctx context.Context, db *pgxpool.Pool, userID string) error {
	return database.InTx(ctx, db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE users SET mfa_enabled = false, mfa_secret_enc = NULL,
			       mfa_pending_secret_enc = NULL, mfa_last_step = 0
			 WHERE id = $1`, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		if err := RevokeUserSessions(ctx, tx, userID, ""); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: audit.MFAReset, TargetType: "user", TargetID: userID})
	})
}

// ---------------------------------------------------------------------------

// RunJanitor remove periodicamente sessões vencidas e tentativas antigas.
func (s *Service) RunJanitor(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := s.cleanup(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "limpeza de sessões", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) cleanup(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `
		DELETE FROM sessions
		 WHERE expires_at < now() - interval '7 days'
		    OR revoked_at < now() - interval '7 days'
		    OR last_seen_at < now() - make_interval(secs => $1) - interval '7 days'`,
		s.cfg.IdleTimeout.Seconds()); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM login_attempts WHERE created_at < now() - interval '1 day'`)
	return err
}

func (s *Service) audit(ctx context.Context, e audit.Entry) {
	if err := audit.Record(ctx, s.db, e); err != nil {
		slog.ErrorContext(ctx, "gravando auditoria", "action", e.Action, "err", err)
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerando token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
