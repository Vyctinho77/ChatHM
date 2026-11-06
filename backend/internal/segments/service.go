package segments

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

var (
	ErrNotFound     = errors.New("segments: segmento não encontrado")
	ErrKeywordGone  = errors.New("segments: palavra-chave não encontrada")
	ErrNameTaken    = errors.New("segments: nome já utilizado")
	ErrTeamNotFound = errors.New("segments: equipe não encontrada")
	ErrDuplicate    = errors.New("segments: palavra-chave já cadastrada neste segmento")
)

type KeywordView struct {
	ID       string `json:"id"`
	Phrase   string `json:"phrase"`
	Priority int    `json:"priority"`
}

type Segment struct {
	ID       string        `json:"id"`
	Key      string        `json:"key"`
	Name     string        `json:"name"`
	TeamID   *string       `json:"teamId"`
	TeamName string        `json:"teamName,omitempty"`
	IsActive bool          `json:"isActive"`
	Keywords []KeywordView `json:"keywords,omitempty"`
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// LoadKeywords devolve as palavras-chave dos segmentos ativos, prontas para Classify.
func LoadKeywords(ctx context.Context, db database.DBTX) ([]Keyword, error) {
	rows, err := db.Query(ctx, `
		SELECT s.id, s.key, k.phrase, k.normalized, k.priority
		  FROM segment_keywords k JOIN segments s ON s.id = k.segment_id
		 WHERE s.is_active
		 ORDER BY k.priority DESC, k.phrase`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Keyword
	for rows.Next() {
		var k Keyword
		if err := rows.Scan(&k.SegmentID, &k.SegmentKey, &k.Phrase, &k.Normalized, &k.Priority); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ClassifyText classifica um texto com as palavras-chave atuais (tela de teste do admin).
func (s *Service) ClassifyText(ctx context.Context, text string) (Result, error) {
	kws, err := LoadKeywords(ctx, s.db)
	if err != nil {
		return Result{}, err
	}
	return Classify(text, kws), nil
}

func (s *Service) List(ctx context.Context, withKeywords bool) ([]Segment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id, s.key, s.name, s.team_id, coalesce(t.name::text, ''), s.is_active
		  FROM segments s LEFT JOIN teams t ON t.id = s.team_id
		 ORDER BY s.position, s.name`)
	if err != nil {
		return nil, err
	}
	out := []Segment{}
	byID := map[string]int{}
	for rows.Next() {
		var sg Segment
		if err := rows.Scan(&sg.ID, &sg.Key, &sg.Name, &sg.TeamID, &sg.TeamName, &sg.IsActive); err != nil {
			rows.Close()
			return nil, err
		}
		byID[sg.ID] = len(out)
		out = append(out, sg)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !withKeywords {
		return out, nil
	}
	krows, err := s.db.Query(ctx, `SELECT id, segment_id, phrase, priority FROM segment_keywords ORDER BY priority DESC, phrase`)
	if err != nil {
		return nil, err
	}
	defer krows.Close()
	for krows.Next() {
		var k KeywordView
		var segID string
		if err := krows.Scan(&k.ID, &segID, &k.Phrase, &k.Priority); err != nil {
			return nil, err
		}
		if i, ok := byID[segID]; ok {
			out[i].Keywords = append(out[i].Keywords, k)
		}
	}
	return out, krows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (Segment, error) {
	if !httpx.ValidUUID(id) {
		return Segment{}, ErrNotFound
	}
	all, err := s.List(ctx, true)
	if err != nil {
		return Segment{}, err
	}
	for _, sg := range all {
		if sg.ID == id {
			return sg, nil
		}
	}
	return Segment{}, ErrNotFound
}

func slugify(name string) string {
	var b strings.Builder
	underscore := true
	for _, r := range strings.ToLower(stripAccents(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			underscore = false
		} else if !underscore {
			b.WriteByte('_')
			underscore = true
		}
	}
	key := strings.Trim(b.String(), "_")
	if key == "" {
		key = "segmento"
	}
	if len(key) > 36 {
		key = strings.Trim(key[:36], "_")
	}
	return key
}

func stripAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if r >= 0x300 && r <= 0x36f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func validName(n string) bool {
	c := utf8.RuneCountInString(n)
	return c >= 1 && c <= 80
}

func (s *Service) Create(ctx context.Context, actorID, ip, name string, teamID *string) (Segment, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Segment{}, httpx.FieldErrors{"name": "Informe um nome de até 80 caracteres."}
	}
	if teamID != nil && !httpx.ValidUUID(*teamID) {
		return Segment{}, httpx.FieldErrors{"teamId": "Equipe inválida."}
	}
	var id string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		base := slugify(name)
		key := base
		for i := 2; ; i++ {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM segments WHERE key = $1)`, key).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				break
			}
			key = base + "_" + strconv.Itoa(i)
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO segments (key, name, team_id, position)
			VALUES ($1, $2, $3, coalesce((SELECT max(position) + 1 FROM segments), 0)) RETURNING id`,
			key, name, teamID).Scan(&id)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if database.IsCode(err, database.ForeignKeyViolation) {
			return ErrTeamNotFound
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentCreate, TargetType: "segment", TargetID: id, IP: ip,
			Metadata: map[string]any{"name": name, "teamId": teamID}})
	})
	if err != nil {
		return Segment{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, actorID, ip, id string, name *string, active *bool) (Segment, error) {
	if !httpx.ValidUUID(id) {
		return Segment{}, ErrNotFound
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		name = &n
		if !validName(n) {
			return Segment{}, httpx.FieldErrors{"name": "Informe um nome de até 80 caracteres."}
		}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE segments SET name = coalesce($2, name), is_active = coalesce($3, is_active) WHERE id = $1`, id, name, active)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrNameTaken
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		meta := map[string]any{}
		if name != nil {
			meta["name"] = *name
		}
		if active != nil {
			meta["isActive"] = *active
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentUpdate, TargetType: "segment", TargetID: id, IP: ip, Metadata: meta})
	})
	if err != nil {
		return Segment{}, err
	}
	return s.Get(ctx, id)
}

