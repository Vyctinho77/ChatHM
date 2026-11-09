// Package agenda persists team appointments and deadlines.
package agenda

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
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var ErrNotFound = errors.New("agenda: item ou equipe não encontrado")

type Contact struct {
	Name           string `json:"name"`
	ConversationID string `json:"conversationId"`
}
type Item struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Date    string   `json:"date"`
	Time    string   `json:"time,omitempty"`
	TeamID  string   `json:"teamId"`
	Team    string   `json:"team"`
	Contact *Contact `json:"contact,omitempty"`
	Note    string   `json:"note"`
	Done    bool     `json:"done"`
}
type Input struct {
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Date           string `json:"date"`
	Time           string `json:"time"`
	TeamID         string `json:"teamId"`
	ConversationID string `json:"conversationId"`
	Note           string `json:"note"`
	Done           bool   `json:"done"`
}

func (in *Input) validate() error {
	f := httpx.FieldErrors{}
	in.Title = strings.TrimSpace(in.Title)
	in.Note = strings.TrimSpace(in.Note)
	if in.Kind != "appointment" && in.Kind != "deadline" {
		f["kind"] = "Tipo inválido."
	}
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > 120 {
		f["title"] = "Informe um título de até 120 caracteres."
	}
	if d, e := time.Parse("2006-01-02", in.Date); e != nil || d.Year() < 1 || d.Format("2006-01-02") != in.Date {
		f["date"] = "Data inválida."
	}
	if in.Time != "" {
		if t, e := time.Parse("15:04", in.Time); e != nil || t.Format("15:04") != in.Time {
			f["time"] = "Hora inválida."
		}
	}
	if in.Kind == "deadline" {
		in.Time = ""
	}
	if !httpx.ValidUUID(in.TeamID) {
		f["team"] = "Escolha uma equipe válida."
	}
	if in.ConversationID != "" && !httpx.ValidUUID(in.ConversationID) {
		f["contact"] = "Conversa inválida."
	}
	if utf8.RuneCountInString(in.Note) > 2000 {
		f["note"] = "Use até 2000 caracteres."
	}
	return f.Err()
}

type Service struct {
	db    *pgxpool.Pool
	convs *conversations.Service
}

func NewService(db *pgxpool.Pool, convs *conversations.Service) *Service {
	return &Service{db: db, convs: convs}
}

const visible = `($2::boolean OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.team_id=a.team_id AND tm.user_id=$1::uuid))`
const selectItem = `SELECT a.id,a.kind,a.title,a.scheduled_date::text,coalesce(to_char(a.scheduled_time,'HH24:MI'),''),a.team_id,t.name::text,a.conversation_id::text,a.note,a.done FROM agenda_items a JOIN teams t ON t.id=a.team_id`

func scan(row pgx.Row) (Item, error) {
	var i Item
	var conv *string
	err := row.Scan(&i.ID, &i.Kind, &i.Title, &i.Date, &i.Time, &i.TeamID, &i.Team, &conv, &i.Note, &i.Done)
	if conv != nil {
		i.Contact = &Contact{ConversationID: *conv}
	}
	return i, err
}
func (s *Service) contact(ctx context.Context, sess *auth.Session, i *Item) error {
	if i.Contact == nil {
		return nil
	}
	c, err := s.convs.Get(ctx, conversations.Viewer{ID: sess.UserID, Name: sess.Name, Admin: sess.IsAdmin(), Supervisor: sess.IsManager()}, i.Contact.ConversationID)
	if errors.Is(err, conversations.ErrNotFound) {
		i.Contact = nil
		return nil
	}
	if err != nil {
		return err
	}
	i.Contact.Name = c.Contact.Name
	return nil
}
func (s *Service) List(ctx context.Context, sess *auth.Session) ([]Item, error) {
	rows, err := s.db.Query(ctx, selectItem+` WHERE `+visible+` ORDER BY a.scheduled_date,a.scheduled_time NULLS FIRST,a.title`, sess.UserID, sess.IsAdmin())
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for rows.Next() {
		i, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for n := range out {
		if err := s.contact(ctx, sess, &out[n]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, sess *auth.Session, id string) (Item, error) {
	if !httpx.ValidUUID(id) {
		return Item{}, ErrNotFound
	}
	i, err := scan(s.db.QueryRow(ctx, selectItem+` WHERE `+visible+` AND a.id=$3`, sess.UserID, sess.IsAdmin(), id))
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	err = s.contact(ctx, sess, &i)
	return i, err
}
func accessible(ctx context.Context, tx pgx.Tx, sess *auth.Session, team string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams t WHERE t.id=$1 AND ($2::boolean OR EXISTS(SELECT 1 FROM team_members tm WHERE tm.team_id=t.id AND tm.user_id=$3)))`, team, sess.IsAdmin(), sess.UserID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
func (s *Service) Save(ctx context.Context, sess *auth.Session, id, ip string, in Input) (Item, error) {
	if !httpx.ValidUUID(id) {
		return Item{}, ErrNotFound
	}
	if err := in.validate(); err != nil {
		return Item{}, err
	}
	if in.ConversationID != "" {
		if _, err := s.convs.Get(ctx, conversations.Viewer{ID: sess.UserID, Name: sess.Name, Admin: sess.IsAdmin(), Supervisor: sess.IsManager()}, in.ConversationID); err != nil {
			if errors.Is(err, conversations.ErrNotFound) {
				return Item{}, ErrNotFound
			}
			return Item{}, err
		}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "agenda:"+id); err != nil {
			return err
		}
		var oldTeam string
		err := tx.QueryRow(ctx, `SELECT team_id FROM agenda_items WHERE id=$1 FOR UPDATE`, id).Scan(&oldTeam)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		action := "agenda.create"
		if err == nil {
			action = "agenda.update"
			if err := accessible(ctx, tx, sess, oldTeam); err != nil {
				return err
			}
		}
		if err := accessible(ctx, tx, sess, in.TeamID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO agenda_items(id,kind,title,scheduled_date,scheduled_time,team_id,conversation_id,note,done,created_by)
   VALUES($1,$2,$3,$4::text::date,NULLIF($5,'')::time,$6,NULLIF($7,'')::uuid,$8,$9,$10)
   ON CONFLICT(id) DO UPDATE SET kind=EXCLUDED.kind,title=EXCLUDED.title,scheduled_date=EXCLUDED.scheduled_date,scheduled_time=EXCLUDED.scheduled_time,team_id=EXCLUDED.team_id,conversation_id=EXCLUDED.conversation_id,note=EXCLUDED.note,done=EXCLUDED.done`, id, in.Kind, in.Title, in.Date, in.Time, in.TeamID, in.ConversationID, in.Note, in.Done, sess.UserID)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: sess.UserID, Action: action, TargetType: "agenda", TargetID: id, IP: ip})
	})
	if err != nil {
		return Item{}, err
	}
	return s.Get(ctx, sess, id)
}
func (s *Service) Mutate(ctx context.Context, sess *auth.Session, id, ip string, done *bool) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var team string
		err := tx.QueryRow(ctx, `SELECT team_id FROM agenda_items WHERE id=$1 FOR UPDATE`, id).Scan(&team)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := accessible(ctx, tx, sess, team); err != nil {
			return err
		}
		action := "agenda.delete"
		if done == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agenda_items WHERE id=$1`, id)
		} else {
			action = "agenda.done"
			_, err = tx.Exec(ctx, `UPDATE agenda_items SET done=$2 WHERE id=$1`, id, *done)
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: sess.UserID, Action: action, TargetType: "agenda", TargetID: id, IP: ip})
	})
}
