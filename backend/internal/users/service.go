// Package users gerencia usuários e perfis (admin, atendente).
package users

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var (
	ErrNotFound       = errors.New("users: usuário não encontrado")
	ErrEmailTaken     = errors.New("users: e-mail já cadastrado")
	ErrTeamNotFound   = errors.New("users: equipe não encontrada")
	ErrLastAdmin      = errors.New("users: deve existir ao menos um administrador ativo")
	ErrSelfDeactivate = errors.New("users: não é possível desativar a si mesmo")
)

type TeamRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        auth.Role  `json:"role"`
	IsActive    bool       `json:"isActive"`
	MFAEnabled  bool       `json:"mfaEnabled"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	Teams       []TeamRef  `json:"teams"`
}

type Service struct {
	db     *pgxpool.Pool
	hasher auth.Hasher
}

func NewService(db *pgxpool.Pool, hasher auth.Hasher) *Service {
	return &Service{db: db, hasher: hasher}
}

const selectUser = `
	SELECT u.id, u.email, u.name, u.role, u.is_active, u.mfa_enabled, u.last_login_at, u.created_at,
	       coalesce(json_agg(json_build_object('id', t.id, 'name', t.name) ORDER BY t.name)
	                FILTER (WHERE t.id IS NOT NULL), '[]')
	  FROM users u
	  LEFT JOIN team_members tm ON tm.user_id = u.id
	  LEFT JOIN teams t ON t.id = tm.team_id`

func scanUser(row pgx.Row) (User, error) {
	var u User
	var role string
	err := row.Scan(&u.ID, &u.Email, &u.Name, &role, &u.IsActive, &u.MFAEnabled, &u.LastLoginAt, &u.CreatedAt, &u.Teams)
	u.Role = auth.Role(role)
	return u, err
}

func (s *Service) List(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, selectUser+` GROUP BY u.id ORDER BY u.name, u.email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	if !httpx.ValidUUID(id) {
		return User{}, ErrNotFound
	}
	u, err := scanUser(s.db.QueryRow(ctx, selectUser+` WHERE u.id = $1 GROUP BY u.id`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

type CreateInput struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Password string    `json:"password"`
	Role     auth.Role `json:"role"`
	TeamIDs  []string  `json:"teamIds"`
}

func normalizeEmail(raw string) (string, bool) {
	e := strings.TrimSpace(raw)
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 {
		return "", false
	}
	return strings.ToLower(e), true
}

func validName(n string) bool {
	c := utf8.RuneCountInString(n)
	return c >= 1 && c <= 120
}

func (in *CreateInput) validate() error {
	f := httpx.FieldErrors{}
	email, ok := normalizeEmail(in.Email)
	if !ok {
		f["email"] = "E-mail inválido."
	}
	in.Email = email
	in.Name = strings.TrimSpace(in.Name)
	if !validName(in.Name) {
		f["name"] = "Informe um nome de até 120 caracteres."
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		f["password"] = err.Error()
	}
	if !in.Role.Valid() {
		f["role"] = "Perfil deve ser admin ou agent."
	}
	for _, id := range in.TeamIDs {
		if !httpx.ValidUUID(id) {
			f["teamIds"] = "Equipe inválida."
			break
		}
	}
	return f.Err()
}

// Create cadastra um usuário e, opcionalmente, vincula-o a equipes.
// actorID vazio indica criação pelo sistema (CLI de bootstrap).
func (s *Service) Create(ctx context.Context, actorID, ip string, in CreateInput) (User, error) {
	if err := in.validate(); err != nil {
		return User{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return User{}, err
	}
	var id string
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO users (email, name, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id`,
			in.Email, in.Name, hash, string(in.Role)).Scan(&id)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrEmailTaken
		}
		if err != nil {
			return err
		}
		for _, teamID := range in.TeamIDs {
			_, err := tx.Exec(ctx,
				`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, teamID, id)
			if database.IsCode(err, database.ForeignKeyViolation) {
				return ErrTeamNotFound
			}
			if err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: actorID, Action: audit.UserCreate, TargetType: "user", TargetID: id, IP: ip,
			Metadata: map[string]any{"role": in.Role, "teamIds": in.TeamIDs},
		})
	})
	if err != nil {
		return User{}, err
	}
	return s.Get(ctx, id)
}

