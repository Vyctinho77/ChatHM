package auth

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var errEmailTaken = errors.New("email already registered")

// Register always creates an agent without team membership. Account and session
// are committed together; clients cannot choose a role or bypass existing MFA.
func (s *Service) Register(ctx context.Context, in RegisterInput, c Client) (string, *Session, error) {
	// Persist the attempt before validation/hashing, including failed submissions.
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "register:"+c.IP); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='auth.register.attempt' AND ip IS NOT DISTINCT FROM NULLIF($1,'')::inet AND created_at > now()-interval '15 minutes'`, c.IP).Scan(&n); err != nil {
			return err
		}
		if n >= 5 {
			return ErrTooManyAttempts
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "auth.register.attempt", IP: c.IP})
	})
	if err != nil {
		return "", nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	fields := httpx.FieldErrors{}
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		fields["name"] = "Informe um nome de até 120 caracteres."
	}
	addr, e := mail.ParseAddress(in.Email)
	if e != nil || addr.Address != in.Email || len(in.Email) > 254 {
		fields["email"] = "E-mail inválido."
	}
	if e := ValidatePassword(in.Password); e != nil {
		fields["password"] = e.Error()
	}
	if e := fields.Err(); e != nil {
		return "", nil, e
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return "", nil, err
	}
	var token string
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO users (name,email,password_hash,role,last_login_at) VALUES ($1,$2,$3,'agent',now()) RETURNING id`, in.Name, in.Email, hash).Scan(&id); err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23505" {
				return errEmailTaken
			}
			return err
		}
		var err error
		token, err = s.createSession(ctx, tx, id, true, c)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: id, Action: "auth.register.success", TargetType: "user", TargetID: id, IP: c.IP})
	})
	if err != nil {
		return "", nil, err
	}
	sess, err := s.Authenticate(ctx, token)
	return token, sess, err
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in RegisterInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	token, sess, err := s.Register(r.Context(), in, ClientFrom(r))
	var fields httpx.FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.Validation(w, fields)
	case errors.Is(err, errEmailTaken):
		httpx.Error(w, http.StatusConflict, "email_unavailable", "Não foi possível usar este e-mail. Entre com sua conta ou use outro endereço.")
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", "Muitas tentativas. Tente novamente mais tarde.")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		s.setCookie(w, token)
		httpx.JSON(w, http.StatusCreated, s.me(sess))
	}
}
