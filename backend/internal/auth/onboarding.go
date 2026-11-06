package auth

// Acesso sem terminal e com autenticador:
//
//   - configuração inicial: com o banco sem usuários, a primeira pessoa cria o administrador e liga o autenticador;
//   - convite: o administrador gera um código de uso único; a pessoa o digita, configura o autenticador e entra;
//   - login: e-mail + código de 6 dígitos do app (ou um código de recuperação);
//   - recuperação: 8 códigos de uso único entregues uma vez; o administrador também pode emitir novo convite.
//
// O segredo do autenticador só é gravado quando a pessoa prova que o configurou (código válido).
// Enquanto isso ele viaja num token lacrado (AES-GCM) e nunca fica no banco.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var (
	ErrInvalidInvite  = errors.New("auth: convite inválido ou expirado")
	ErrSetupDone      = errors.New("auth: configuração inicial já concluída")
	ErrSetupForbidden = errors.New("auth: código de instalação inválido")
)

const (
	inviteTTL         = 72 * time.Hour
	maxInviteFailures = 10
	enrollTokenTTL    = 15 * time.Minute
	recoveryCodeCount = 8
	codeAlphabet      = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // sem 0/O/1/I/L
	enrollAADPrefix   = "enroll:"
)

// newCode gera "XXXXX-XXXXX" (~50 bits).
func newCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 0, 11)
	for i, v := range b {
		if i == 5 {
			out = append(out, '-')
		}
		out = append(out, codeAlphabet[int(v)%len(codeAlphabet)])
	}
	return string(out), nil
}

// normalizeCode: maiúsculas e só letras/números ("abcde 12345" == "ABCDE-12345").
func normalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func codeHash(code string) []byte { return hashToken(normalizeCode(code)) }

func isTOTPShape(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != totpDigits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Token lacrado do segredo pendente

type enrollClaims struct {
	Kind     string `json:"k"` // setup | enroll
	UserID   string `json:"u,omitempty"`
	InviteID string `json:"i,omitempty"`
	Name     string `json:"n,omitempty"`
	Email    string `json:"e,omitempty"`
	Secret   string `json:"s"`
	Exp      int64  `json:"x"`
}

func (s *Service) sealClaims(c enrollClaims) (string, error) {
	raw, _ := json.Marshal(c)
	enc, err := s.box.Seal(raw, []byte(enrollAADPrefix+c.Kind))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(enc), nil
}

func (s *Service) openClaims(token, kind string) (enrollClaims, bool) {
	var c enrollClaims
	enc, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return c, false
	}
	raw, err := s.box.Open(enc, []byte(enrollAADPrefix+kind))
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return c, false
	}
	return c, c.Kind == kind && time.Now().Unix() < c.Exp
}

// Enrollment é o que a tela mostra para configurar o autenticador.
type Enrollment struct {
	Secret     string `json:"secret"`
	OtpauthURL string `json:"otpauthUrl"`
	Token      string `json:"token"`
	Name       string `json:"name,omitempty"`
}

func (s *Service) newEnrollment(c enrollClaims, account string) (Enrollment, error) {
	secret, err := newTOTPSecret()
	if err != nil {
		return Enrollment{}, err
	}
	c.Secret, c.Exp = secret, time.Now().Add(enrollTokenTTL).Unix()
	tok, err := s.sealClaims(c)
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{Secret: secret, OtpauthURL: otpauthURL(mfaIssuer, account, secret), Token: tok, Name: c.Name}, nil
}

// ---------------------------------------------------------------------------
// Códigos de recuperação

func insertRecoveryCodes(ctx context.Context, db database.DBTX, userID string) ([]string, error) {
	if _, err := db.Exec(ctx, `DELETE FROM recovery_codes WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	codes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		c, err := newCode()
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(ctx, `INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2)`, userID, codeHash(c)); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

// RegenerateRecoveryCodes substitui os códigos de recuperação da pessoa logada.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, sess *Session, c Client) ([]string, error) {
	if !sess.MFAEnabled {
		return nil, ErrMFANotEnabled
	}
	var codes []string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) (err error) {
		if codes, err = insertRecoveryCodes(ctx, tx, sess.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: sess.UserID, Action: "auth.recovery.regenerate", TargetType: "user", TargetID: sess.UserID, IP: c.IP})
	})
	return codes, err
}

// ---------------------------------------------------------------------------
// Configuração inicial (primeiro administrador)

func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	// A instalação inicial está aberta enquanto o escritório não tem administrador.
	var any bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin')`).Scan(&any)
	return !any, err
}

func validateIdentity(name, email string) (string, string, error) {
	name, email = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email))
	f := httpx.FieldErrors{}
	if n := utf8.RuneCountInString(name); n < 1 || n > 120 {
		f["name"] = "Informe um nome de até 120 caracteres."
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		f["email"] = "E-mail inválido."
	}
	return name, email, f.Err()
}

