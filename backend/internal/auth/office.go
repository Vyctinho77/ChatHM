package auth

// Instância do escritório: o administrador nasce pelo QR do WhatsApp (instalação inicial) e a equipe entra com o
// token do escritório (criando a própria conta, com autenticador). Um escritório por servidor.

import (
	"context"
	"crypto/subtle"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var (
	ErrInvalidOfficeToken = errors.New("auth: token do escritório inválido")
	ErrSetupNeedsWhatsApp = errors.New("auth: conecte o WhatsApp antes")
	ErrEmailUnavailable   = errors.New("auth: e-mail indisponível")
)

var aadOfficeToken = []byte("office-token")

// SetWhatsAppGate informa se o WhatsApp do escritório está conectado (exigido na instalação inicial).
func (s *Service) SetWhatsAppGate(connected func() bool) { s.waConnected = connected }

// newOfficeToken: "XXXX-XXXX-XXXX" (~60 bits).
func newOfficeToken() (string, error) {
	c, err := newCode() // XXXXX-XXXXX
	if err != nil {
		return "", err
	}
	c2, err := newCode()
	if err != nil {
		return "", err
	}
	raw := normalizeCode(c) + normalizeCode(c2)[:2] // 12 caracteres
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12], nil
}

func (s *Service) storeNewOfficeToken(ctx context.Context, db database.DBTX, rotate bool) (string, error) {
	tok, err := newOfficeToken()
	if err != nil {
		return "", err
	}
	enc, err := s.box.Seal([]byte(normalizeCode(tok)), aadOfficeToken)
	if err != nil {
		return "", err
	}
	if rotate {
		_, err = db.Exec(ctx, `
			INSERT INTO office_access (token_enc, rotated_at) VALUES ($1, now())
			ON CONFLICT (id) DO UPDATE SET token_enc = EXCLUDED.token_enc, rotated_at = now()`, enc)
	} else {
		_, err = db.Exec(ctx, `INSERT INTO office_access (token_enc) VALUES ($1) ON CONFLICT (id) DO NOTHING`, enc)
	}
	return tok, err
}

// OfficeAccess é o que o administrador vê em "Acesso da equipe".
type OfficeAccess struct {
	Token         string `json:"token"`
	AllowedDomain string `json:"allowedDomain"`
}

func (s *Service) loadOffice(ctx context.Context, db database.DBTX) (token, domain string, err error) {
	var enc []byte
	err = db.QueryRow(ctx, `SELECT token_enc, allowed_domain FROM office_access WHERE id`).Scan(&enc, &domain)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = s.storeNewOfficeToken(ctx, db, false); err != nil {
			return "", "", err
		}
		err = db.QueryRow(ctx, `SELECT token_enc, allowed_domain FROM office_access WHERE id`).Scan(&enc, &domain)
	}
	if err != nil {
		return "", "", err
	}
	raw, err := s.box.Open(enc, aadOfficeToken)
	if err != nil {
		return "", "", err
	}
	return string(raw), domain, nil
}

func formatOfficeToken(raw string) string {
	if len(raw) != 12 {
		return raw
	}
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12]
}

func (s *Service) OfficeAccess(ctx context.Context) (OfficeAccess, error) {
	raw, dom, err := s.loadOffice(ctx, s.db)
	return OfficeAccess{Token: formatOfficeToken(raw), AllowedDomain: dom}, err
}

