// Package teams gerencia equipes e seus membros.
//
// Visibilidade: admin vê todas as equipes; atendente vê apenas as equipes de que participa.
// Para quem não tem acesso, uma equipe existente responde como inexistente (404).
package teams

import (
	"context"
	"errors"
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
	ErrNotFound     = errors.New("teams: equipe não encontrada")
	ErrNameTaken    = errors.New("teams: nome já utilizado")
	ErrUserNotFound = errors.New("teams: usuário não encontrado")
	ErrInUse        = errors.New("teams: equipe vinculada a segmentos")
)

type Team struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MemberCount int       `json:"memberCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Member struct {
	UserID   string    `json:"userId"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     auth.Role `json:"role"`
	IsActive bool      `json:"isActive"`
	JoinedAt time.Time `json:"joinedAt"`
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// IsMember informa se o usuário pertence à equipe. Usado também por outros módulos.
func IsMember(ctx context.Context, db database.DBTX, teamID, userID string) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`, teamID, userID).Scan(&ok)
	return ok, err
}

// canView: admin vê tudo; demais apenas equipes das quais participam.
func (s *Service) canView(ctx context.Context, sess *auth.Session, teamID string) error {
	if !httpx.ValidUUID(teamID) {
		return ErrNotFound
	}
	if sess.IsAdmin() {
		return nil
	}
	ok, err := IsMember(ctx, s.db, teamID, sess.UserID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// canManageMembers: o administrador mexe em qualquer equipe; o gestor só nas equipes de que participa
// (as demais respondem como inexistentes, sem confirmar que existem).
func (s *Service) canManageMembers(ctx context.Context, sess *auth.Session, teamID string) error {
	if sess.IsAdmin() {
		return nil
	}
	if !sess.IsManager() {
		return ErrNotFound
	}
	ok, err := IsMember(ctx, s.db, teamID, sess.UserID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

const selectTeam = `
	SELECT t.id, t.name, t.description, t.created_at,
	       (SELECT count(*) FROM team_members m WHERE m.team_id = t.id)
	  FROM teams t`

func scanTeam(row pgx.Row) (Team, error) {
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Description, &t.CreatedAt, &t.MemberCount)
	return t, err
}

func (s *Service) List(ctx context.Context, sess *auth.Session) ([]Team, error) {
	q := selectTeam + ` ORDER BY t.name`
	args := []any{}
	if !sess.IsAdmin() {
		q = selectTeam + ` WHERE EXISTS (SELECT 1 FROM team_members m WHERE m.team_id = t.id AND m.user_id = $1) ORDER BY t.name`
		args = append(args, sess.UserID)
	}
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, sess *auth.Session, id string) (Team, error) {
	if err := s.canView(ctx, sess, id); err != nil {
		return Team{}, err
	}
	t, err := scanTeam(s.db.QueryRow(ctx, selectTeam+` WHERE t.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}
	return t, err
}

type Input struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (in *Input) validate(requireName bool) error {
	f := httpx.FieldErrors{}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if (requireName && in.Name == nil) || (in.Name != nil && (*in.Name == "" || utf8.RuneCountInString(*in.Name) > 80)) {
		f["name"] = "Informe um nome de até 80 caracteres."
	}
	if in.Description != nil {
		d := strings.TrimSpace(*in.Description)
		in.Description = &d
		if utf8.RuneCountInString(d) > 500 {
			f["description"] = "Descrição deve ter até 500 caracteres."
		}
	}
	return f.Err()
}

func (s *Service) Create(ctx context.Context, sess *auth.Session, ip string, in Input) (Team, error) {
	if err := in.validate(true); err != nil {
		return Team{}, err
	}
	desc := ""
	if in.Description != nil {
		desc = *in.Description
	}
	var id string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO teams (name, description) VALUES ($1, $2) RETURNING id`,
			*in.Name, desc).Scan(&id)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.TeamCreate, TargetType: "team", TargetID: id, IP: ip,
			Metadata: map[string]any{"name": *in.Name},
		})
	})
	if err != nil {
		return Team{}, err
	}
	return s.Get(ctx, sess, id)
}

func (s *Service) Update(ctx context.Context, sess *auth.Session, ip, id string, in Input) (Team, error) {
	if !httpx.ValidUUID(id) {
		return Team{}, ErrNotFound
	}
	if err := in.validate(false); err != nil {
		return Team{}, err
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE teams SET name = coalesce($2, name), description = coalesce($3, description) WHERE id = $1`,
			id, in.Name, in.Description)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.TeamUpdate, TargetType: "team", TargetID: id, IP: ip,
		})
	})
	if err != nil {
		return Team{}, err
	}
	return s.Get(ctx, sess, id)
}

func (s *Service) Delete(ctx context.Context, sess *auth.Session, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `DELETE FROM teams WHERE id = $1 RETURNING name`, id).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if database.IsCode(err, database.ForeignKeyViolation) {
			return ErrInUse // segmentos vinculados: excluir tornaria a fila visível a todos
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.TeamDelete, TargetType: "team", TargetID: id, IP: ip,
			Metadata: map[string]any{"name": name},
		})
	})
}

func (s *Service) Members(ctx context.Context, sess *auth.Session, teamID string) ([]Member, error) {
	if _, err := s.Get(ctx, sess, teamID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.is_active, m.created_at
		  FROM team_members m JOIN users u ON u.id = m.user_id
		 WHERE m.team_id = $1
		 ORDER BY u.name, u.email`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		var role string
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &role, &m.IsActive, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = auth.Role(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMember é idempotente: adicionar alguém que já é membro não gera erro.
func (s *Service) AddMember(ctx context.Context, sess *auth.Session, ip, teamID, userID string) error {
	if !httpx.ValidUUID(teamID) {
		return ErrNotFound
	}
	if !httpx.ValidUUID(userID) {
		return ErrUserNotFound
	}
	if err := s.canManageMembers(ctx, sess, teamID); err != nil {
		return err
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE id = $1)`, teamID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, teamID, userID)
		if database.IsCode(err, database.ForeignKeyViolation) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.TeamMemberAdd, TargetType: "team", TargetID: teamID, IP: ip,
			Metadata: map[string]any{"userId": userID},
		})
	})
}

func (s *Service) RemoveMember(ctx context.Context, sess *auth.Session, ip, teamID, userID string) error {
	if !httpx.ValidUUID(teamID) || !httpx.ValidUUID(userID) {
		return ErrNotFound
	}
	if err := s.canManageMembers(ctx, sess, teamID); err != nil {
		return err
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorID: sess.UserID, Action: audit.TeamMemberRemove, TargetType: "team", TargetID: teamID, IP: ip,
			Metadata: map[string]any{"userId": userID},
		})
	})
}