// SetupStart valida os dados do primeiro administrador e devolve o segredo do autenticador.
func (s *Service) SetupStart(ctx context.Context, name, email, installCode string, c Client) (Enrollment, error) {
	needs, err := s.NeedsSetup(ctx)
	if err != nil {
		return Enrollment{}, err
	}
	if !needs {
		return Enrollment{}, ErrSetupDone
	}
	if err := s.checkInstallCode(ctx, installCode, c.IP); err != nil {
		return Enrollment{}, err
	}
	if s.waConnected != nil && !s.waConnected() {
		return Enrollment{}, ErrSetupNeedsWhatsApp
	}
	name, email, err = validateIdentity(name, email)
	if err != nil {
		return Enrollment{}, err
	}
	return s.newEnrollment(enrollClaims{Kind: "setup", Name: name, Email: email}, email)
}

// SetupFinish cria o administrador (com autenticador ativo) e a sessão. Só funciona enquanto não há usuários.
func (s *Service) SetupFinish(ctx context.Context, token, code string, c Client) (string, *Session, []string, error) {
	cl, ok := s.openClaims(token, "setup")
	if !ok {
		return "", nil, nil, ErrInvalidInvite
	}
	step, ok := validateTOTP(cl.Secret, code, time.Now(), 0)
	if !ok {
		return "", nil, nil, ErrInvalidCode
	}
	hash, err := s.hasher.Hash(mustToken())
	if err != nil {
		return "", nil, nil, err
	}
	var (
		token2 string
		codes  []string
	)
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('setup', 0))`); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin')`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrSetupDone
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, name, password_hash, role, last_login_at) VALUES ($1, $2, $3, 'admin', now()) RETURNING id`,
			cl.Email, cl.Name, hash).Scan(&id); err != nil {
			return err
		}
		sealed, err := s.box.Seal([]byte(cl.Secret), mfaAAD(id))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET mfa_enabled = true, mfa_secret_enc = $2, mfa_last_step = $3 WHERE id = $1`, id, sealed, step); err != nil {
			return err
		}
		if codes, err = insertRecoveryCodes(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE install_codes SET used_at = now() WHERE used_at IS NULL`); err != nil { // código de entrega: uso único
			return err
		}
		if _, err := s.storeNewOfficeToken(ctx, tx, false); err != nil { // token do escritório nasce com o administrador
			return err
		}
		if token2, err = s.createSession(ctx, tx, id, true, c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: id, Action: "setup.complete", TargetType: "user", TargetID: id, IP: c.IP})
	})
	if err != nil {
		return "", nil, nil, err
	}
	sess, err := s.Authenticate(ctx, token2)
	return token2, sess, codes, err
}

func mustToken() string {
	t, err := randomToken()
	if err != nil {
		panic(err)
	}
	return t
}

// ---------------------------------------------------------------------------
// Convites

// IssueInvite emite (e invalida o anterior) o convite da pessoa. O código só é devolvido aqui.
func IssueInvite(ctx context.Context, db database.DBTX, userID, createdBy string) (string, time.Time, error) {
	code, err := newCode()
	if err != nil {
		return "", time.Time{}, err
	}
	if _, err := db.Exec(ctx, `DELETE FROM invites WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(inviteTTL)
	var by any
	if createdBy != "" {
		by = createdBy
	}
	if _, err := db.Exec(ctx, `INSERT INTO invites (user_id, code_hash, expires_at, created_by) VALUES ($1, $2, $3, $4)`,
		userID, codeHash(code), exp, by); err != nil {
		return "", time.Time{}, err
	}
	return code, exp, nil
}

// RandomPasswordHash: contas criadas por convite não têm senha utilizável.
func (s *Service) RandomPasswordHash() (string, error) { return s.hasher.Hash(mustToken()) }

type inviteRow struct {
	userID, inviteID, name string
}

// checkInvite confere e-mail + código. Falhas contam por convite e por IP/e-mail (mesma proteção do login).
func (s *Service) checkInvite(ctx context.Context, email, inviteCode string, c Client) (inviteRow, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 254 || len(inviteCode) > 64 {
		return inviteRow{}, ErrInvalidInvite
	}
	throttled, err := s.throttled(ctx, email, c.IP)
	if err != nil {
		return inviteRow{}, err
	}
	if throttled {
		return inviteRow{}, ErrTooManyAttempts
	}
	var (
		r        inviteRow
		hash     []byte
		expires  time.Time
		failures int
	)
	err = s.db.QueryRow(ctx, `
		SELECT u.id, i.id, u.name, i.code_hash, i.expires_at, i.failures
		  FROM users u JOIN invites i ON i.user_id = u.id AND i.used_at IS NULL
		 WHERE u.email = $1`, email).Scan(&r.userID, &r.inviteID, &r.name, &hash, &expires, &failures)
	if errors.Is(err, pgx.ErrNoRows) {
		s.recordFailure(ctx, email, "", c)
		return inviteRow{}, ErrInvalidInvite
	}
	if err != nil {
		return inviteRow{}, err
	}
	if failures >= maxInviteFailures || time.Now().After(expires) {
		s.recordFailure(ctx, email, r.userID, c)
		return inviteRow{}, ErrInvalidInvite
	}
	if subtle.ConstantTimeCompare(hash, codeHash(inviteCode)) != 1 {
		_, _ = s.db.Exec(ctx, `UPDATE invites SET failures = failures + 1 WHERE id = $1`, r.inviteID)
		s.recordFailure(ctx, email, r.userID, c)
		return inviteRow{}, ErrInvalidInvite
	}
	return r, nil
}