// RotateOfficeToken gera um novo token; o anterior deixa de valer para novos cadastros (contas já criadas seguem ativas).
func (s *Service) RotateOfficeToken(ctx context.Context, actorID, ip string) (OfficeAccess, error) {
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := s.storeNewOfficeToken(ctx, tx, true); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "office.token.rotate", IP: ip})
	})
	if err != nil {
		return OfficeAccess{}, err
	}
	return s.OfficeAccess(ctx)
}

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// SetAllowedDomain restringe os cadastros a um domínio de e-mail ("escritorio.com.br"); vazio libera qualquer e-mail.
func (s *Service) SetAllowedDomain(ctx context.Context, actorID, ip, domain string) (OfficeAccess, error) {
	domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
	if domain != "" && (len(domain) > 253 || !domainRe.MatchString(domain)) {
		return OfficeAccess{}, httpx.FieldErrors{"allowedDomain": "Informe um domínio válido, como escritorio.com.br."}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, _, err := s.loadOffice(ctx, tx); err != nil { // garante a linha
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE office_access SET allowed_domain = $1 WHERE id`, domain); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "office.domain.set", IP: ip, Metadata: map[string]any{"domain": domain}})
	})
	if err != nil {
		return OfficeAccess{}, err
	}
	return s.OfficeAccess(ctx)
}

// ---------------------------------------------------------------------------
// Instalação inicial (QR do WhatsApp) — só enquanto não há administrador

// CheckSetupOpen: a instalação inicial está aberta (sem administrador) e o código de instalação confere.
func (s *Service) CheckSetupOpen(ctx context.Context, installCode, ip string) error {
	needs, err := s.NeedsSetup(ctx)
	if err != nil {
		return err
	}
	if !needs {
		return ErrSetupDone
	}
	return s.checkInstallCode(ctx, installCode, ip)
}

// ---------------------------------------------------------------------------
// Entrar na equipe com o token do escritório

func (s *Service) checkOfficeToken(ctx context.Context, token, email string, c Client) (domain string, err error) {
	throttled, err := s.throttled(ctx, email, c.IP)
	if err != nil {
		return "", err
	}
	if throttled {
		return "", ErrTooManyAttempts
	}
	raw, dom, err := s.loadOffice(ctx, s.db)
	if err != nil {
		return "", err
	}
	if len(token) > 64 || subtle.ConstantTimeCompare([]byte(normalizeCode(token)), []byte(raw)) != 1 {
		s.recordFailure(ctx, email, "", c)
		return "", ErrInvalidOfficeToken
	}
	return dom, nil
}

// JoinStart valida token e dados e devolve o segredo do autenticador. Nada é gravado ainda.
func (s *Service) JoinStart(ctx context.Context, name, email, officeToken string, c Client) (Enrollment, error) {
	if needs, err := s.NeedsSetup(ctx); err != nil {
		return Enrollment{}, err
	} else if needs {
		return Enrollment{}, ErrInvalidOfficeToken // ainda sem escritório configurado
	}
	name, email, err := validateIdentity(name, email)
	if err != nil {
		return Enrollment{}, err
	}
	dom, err := s.checkOfficeToken(ctx, officeToken, email, c)
	if err != nil {
		return Enrollment{}, err
	}
	if dom != "" && !strings.HasSuffix(email, "@"+dom) {
		return Enrollment{}, httpx.FieldErrors{"email": "Use seu e-mail do escritório (@" + dom + ")."}
	}
	var taken bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&taken); err != nil {
		return Enrollment{}, err
	}
	if taken {
		return Enrollment{}, ErrEmailUnavailable
	}
	return s.newEnrollment(enrollClaims{Kind: "join", Name: name, Email: email}, email)
}

// JoinFinish cria a conta ativa (atendente, sem equipe, com autenticador) e abre a sessão.
func (s *Service) JoinFinish(ctx context.Context, officeToken, enrollToken, code string, c Client) (string, *Session, []string, error) {
	cl, ok := s.openClaims(enrollToken, "join")
	if !ok {
		return "", nil, nil, ErrInvalidInvite
	}
	if _, err := s.checkOfficeToken(ctx, officeToken, cl.Email, c); err != nil {
		return "", nil, nil, err
	}
	step, ok := validateTOTP(cl.Secret, code, time.Now(), 0)
	if !ok {
		return "", nil, nil, ErrInvalidCode
	}
	hash, err := s.RandomPasswordHash()
	if err != nil {
		return "", nil, nil, err
	}
	var (
		tok   string
		codes []string
	)
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO users (email, name, password_hash, role, last_login_at) VALUES ($1, $2, $3, 'agent', now()) RETURNING id`,
			cl.Email, cl.Name, hash).Scan(&id)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrEmailUnavailable
		}
		if err != nil {
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
		if tok, err = s.createSession(ctx, tx, id, true, c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: id, Action: "auth.join.success", TargetType: "user", TargetID: id, IP: c.IP})
	})
	if err != nil {
		return "", nil, nil, err
	}
	sess, err := s.Authenticate(ctx, tok)
	return tok, sess, codes, err
}
