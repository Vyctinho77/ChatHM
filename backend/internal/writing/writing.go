// Package writing guarda a medição do corretor do campo de mensagem (acentos e erros de digitação): só contadores por palavra,
// nunca texto de mensagem nem quem digitou. Serve para melhorar a lista de palavras com uso real. Ver docs/corretor.md.
package writing

import (
	"context"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

const (
	maxEntries = 200  // por envio
	maxCount   = 1000 // por contador e por envio
)

// Camadas do corretor (as mesmas do front-end e do CHECK da tabela).
var layers = map[string]bool{"curated": true, "extended": true, "suffix": true, "typo_tr": true, "typo_ins": true, "typo_dup": true, "typo_key": true}

var reWord = regexp.MustCompile(`^\pL{1,40}$`)

// Entry: uma palavra corrigida, e quantas vezes (e quantas foram desfeitas) desde o último envio.
type Entry struct {
	Layer   string `json:"layer"`
	From    string `json:"from"`
	To      string `json:"to"`
	Applied int    `json:"applied"`
	Undone  int    `json:"undone"`
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// Record soma os contadores. Tudo ou nada: um item inválido recusa o lote inteiro.
func (s *Service) Record(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 || len(entries) > maxEntries {
		return httpx.FieldErrors{"entries": "Envie de 1 a 200 itens."}
	}
	for i, e := range entries {
		field := "entries[" + strconv.Itoa(i) + "]"
		switch {
		case !layers[e.Layer]:
			return httpx.FieldErrors{field: "Camada desconhecida."}
		case !reWord.MatchString(e.From) || !reWord.MatchString(e.To):
			return httpx.FieldErrors{field: "Só palavras soltas, com letras (até 40)."}
		case e.Applied < 0 || e.Undone < 0 || e.Applied > maxCount || e.Undone > maxCount || e.Applied+e.Undone == 0:
			return httpx.FieldErrors{field: "Contadores inválidos."}
		}
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		for _, e := range entries {
			if _, err := tx.Exec(ctx, `
				INSERT INTO writing_stats (layer, from_word, to_word, applied, undone) VALUES ($1, lower($2), lower($3), $4, $5)
				ON CONFLICT (layer, from_word, to_word) DO UPDATE
				   SET applied = writing_stats.applied + EXCLUDED.applied, undone = writing_stats.undone + EXCLUDED.undone, updated_at = now()`,
				e.Layer, e.From, e.To, e.Applied, e.Undone); err != nil {
				return err
			}
		}
		return nil
	})
}

// LayerTotal: o resumo de uma camada.
type LayerTotal struct {
	Layer       string  `json:"layer"`
	Applied     int64   `json:"applied"`
	Undone      int64   `json:"undone"`
	UndoRate    float64 `json:"undoRate"` // desfeitas / aplicadas
	Words       int     `json:"words"`
	UndoneWords int     `json:"undoneWords"` // palavras desfeitas ao menos uma vez
}

// Word: uma palavra e como ela foi.
type Word struct {
	Layer    string  `json:"layer"`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Applied  int64   `json:"applied"`
	Undone   int64   `json:"undone"`
	UndoRate float64 `json:"undoRate"`
}

type Report struct {
	Layers []LayerTotal `json:"layers"`
	// Suspicious: as palavras mais desfeitas (candidatas a sair da lista), com pelo menos `minApplied` aplicações.
	Suspicious []Word `json:"suspicious"`
	// Popular: as correções mais usadas que ninguém desfez (o que a lista acerta).
	Popular []Word `json:"popular"`
}

// Stats monta o relatório para o administrador.
func (s *Service) Stats(ctx context.Context, minApplied, limit int) (Report, error) {
	if minApplied < 1 {
		minApplied = 5
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rep := Report{Layers: []LayerTotal{}, Suspicious: []Word{}, Popular: []Word{}}
	rows, err := s.db.Query(ctx, `
		SELECT layer, sum(applied), sum(undone), count(*), count(*) FILTER (WHERE undone > 0) FROM writing_stats GROUP BY layer ORDER BY layer`)
	if err != nil {
		return rep, err
	}
	for rows.Next() {
		var l LayerTotal
		if err := rows.Scan(&l.Layer, &l.Applied, &l.Undone, &l.Words, &l.UndoneWords); err != nil {
			rows.Close()
			return rep, err
		}
		if l.Applied > 0 {
			l.UndoRate = float64(l.Undone) / float64(l.Applied)
		}
		rep.Layers = append(rep.Layers, l)
	}
	rows.Close()
	words := func(query string, args ...any) ([]Word, error) {
		rs, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rs.Close()
		out := []Word{}
		for rs.Next() {
			var w Word
			if err := rs.Scan(&w.Layer, &w.From, &w.To, &w.Applied, &w.Undone); err != nil {
				return nil, err
			}
			if w.Applied > 0 {
				w.UndoRate = float64(w.Undone) / float64(w.Applied)
			}
			out = append(out, w)
		}
		return out, rs.Err()
	}
	if rep.Suspicious, err = words(`
		SELECT layer, from_word, to_word, applied, undone FROM writing_stats
		 WHERE undone > 0 AND applied >= $1 ORDER BY undone::float / applied DESC, undone DESC LIMIT $2`, minApplied, limit); err != nil {
		return rep, err
	}
	rep.Popular, err = words(`
		SELECT layer, from_word, to_word, applied, undone FROM writing_stats WHERE undone = 0 ORDER BY applied DESC LIMIT $1`, limit)
	return rep, err
}

type Handler struct{ svc *Service }

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Routes: qualquer pessoa logada envia os contadores; só o administrador lê o relatório.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/writing/stats", h.record)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/writing/stats", h.stats)
	})
}

func (h *Handler) record(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Entries []Entry `json:"entries"`
	}
	if !httpx.Decode(w, r, &in) {
		return
	}
	if err := h.svc.Record(r.Context(), in.Entries); err != nil {
		if fe, ok := err.(httpx.FieldErrors); ok {
			httpx.Validation(w, fe)
			return
		}
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	min, _ := strconv.Atoi(q.Get("min"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	rep, err := h.svc.Stats(r.Context(), min, limit)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}