// EnrollStart valida o convite e devolve o segredo do autenticador a configurar.
func (s *Service) EnrollStart(ctx context.Context, email, inviteCode string, c Client) (Enrollment, error) {
	r, err := s.checkInvite(ctx, email, inviteCode, c)
	if err != nil {
		return Enrollment{}, err
	}
	return s.newEnrollment(enrollClaims{Kind: "enroll", UserID: r.userID, InviteID: r.inviteID, Name: r.name},
		strings.ToLower(strings.TrimSpace(email)))
}

// EnrollFinish confirma o autenticador com um código válido: ativa a conta, consome o convite,
// entrega os códigos de recuperação e abre a sessão.
func (s *Service) EnrollFinish(ctx context.Context, email, inviteCode, token, code string, c Client) (string, *Session, []string, error) {
	r, err := s.checkInvite(ctx, email, inviteCode, c)
	if err != nil {
		return "", nil, nil, err
	}
	cl, ok := s.openClaims(token, "enroll")
	if !ok || cl.UserID != r.userID || cl.InviteID != r.inviteID {
		return "", nil, nil, ErrInvalidInvite
	}
	step, ok := validateTOTP(cl.Secret, code, time.Now(), 0)
	if !ok {
		_, _ = s.db.Exec(ctx, `UPDATE invites SET failures = failures + 1 WHERE id = $1`, r.inviteID)
		return "", nil, nil, ErrInvalidCode
	}
	var (
		tok   string
		codes []string
	)
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE invites SET used_at = now() WHERE id = $1 AND used_at IS NULL`, r.inviteID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrInvalidInvite // usado em paralelo
		}
		sealed, err := s.box.Seal([]byte(cl.Secret), mfaAAD(r.userID))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET mfa_enabled = true, mfa_secret_enc = $2, mfa_pending_secret_enc = NULL,
			       mfa_last_step = $3, is_active = true, last_login_at = now()
			 WHERE id = $1`, r.userID, sealed, step); err != nil {
			return err
		}
		if codes, err = insertRecoveryCodes(ctx, tx, r.userID); err != nil {
			return err
		}
		if tok, err = s.createSession(ctx, tx, r.userID, true, c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: r.userID, Action: "auth.enroll.complete", TargetType: "user", TargetID: r.userID, IP: c.IP})
	})
	if err != nil {
		return "", nil, nil, err
	}
	sess, err := s.Authenticate(ctx, tok)
	return tok, sess, codes, err
}

// ---------------------------------------------------------------------------
// Login só com o código do autenticador (ou de recuperação)

func (s *Service) LoginWithCode(ctx context.Context, email, code string, c Client) (string, *Session, error) {
	email = strings.TrimSpace(email)
	code = strings.TrimSpace(code)
	if email == "" || len(email) > 254 || code == "" || len(code) > 64 {
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
		userID string
		active bool
		mfa    bool
	)
	err = s.db.QueryRow(ctx, `SELECT id, is_active, mfa_enabled FROM users WHERE email = $1`, email).Scan(&userID, &active, &mfa)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!active || !mfa)) {
		_, _ = validateTOTP("JBSWY3DPEHPK3PXP", code, time.Now(), 0) // custo parecido ao de um usuário real
		s.recordFailure(ctx, email, userID, c)
		return "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}

	var token string
	ok := false
	method := "totp"
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if isTOTPShape(code) {
			var (
				enc      []byte
				lastStep int64
			)
			if err := tx.QueryRow(ctx, `SELECT mfa_secret_enc, mfa_last_step FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&enc, &lastStep); err != nil {
				return err
			}
			secret, err := s.box.Open(enc, mfaAAD(userID))
			if err != nil {
				return err
			}
			step, valid := validateTOTP(string(secret), code, time.Now(), lastStep)
			if !valid {
				return nil
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET mfa_last_step = $2 WHERE id = $1`, userID, step); err != nil {
				return err
			}
		} else {
			method = "recovery"
			tag, err := tx.Exec(ctx, `UPDATE recovery_codes SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, codeHash(code))
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return nil
			}
		}
		ok = true
		if _, err := tx.Exec(ctx, `INSERT INTO login_attempts (email, ip, success) VALUES ($1, NULLIF($2, '')::inet, true)`, email, c.IP); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID); err != nil {
			return err
		}
		if token, err = s.createSession(ctx, tx, userID, true, c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: audit.LoginSuccess, TargetType: "user", TargetID: userID, IP: c.IP,
			Metadata: map[string]any{"method": method}})
	})
	if err != nil {
		return "", nil, err
	}
	if !ok {
		s.recordFailure(ctx, email, userID, c)
		return "", nil, ErrInvalidCredentials
	}
	sess, err := s.Authenticate(ctx, token)
	return token, sess, err
}