type UpdateInput struct {
	Name     *string    `json:"name"`
	Role     *auth.Role `json:"role"`
	IsActive *bool      `json:"isActive"`
}

// Update altera nome, perfil e status. Garante que sempre reste um admin ativo.
func (s *Service) Update(ctx context.Context, actorID, ip, id string, in UpdateInput) (User, error) {
	if !httpx.ValidUUID(id) {
		return User{}, ErrNotFound
	}
	f := httpx.FieldErrors{}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
		if !validName(n) {
			f["name"] = "Informe um nome de até 120 caracteres."
		}
	}
	if in.Role != nil && !in.Role.Valid() {
		f["role"] = "Perfil deve ser admin ou agent."
	}
	if err := f.Err(); err != nil {
		return User{}, err
	}
	if in.IsActive != nil && !*in.IsActive && id == actorID {
		return User{}, ErrSelfDeactivate
	}

	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		// Mudanças de perfil/status travam antes todos os admins ativos, sempre na
		// mesma ordem: serializa alterações concorrentes sem risco de deadlock.
		activeAdmins := map[string]bool{}
		if in.Role != nil || in.IsActive != nil {
			rows, err := tx.Query(ctx,
				`SELECT id FROM users WHERE role = 'admin' AND is_active ORDER BY id FOR UPDATE`)
			if err != nil {
				return err
			}
			for rows.Next() {
				var aid string
				if err := rows.Scan(&aid); err != nil {
					rows.Close()
					return err
				}
				activeAdmins[aid] = true
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}

		var curRole string
		var curActive bool
		err := tx.QueryRow(ctx, `SELECT role, is_active FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&curRole, &curActive)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		newRole, newActive := curRole, curActive
		if in.Role != nil {
			newRole = string(*in.Role)
		}
		if in.IsActive != nil {
			newActive = *in.IsActive
		}
		losingAdmin := activeAdmins[id] && (newRole != string(auth.RoleAdmin) || !newActive)
		if losingAdmin && len(activeAdmins) <= 1 {
			return ErrLastAdmin
		}

		if _, err := tx.Exec(ctx, `
			UPDATE users SET name = coalesce($2, name), role = $3, is_active = $4 WHERE id = $1`,
			id, in.Name, newRole, newActive); err != nil {
			return err
		}
		if curActive && !newActive {
			if err := auth.RevokeUserSessions(ctx, tx, id, ""); err != nil {
				return err
			}
		}
		meta := map[string]any{}
		if in.Name != nil {
			meta["name"] = true
		}
		if newRole != curRole {
			meta["role"] = newRole
		}
		if newActive != curActive {
			meta["isActive"] = newActive
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: actorID, Action: audit.UserUpdate, TargetType: "user", TargetID: id, IP: ip, Metadata: meta,
		})
	})
	if err != nil {
		return User{}, err
	}
	return s.Get(ctx, id)
}

// ResetPassword define uma nova senha (ação do admin) e encerra as sessões do usuário.
func (s *Service) ResetPassword(ctx context.Context, actorID, ip, id, password string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	if err := auth.ValidatePassword(password); err != nil {
		return httpx.FieldErrors{"password": err.Error()}
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE users SET password_hash = $2, password_changed_at = now() WHERE id = $1`, id, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := auth.RevokeUserSessions(ctx, tx, id, ""); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: actorID, Action: audit.UserPasswordReset, TargetType: "user", TargetID: id, IP: ip,
		})
	})
}

// SetTheme salva a preferência de tema do próprio usuário.
func (s *Service) SetTheme(ctx context.Context, userID, theme string) error {
	switch theme {
	case "light", "dark", "system":
	default:
		return httpx.FieldErrors{"theme": "Tema deve ser light, dark ou system."}
	}
	_, err := s.db.Exec(ctx, `UPDATE users SET theme = $2 WHERE id = $1`, userID, theme)
	return err
}

// FindIDByEmail é usado pela CLI de administração.
func (s *Service) FindIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, strings.TrimSpace(email)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