// SetTeam vincula (ou desvincula, com nil) o segmento a uma equipe. Atenção: o vínculo
// define quem enxerga a fila; sem equipe, a fila fica visível a todos os usuários.
func (s *Service) SetTeam(ctx context.Context, actorID, ip, id string, teamID *string) (Segment, error) {
	if !httpx.ValidUUID(id) {
		return Segment{}, ErrNotFound
	}
	if teamID != nil && !httpx.ValidUUID(*teamID) {
		return Segment{}, httpx.FieldErrors{"teamId": "Equipe inválida."}
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE segments SET team_id = $2 WHERE id = $1`, id, teamID)
		if database.IsCode(err, database.ForeignKeyViolation) {
			return ErrTeamNotFound
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentUpdate, TargetType: "segment", TargetID: id, IP: ip,
			Metadata: map[string]any{"teamId": teamID}})
	})
	if err != nil {
		return Segment{}, err
	}
	return s.Get(ctx, id)
}

// Delete remove o segmento; as conversas dele voltam para a fila geral.
func (s *Service) Delete(ctx context.Context, actorID, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `DELETE FROM segments WHERE id = $1 RETURNING name`, id).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentDelete, TargetType: "segment", TargetID: id, IP: ip,
			Metadata: map[string]any{"name": name}})
	})
}

func validatePhrase(phrase string, priority int) (string, string, error) {
	phrase = strings.TrimSpace(phrase)
	f := httpx.FieldErrors{}
	norm := Normalize(phrase)
	switch {
	case phrase == "" || utf8.RuneCountInString(phrase) > 120:
		f["phrase"] = "Informe uma palavra ou expressão de até 120 caracteres."
	case norm == "":
		f["phrase"] = "Use ao menos uma palavra com significado (palavras como \"de\", \"uma\" e \"para\" são ignoradas)."
	}
	if priority < 0 || priority > 100 {
		f["priority"] = "Prioridade deve ficar entre 0 e 100."
	}
	if err := f.Err(); err != nil {
		return "", "", err
	}
	return phrase, norm, nil
}

func (s *Service) AddKeyword(ctx context.Context, actorID, ip, segmentID, phrase string, priority int) (KeywordView, error) {
	if !httpx.ValidUUID(segmentID) {
		return KeywordView{}, ErrNotFound
	}
	phrase, norm, err := validatePhrase(phrase, priority)
	if err != nil {
		return KeywordView{}, err
	}
	var k KeywordView
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO segment_keywords (segment_id, phrase, normalized, priority) VALUES ($1, $2, $3, $4)
			RETURNING id, phrase, priority`, segmentID, phrase, norm, priority).Scan(&k.ID, &k.Phrase, &k.Priority)
		if database.IsCode(err, database.UniqueViolation) {
			return ErrDuplicate
		}
		if database.IsCode(err, database.ForeignKeyViolation) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentKeywordAdd, TargetType: "segment", TargetID: segmentID, IP: ip,
			Metadata: map[string]any{"phrase": phrase, "priority": priority}})
	})
	return k, err
}

