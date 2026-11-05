// Package tags gerencia o catálogo de etiquetas simples (PENDENTE, URGENTE, PGDAS…).
// Etiquetas nunca alteram o segmento da conversa.
package tags

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var (
	ErrNotFound  = errors.New("tags: etiqueta não encontrada")
	ErrNameTaken = errors.New("tags: nome já utilizado")
)

type Tag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	InUse int    `json:"inUse"` // conversas que usam a etiqueta
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.name::text, (SELECT count(*) FROM conversation_tags ct WHERE ct.tag_id = t.id)
		  FROM tags t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.InUse); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func cleanName(n string) (string, error) {
	n = strings.ToUpper(strings.Join(strings.Fields(n), " "))
	if c := utf8.RuneCountInString(n); c < 1 || c > 30 {
		return "", httpx.FieldErrors{"name": "Informe um nome de até 30 caracteres."}
	}
	return n, nil
}

func (s *Service) Create(ctx context.Context, actorID, ip, name string) (Tag, error) {
	name, err := cleanName(name)
	if err != nil {
		return Tag{}, err
	}
	var t Tag
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO tags (name) VALUES ($1) RETURNING id, name::text`, name).Scan(&t.ID, &t.Name)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.TagCreate, TargetType: "tag", TargetID: t.ID, IP: ip})
	})
	return t, err
}

func (s *Service) Rename(ctx context.Context, actorID, ip, id, name string) (Tag, error) {
	if !httpx.ValidUUID(id) {
		return Tag{}, ErrNotFound
	}
	name, err := cleanName(name)
	if err != nil {
		return Tag{}, err
	}
	var t Tag
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE tags SET name = $2 WHERE id = $1 RETURNING id, name::text`, id, name).Scan(&t.ID, &t.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.TagUpdate, TargetType: "tag", TargetID: id, IP: ip})
	})
	return t, err
}

func (s *Service) Delete(ctx context.Context, actorID, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM tags WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.TagDelete, TargetType: "tag", TargetID: id, IP: ip})
	})
}

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Routes: catálogo legível por todos; criar/renomear/excluir só admin.
// (Aplicar/remover em conversas fica no módulo conversations.)
func (h *Handler) Routes(r chi.Router) {
	r.Get("/tags", func(w http.ResponseWriter, r *http.Request) {
		ts, err := h.svc.List(r.Context())
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"tags": ts})
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Post("/tags", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Name string `json:"name"`
			}
			if !httpx.Decode(w, r, &in) {
				return
			}
			t, err := h.svc.Create(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), in.Name)
			if err != nil {
				writeError(w, r, err)
				return
			}
			httpx.JSON(w, http.StatusCreated, t)
		})
		r.Patch("/tags/{id}", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Name string `json:"name"`
			}
			if !httpx.Decode(w, r, &in) {
				return
			}
			t, err := h.svc.Rename(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id"), in.Name)
			if err != nil {
				writeError(w, r, err)
				return
			}
			httpx.JSON(w, http.StatusOK, t)
		})
		r.Delete("/tags/{id}", func(w http.ResponseWriter, r *http.Request) {
			if err := h.svc.Delete(r.Context(), auth.MustFromContext(r.Context()).UserID, httpx.ClientIP(r), chi.URLParam(r, "id")); err != nil {
				writeError(w, r, err)
				return
			}
			httpx.NoContent(w)
		})
	})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var fe httpx.FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.Validation(w, fe)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Etiqueta não encontrada.")
	case errors.Is(err, ErrNameTaken):
		httpx.Validation(w, httpx.FieldErrors{"name": "Já existe uma etiqueta com esse nome."})
	default:
		httpx.Internal(w, r, err)
	}
}