func (s *Service) UpdateKeywordPriority(ctx context.Context, actorID, ip, segmentID, keywordID string, priority int) error {
	if !httpx.ValidUUID(segmentID) || !httpx.ValidUUID(keywordID) {
		return ErrKeywordGone
	}
	if priority < 0 || priority > 100 {
		return httpx.FieldErrors{"priority": "Prioridade deve ficar entre 0 e 100."}
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE segment_keywords SET priority = $3 WHERE id = $2 AND segment_id = $1`, segmentID, keywordID, priority)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrKeywordGone
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentKeywordUpdate, TargetType: "segment", TargetID: segmentID, IP: ip,
			Metadata: map[string]any{"keywordId": keywordID, "priority": priority}})
	})
}

func (s *Service) DeleteKeyword(ctx context.Context, actorID, ip, segmentID, keywordID string) error {
	if !httpx.ValidUUID(segmentID) || !httpx.ValidUUID(keywordID) {
		return ErrKeywordGone
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var phrase string
		err := tx.QueryRow(ctx, `DELETE FROM segment_keywords WHERE id = $2 AND segment_id = $1 RETURNING phrase`, segmentID, keywordID).Scan(&phrase)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKeywordGone
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: audit.SegmentKeywordRemove, TargetType: "segment", TargetID: segmentID, IP: ip,
			Metadata: map[string]any{"phrase": phrase}})
	})
}

// EnsureDefaults aplica os exemplos uma única vez (marcado em seed_flags). Não roda numa instalação
// nova: só com SEED_EXAMPLE_DATA=true (desenvolvimento) ou nos testes. Quem decide os segmentos é o gestor.
func (s *Service) EnsureDefaults(ctx context.Context) error {
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var applied string
		err := tx.QueryRow(ctx, `INSERT INTO seed_flags (name) VALUES ('segments_v1') ON CONFLICT DO NOTHING RETURNING name`).Scan(&applied)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // já aplicado
		}
		if err != nil {
			return err
		}
		return applyExamples(ctx, tx)
	})
}

// ApplyExamples importa os segmentos de exemplo (Clientes novos, Folha de pagamento, Emissão de notas),
// suas palavras-chave e etiquetas comuns, sem sobrescrever o que o gestor já tem: chaves existentes
// são mantidas e nenhuma equipe é vinculada. Pode ser repetido sem efeito colateral.
func (s *Service) ApplyExamples(ctx context.Context, actorID, ip string) error {
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := applyExamples(ctx, tx); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "segment.examples", IP: ip})
	})
}

func applyExamples(ctx context.Context, tx pgx.Tx) error {
	for _, d := range Defaults {
		// Quem já tem um segmento com essa chave ou esse nome mantém o seu, sem alterações.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM segments WHERE key = $1 OR lower(name) = lower($2))`, d.Key, d.Name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		var segID string
		if err := tx.QueryRow(ctx, `INSERT INTO segments (key, name, position)
			VALUES ($1, $2, (SELECT coalesce(max(position), -1) + 1 FROM segments)) RETURNING id`, d.Key, d.Name).Scan(&segID); err != nil {
			return err
		}
		for _, phrase := range d.Keywords {
			if _, err := tx.Exec(ctx, `INSERT INTO segment_keywords (segment_id, phrase, normalized) VALUES ($1, $2, $3)
				ON CONFLICT DO NOTHING`, segID, phrase, Normalize(phrase)); err != nil {
				return err
			}
		}
	}
	for _, t := range DefaultTags {
		if _, err := tx.Exec(ctx, `INSERT INTO tags (name) VALUES ($1) ON CONFLICT DO NOTHING`, t); err != nil {
			return err
		}
	}
	return nil
}
