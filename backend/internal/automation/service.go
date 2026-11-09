// Package automation: envios automáticos pela IA (docs/mcp-automacao.md, fase 6a).
//
// A IA não é a barreira de segurança: o servidor é. Cada regra abaixo é conferida na hora de enfileirar e de novo na hora de enviar.
package automation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/conversations"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/storage"
)

// Status dos envios.
const (
	StatusPending   = "pending_approval"
	StatusApproved  = "approved"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusRejected  = "rejected"
	StatusSkipped   = "skipped"
)

// activeStatuses: ainda podem sair (ou já saíram) e portanto contam contra repetição e limites por cliente.
const activeStatuses = `('pending_approval','approved','sending','sent')`

type Service struct {
	db    *pgxpool.Pool
	convs *conversations.Service
	store storage.Store
	media *media.Service
	log   *slog.Logger
	now   func() time.Time
	rnd   func(n int) int // sorteio para o intervalo entre envios (trocável nos testes)
}

func NewService(db *pgxpool.Pool, convs *conversations.Service, store storage.Store, m *media.Service, log *slog.Logger) *Service {
	return &Service{db: db, convs: convs, store: store, media: m, log: log, now: time.Now, rnd: rand.IntN}
}

// SetClock troca o relógio (testes).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetRandom troca o sorteio do intervalo (testes). f(n) devolve um número em [0, n).
func (s *Service) SetRandom(f func(n int) int) { s.rnd = f }

// Actor: quem pediu o envio. Para a IA, o token e o provedor (a tela mostra a logo); para uma pessoa pela tela, só UserID.
type Actor struct {
	UserID   string
	TokenID  string
	Provider string // claude | chatgpt | grok | gemini | other
	App      string
}

// --- configurações ---------------------------------------------------------

type Settings struct {
	Paused         bool              `json:"paused"`
	PausedReason   string            `json:"pausedReason"`
	Signature      string            `json:"signature"`
	HourlyCap      int               `json:"hourlyCap"`
	DailyCap       int               `json:"dailyCap"`
	GapMinSeconds  int               `json:"gapMinSeconds"`
	GapMaxSeconds  int               `json:"gapMaxSeconds"`
	DialogHours    int               `json:"dialogHours"`
	ClientDaily    int               `json:"clientDaily"`
	Timezone       string            `json:"timezone"`
	AllowedDomains []string          `json:"allowedDomains"`
	OfficeFields   map[string]string `json:"officeFields"`
}

const selectSettings = `SELECT paused, paused_reason, signature, hourly_cap, daily_cap, gap_min_s, gap_max_s, dialog_hours, client_daily, timezone, allowed_domains, office_fields
	FROM automation_settings WHERE id`

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	var st Settings
	scan := func() error {
		return s.db.QueryRow(ctx, selectSettings).Scan(&st.Paused, &st.PausedReason, &st.Signature, &st.HourlyCap, &st.DailyCap,
			&st.GapMinSeconds, &st.GapMaxSeconds, &st.DialogHours, &st.ClientDaily, &st.Timezone, &st.AllowedDomains, &st.OfficeFields)
	}
	err := scan()
	if errors.Is(err, pgx.ErrNoRows) { // linha única recriada com os padrões se faltar
		if _, ierr := s.db.Exec(ctx, `INSERT INTO automation_settings (id) VALUES (true) ON CONFLICT (id) DO NOTHING`); ierr != nil {
			return st, ierr
		}
		err = scan()
	}
	if st.OfficeFields == nil {
		st.OfficeFields = map[string]string{}
	}
	if st.AllowedDomains == nil {
		st.AllowedDomains = []string{}
	}
	return st, err
}

// SetSettings (administrador).
func (s *Service) SetSettings(ctx context.Context, actorID, ip string, in Settings) (Settings, error) {
	errs := httpx.FieldErrors{}
	sig := strings.Join(strings.Fields(in.Signature), " ")
	if utf8.RuneCountInString(sig) > 60 || strings.ContainsAny(sig, "*_~`") {
		errs["signature"] = "A assinatura pode ter até 60 caracteres, sem * _ ~ `."
	}
	if in.HourlyCap < 1 || in.HourlyCap > 500 {
		errs["hourlyCap"] = "Entre 1 e 500 por hora."
	}
	if in.DailyCap < 1 || in.DailyCap > 3000 {
		errs["dailyCap"] = "Entre 1 e 3000 por dia."
	}
	if in.GapMinSeconds < 1 || in.GapMinSeconds > 300 || in.GapMaxSeconds < in.GapMinSeconds || in.GapMaxSeconds > 600 {
		errs["gapMinSeconds"] = "O intervalo entre envios vai de 1 a 600 segundos, com o máximo não menor que o mínimo."
	}
	if in.DialogHours < 0 || in.DialogHours > 168 {
		errs["dialogHours"] = "Entre 0 e 168 horas."
	}
	if in.ClientDaily < 1 || in.ClientDaily > 10 {
		errs["clientDaily"] = "Entre 1 e 10 por cliente por dia."
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		errs["timezone"] = "Fuso horário inválido (exemplo: America/Manaus)."
	}
	domains, derr := CleanDomains(in.AllowedDomains)
	if derr != nil {
		errs["allowedDomains"] = derr.Error()
	}
	fields, ferr := CleanFields(in.OfficeFields)
	if ferr != nil {
		errs["officeFields"] = ferr.Error()
	}
	if len(errs) > 0 {
		return Settings{}, errs
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE automation_settings SET signature=$1, hourly_cap=$2, daily_cap=$3, gap_min_s=$4, gap_max_s=$5, dialog_hours=$6, client_daily=$7,
			       timezone=$8, allowed_domains=$9, office_fields=$10, updated_at=now(), updated_by=$11 WHERE id`,
			sig, in.HourlyCap, in.DailyCap, in.GapMinSeconds, in.GapMaxSeconds, in.DialogHours, in.ClientDaily, in.Timezone, domains, fields, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.settings.update", TargetType: "automation_settings", IP: ip,
			Metadata: map[string]any{"hourlyCap": in.HourlyCap, "dailyCap": in.DailyCap, "gapMin": in.GapMinSeconds, "gapMax": in.GapMaxSeconds,
				"dialogHours": in.DialogHours, "clientDaily": in.ClientDaily, "domains": domains, "fieldKeys": keys(fields)}})
	})
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx)
}

// SetPaused é o "Pausar tudo": efeito imediato (o worker confere a cada item).
func (s *Service) SetPaused(ctx context.Context, actorID, ip string, paused bool, reason string) error {
	if reason = strings.TrimSpace(reason); utf8.RuneCountInString(reason) > 200 {
		reason = string([]rune(reason)[:200])
	}
	if !paused {
		reason = ""
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE automation_settings SET paused = $1, paused_reason = $2, updated_at = now() WHERE id`, paused, reason); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.pause", TargetType: "automation_settings", IP: ip,
			Metadata: map[string]any{"paused": paused, "reason": reason}})
	})
}

// autoPause: parada automática (muitas falhas seguidas). Sem autor humano.
func (s *Service) autoPause(ctx context.Context, reason string) {
	_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE automation_settings SET paused = true, paused_reason = $1, updated_at = now() WHERE id AND NOT paused`, reason); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "automation.pause", TargetType: "automation_settings", Metadata: map[string]any{"paused": true, "reason": reason, "auto": true}})
	})
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- categorias ------------------------------------------------------------

type Category struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Enabled         bool   `json:"enabled"`
	Level           int    `json:"level"`
	WindowStart     string `json:"windowStart"` // HH:MM
	WindowEnd       string `json:"windowEnd"`
	Weekdays        []int  `json:"weekdays"` // 1 = segunda ... 7 = domingo
	MinIntervalDays int    `json:"minIntervalDays"`
	Guidelines      string `json:"guidelines"`
}

const selectCategory = `SELECT key, label, enabled, level, to_char(window_start, 'HH24:MI'), to_char(window_end, 'HH24:MI'), weekdays, min_interval_days, guidelines FROM automation_categories`

func scanCategory(row pgx.Row) (Category, error) {
	var c Category
	var wd []int16
	var level int16
	err := row.Scan(&c.Key, &c.Label, &c.Enabled, &level, &c.WindowStart, &c.WindowEnd, &wd, &c.MinIntervalDays, &c.Guidelines)
	c.Level = int(level)
	c.Weekdays = make([]int, len(wd))
	for i, d := range wd {
		c.Weekdays[i] = int(d)
	}
	return c, err
}

func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	rows, err := s.db.Query(ctx, selectCategory+` ORDER BY sort, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) category(ctx context.Context, key string) (Category, error) {
	c, err := scanCategory(s.db.QueryRow(ctx, selectCategory+` WHERE key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

const (
	LevelApproval  = 1
	LevelAutomatic = 3
)

// UpdateCategory: o administrador escolhe aprovação ou envio autônomo sob as regras do servidor.
func (s *Service) UpdateCategory(ctx context.Context, actorID, ip, key string, in Category) (Category, error) {
	errs := httpx.FieldErrors{}
	if in.Level != LevelApproval && in.Level != LevelAutomatic {
		errs["level"] = "Escolha aprovação (1) ou automático sem aprovação (3)."
	}
	if !hhmm.MatchString(in.WindowStart) || !hhmm.MatchString(in.WindowEnd) || in.WindowStart >= in.WindowEnd {
		errs["windowStart"] = "Informe a janela como HH:MM, com o fim depois do início."
	}
	wd := make([]int16, 0, 7)
	seen := map[int]bool{}
	for _, d := range in.Weekdays {
		if d < 1 || d > 7 {
			errs["weekdays"] = "Dias da semana de 1 (segunda) a 7 (domingo)."
		} else if !seen[d] {
			seen[d] = true
			wd = append(wd, int16(d))
		}
	}
	if len(wd) == 0 {
		errs["weekdays"] = "Escolha ao menos um dia da semana."
	}
	if in.MinIntervalDays < 0 || in.MinIntervalDays > 365 {
		errs["minIntervalDays"] = "Entre 0 e 365 dias."
	}
	if utf8.RuneCountInString(in.Guidelines) > 2000 {
		errs["guidelines"] = "As diretrizes podem ter até 2000 caracteres."
	}
	if len(errs) > 0 {
		return Category{}, errs
	}
	var out Category
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE automation_categories SET enabled=$2, level=$8, window_start=$3::time, window_end=$4::time, weekdays=$5, min_interval_days=$6, guidelines=$7 WHERE key=$1`,
			key, in.Enabled, in.WindowStart, in.WindowEnd, wd, in.MinIntervalDays, strings.TrimSpace(in.Guidelines), in.Level)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.category.update", TargetType: "automation_category", TargetID: key, IP: ip,
			Metadata: map[string]any{"enabled": in.Enabled, "level": in.Level, "window": in.WindowStart + "-" + in.WindowEnd, "weekdays": in.Weekdays, "minIntervalDays": in.MinIntervalDays}})
	})
	if err != nil {
		return out, err
	}
	return s.category(ctx, key)
}

// --- envios ----------------------------------------------------------------

var (
	ErrNotFound = errors.New("automation: não encontrado")
	ErrState    = errors.New("automation: o envio não está num estado que permita isto")
)

type Send struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"createdAt"`
	Category      string     `json:"category"`
	CategoryLabel string     `json:"categoryLabel"`
	Reason        string     `json:"reason"`
	Text          string     `json:"text"`
	ContactID     string     `json:"contactId"`
	ContactName   string     `json:"contactName"`
	DocumentID    string     `json:"documentId,omitempty"`
	DocumentName  string     `json:"documentName,omitempty"`
	Status        string     `json:"status"`
	StatusDetail  string     `json:"statusDetail,omitempty"`
	Flags         []string   `json:"flags"`
	Provider      string     `json:"provider"`
	App           string     `json:"app"`
	Edited        bool       `json:"edited"`
	ScheduledFor  *time.Time `json:"scheduledFor,omitempty"`
	SentAt        *time.Time `json:"sentAt,omitempty"`
	MessageID     string     `json:"messageId,omitempty"`
	// Preview: o texto como o cliente receberá (variáveis substituídas, assinatura e rodapé).
	Preview string `json:"preview"`
}

type EnqueueInput struct {
	ContactID  string
	Category   string
	Reason     string
	Text       string
	DocumentID string
	DedupeKey  string
}

// masterOn: IA ligada e envios automáticos liberados pelo administrador.
func (s *Service) masterOn(ctx context.Context) (bool, error) {
	var on bool
	err := s.db.QueryRow(ctx, `SELECT enabled AND allow_write AND allow_send AND allow_automation FROM ai_settings WHERE id`).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return on, err
}

type contactInfo struct {
	Name string
}

// guardContact: as regras sobre o destinatário e a conversa. Conferidas ao enfileirar e de novo ao enviar.
func (s *Service) guardContact(ctx context.Context, contactID string, st Settings) (contactInfo, *Refusal, error) {
	var info contactInfo
	var kind, waID string
	var optOut *time.Time
	if !httpx.ValidUUID(contactID) {
		return info, refuse("contact_not_found", "contact_id inválido: use o id de um contato do ChatHM (list_contacts)."), nil
	}
	err := s.db.QueryRow(ctx, `SELECT name, kind, wa_id, automation_opt_out_at FROM contacts WHERE id = $1`, contactID).Scan(&info.Name, &kind, &waID, &optOut)
	if errors.Is(err, pgx.ErrNoRows) {
		return info, refuse("contact_not_found", "Esse contato não existe no ChatHM. Só é possível enviar a contatos já cadastrados."), nil
	}
	if err != nil {
		return info, nil, err
	}
	if kind == "group" {
		return info, refuse("group", "Grupos não recebem mensagens automáticas."), nil
	}
	if optOut != nil {
		return info, refuse("opt_out", "Este cliente pediu para não receber avisos automáticos (PARAR)."), nil
	}
	var snoozed bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE contact_id = $1 AND status <> 'closed' AND snoozed_until IS NOT NULL)`, contactID).Scan(&snoozed); err != nil {
		return info, nil, err
	}
	if snoozed {
		return info, refuse("snoozed", "A conversa deste cliente está adiada por alguém da equipe."), nil
	}
	var assigned bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE contact_id = $1 AND status <> 'closed' AND assignee_id IS NOT NULL)`, contactID).Scan(&assigned); err != nil {
		return info, nil, err
	}
	if assigned {
		return info, refuse("assigned", "Este cliente está em atendimento pela equipe. A IA não entra em conversas de uma pessoa."), nil
	}
	if st.DialogHours > 0 {
		var last *time.Time
		if err := s.db.QueryRow(ctx, `
			SELECT max(m.sent_at) FROM messages m JOIN conversations c ON c.id = m.conversation_id
			 WHERE c.contact_id = $1 AND coalesce(m.payload->>'automated', '') <> 'true' AND coalesce(m.payload->>'historical', '') <> 'true'`, contactID).Scan(&last); err != nil {
			return info, nil, err
		}
		if last != nil && s.now().Sub(*last) < time.Duration(st.DialogHours)*time.Hour {
			return info, refuse("dialog_active", "Há conversa recente com este cliente (menos de %d h). Mensagens automáticas só iniciam assunto, nunca entram num diálogo: deixe para uma pessoa.", st.DialogHours), nil
		}
	}
	return info, nil, nil
}

// Enqueue persiste o pedido e devolve imediatamente. O worker envia sem depender de sessão no navegador.
func (s *Service) Enqueue(ctx context.Context, a Actor, in EnqueueInput) (Send, error) {
	on, err := s.masterOn(ctx)
	if err != nil {
		return Send{}, err
	}
	if !on {
		return Send{}, refuse("automation_off", "Os envios automáticos estão desligados pelo administrador.")
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return Send{}, err
	}
	if st.Paused {
		msg := "Os envios automáticos estão pausados pelo administrador."
		if st.PausedReason != "" {
			msg += " Motivo: " + st.PausedReason
		}
		return Send{}, refuse("paused", "%s", msg)
	}
	cat, err := s.category(ctx, in.Category)
	if errors.Is(err, ErrNotFound) {
		return Send{}, refuse("category_off", "Categoria desconhecida %q. Use automation_status para ver as liberadas.", in.Category)
	}
	if err != nil {
		return Send{}, err
	}
	if !cat.Enabled {
		return Send{}, refuse("category_off", "A categoria %q não está liberada pelo administrador.", cat.Key)
	}
	reason := strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 300 {
		return Send{}, refuse("reason", "Informe o motivo concreto do envio (5 a 300 caracteres), por exemplo: DAS 10/2026 vence em 20/11.")
	}
	key := strings.TrimSpace(in.DedupeKey)
	if !dedupeKey.MatchString(key) {
		return Send{}, refuse("dedupe_key", "dedupe_key deve ter de 1 a 80 caracteres (letras, números e - _ . : /). Use algo como das-2026-10.")
	}
	flags, ref := ValidateText(in.Text, st.AllowedDomains, st.OfficeFields)
	if ref != nil {
		return Send{}, ref
	}
	info, ref, err := s.guardContact(ctx, in.ContactID, st)
	if err != nil {
		return Send{}, err
	}
	if ref != nil {
		return Send{}, ref
	}
	var docName string
	if in.DocumentID != "" {
		d, ref, err := s.guardDocument(ctx, in.DocumentID, in.ContactID)
		if err != nil {
			return Send{}, err
		}
		if ref != nil {
			return Send{}, ref
		}
		docName = d.Name
	}
	var out Send
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		// Serializa com mudanças de autonomia feitas pelo administrador.
		var level int
		var enabled bool
		if err := tx.QueryRow(ctx, `SELECT level, enabled FROM automation_categories WHERE key=$1 FOR SHARE`, cat.Key).Scan(&level, &enabled); err != nil {
			return err
		}
		if !enabled {
			return refuse("category_off", "O assunto foi desligado pelo administrador.")
		}
		// Serializa pedidos do mesmo cliente antes de conferir os limites.
		if _, err := tx.Exec(ctx, `SELECT id FROM contacts WHERE id=$1 FOR UPDATE`, in.ContactID); err != nil {
			return err
		}
		if ref, err := s.clientLimits(ctx, tx, in.ContactID, cat, st, ""); err != nil {
			return err
		} else if ref != nil {
			return ref
		}
		status := StatusPending
		var scheduled *time.Time
		if level == LevelAutomatic && len(flags) == 0 {
			status = StatusApproved
			now := s.now()
			scheduled = &now
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO automation_sends (token_id, created_by, provider, app_name, category, reason, text, document_id, contact_id, dedupe_key, flags, status, scheduled_for)
			VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid, $9, $10, $11, $12, $13) RETURNING id::text`,
			a.TokenID, a.UserID, a.Provider, a.App, cat.Key, reason, strings.TrimSpace(in.Text), in.DocumentID, in.ContactID, key, nonNil(flags), status, scheduled).Scan(&id); err != nil {
			return err
		}
		out.ID = id
		return audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "automation.enqueue", TargetType: "automation_send", TargetID: id,
			Metadata: map[string]any{"category": cat.Key, "contactId": in.ContactID, "provider": a.Provider, "flags": flags, "hasDocument": in.DocumentID != "", "dedupeKey": key, "level": level, "status": status}})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "automation_sends_dedupe" {
		r := refuse("duplicate", "Já existe um envio com esta dedupe_key para este cliente e assunto. Não repita.")
		_ = s.db.QueryRow(ctx, `SELECT id::text FROM automation_sends WHERE contact_id=$1 AND category=$2 AND dedupe_key=$3 AND status IN `+activeStatuses, in.ContactID, cat.Key, key).Scan(&r.ExistingID)
		return Send{}, r
	}
	if err != nil {
		return Send{}, err
	}
	_ = info
	_ = docName
	return s.Get(ctx, out.ID)
}

func (s *Service) clientLimits(ctx context.Context, db database.DBTX, contactID string, cat Category, st Settings, excludeID string) (*Refusal, error) {
	var today, inInterval int
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE created_at > now() - interval '24 hours'),
		count(*) FILTER (WHERE category=$2 AND created_at > now() - make_interval(days => $3))
		FROM automation_sends WHERE contact_id=$1 AND id::text<>$4 AND status IN `+activeStatuses,
		contactID, cat.Key, cat.MinIntervalDays, excludeID).Scan(&today, &inInterval); err != nil {
		return nil, err
	}
	if today >= st.ClientDaily {
		return refuse("client_daily", "Este cliente atingiu o limite de %d aviso(s) em 24 horas.", st.ClientDaily), nil
	}
	if cat.MinIntervalDays > 0 && inInterval > 0 {
		return refuse("min_interval", "Este cliente já tem um aviso de %q nos últimos %d dias.", cat.Label, cat.MinIntervalDays), nil
	}
	return nil, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

const selectSend = `
	SELECT s.id::text, s.created_at, s.category, coalesce(c.label, s.category), s.reason, s.text, s.contact_id::text, coalesce(nullif(ct.name, ''), ct.phone, ''),
	       coalesce(s.document_id::text, ''), coalesce(d.name, ''), s.status, s.status_detail, s.flags, s.provider, s.app_name, s.edited,
	       s.scheduled_for, s.sent_at, coalesce(s.message_id::text, '')
	  FROM automation_sends s
	  JOIN contacts ct ON ct.id = s.contact_id
	  LEFT JOIN automation_categories c ON c.key = s.category
	  LEFT JOIN automation_documents d ON d.id = s.document_id`

func scanSend(row pgx.Row) (Send, error) {
	var x Send
	err := row.Scan(&x.ID, &x.CreatedAt, &x.Category, &x.CategoryLabel, &x.Reason, &x.Text, &x.ContactID, &x.ContactName, &x.DocumentID, &x.DocumentName,
		&x.Status, &x.StatusDetail, &x.Flags, &x.Provider, &x.App, &x.Edited, &x.ScheduledFor, &x.SentAt, &x.MessageID)
	if x.Flags == nil {
		x.Flags = []string{}
	}
	return x, err
}

// withPreview acrescenta o texto final como o cliente receberá.
func (s *Service) withPreview(ctx context.Context, list []Send) []Send {
	st, err := s.Settings(ctx)
	if err != nil {
		return list
	}
	for i := range list {
		list[i].Preview = "*" + Author(st.Signature) + ":*\n" + Render(list[i].Text, list[i].ContactName, st.OfficeFields)
	}
	return list
}

func (s *Service) Get(ctx context.Context, id string) (Send, error) {
	if !httpx.ValidUUID(id) {
		return Send{}, ErrNotFound
	}
	x, err := scanSend(s.db.QueryRow(ctx, selectSend+` WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Send{}, ErrNotFound
	}
	if err != nil {
		return Send{}, err
	}
	return s.withPreview(ctx, []Send{x})[0], nil
}

type ListFilter struct {
	Status    string // vazio = todos; "open" = pendente + aprovado + enviando
	ContactID string
	CreatedBy string // só os envios pedidos por esta pessoa (visão da IA)
	Limit     int
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Send, error) {
	if f.Limit < 1 || f.Limit > 200 {
		f.Limit = 50
	}
	q := selectSend + ` WHERE ($1 = '' OR ($1 = 'open' AND s.status IN ('pending_approval','approved','sending')) OR s.status = $1)
	       AND ($2 = '' OR s.contact_id::text = $2) AND ($3 = '' OR s.created_by::text = $3)
	 ORDER BY CASE WHEN s.status = 'pending_approval' THEN 0 WHEN s.status IN ('approved','sending') THEN 1 ELSE 2 END, s.created_at DESC LIMIT $4`
	rows, err := s.db.Query(ctx, q, f.Status, f.ContactID, f.CreatedBy, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Send{}
	for rows.Next() {
		x, err := scanSend(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return s.withPreview(ctx, out), rows.Err()
}

// Approve (administrador): confere TUDO de novo (as regras podem ter mudado desde que a IA enfileirou) e libera. edits troca o texto de um envio.
func (s *Service) Approve(ctx context.Context, actorID, ip string, ids []string, edits map[string]string) (approved []string, refused map[string]string, err error) {
	if len(ids) == 0 || len(ids) > 200 {
		return nil, nil, httpx.FieldErrors{"ids": "Escolha de 1 a 200 envios."}
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, nil, err
	}
	refused = map[string]string{}
	for _, id := range ids {
		x, gerr := s.Get(ctx, id)
		if gerr != nil || x.Status != StatusPending {
			refused[id] = "não está aguardando aprovação"
			continue
		}
		text, edited := x.Text, false
		if e, ok := edits[id]; ok && strings.TrimSpace(e) != strings.TrimSpace(x.Text) {
			text, edited = strings.TrimSpace(e), true
		}
		flags, ref := ValidateText(text, st.AllowedDomains, st.OfficeFields)
		if ref != nil {
			refused[id] = ref.Message
			continue
		}
		if _, ref, gerr := s.guardContact(ctx, x.ContactID, st); gerr != nil {
			return approved, refused, gerr
		} else if ref != nil {
			refused[id] = ref.Message
			continue
		}
		if x.DocumentID != "" {
			if _, ref, gerr := s.guardDocument(ctx, x.DocumentID, x.ContactID); gerr != nil {
				return approved, refused, gerr
			} else if ref != nil {
				refused[id] = ref.Message
				continue
			}
		}
		now := s.now()
		tag, uerr := s.db.Exec(ctx, `
			UPDATE automation_sends SET status = 'approved', approved_by = $2, approved_at = $3, scheduled_for = $3, text = $4, edited = edited OR $5, flags = $6, updated_at = $3
			 WHERE id = $1 AND status = 'pending_approval'`, id, actorID, now, text, edited, nonNil(flags))
		if uerr != nil {
			return approved, refused, uerr
		}
		if tag.RowsAffected() == 0 {
			refused[id] = "outra pessoa já decidiu"
			continue
		}
		_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.approve", TargetType: "automation_send", TargetID: id, IP: ip,
				Metadata: map[string]any{"edited": edited, "contactId": x.ContactID, "category": x.Category}})
		})
		approved = append(approved, id)
	}
	return approved, refused, nil
}

// Reject (administrador): descarta antes de sair.
func (s *Service) Reject(ctx context.Context, actorID, ip string, ids []string, reason string) (int, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return 0, httpx.FieldErrors{"ids": "Escolha de 1 a 200 envios."}
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 200 {
		reason = string([]rune(reason)[:200])
	}
	n := 0
	for _, id := range ids {
		if !httpx.ValidUUID(id) {
			continue
		}
		tag, err := s.db.Exec(ctx, `UPDATE automation_sends SET status = 'rejected', status_detail = $2, updated_at = now() WHERE id = $1 AND status = 'pending_approval'`, id, reason)
		if err != nil {
			return n, err
		}
		if tag.RowsAffected() > 0 {
			n++
			_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
				return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.reject", TargetType: "automation_send", TargetID: id, IP: ip, Metadata: map[string]any{"reason": reason}})
			})
		}
	}
	return n, nil
}

// Cancel cancela o que ainda não saiu. onlyOwner (a IA): só os envios pedidos por esta pessoa.
func (s *Service) Cancel(ctx context.Context, actorID, ip string, ids []string, onlyOwner bool) (int, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return 0, httpx.FieldErrors{"ids": "Escolha de 1 a 200 envios."}
	}
	n := 0
	for _, id := range ids {
		if !httpx.ValidUUID(id) {
			continue
		}
		owner := ""
		if onlyOwner {
			owner = actorID
		}
		tag, err := s.db.Exec(ctx, `
			UPDATE automation_sends SET status = 'cancelled', status_detail = 'Cancelado.', updated_at = now()
			 WHERE id = $1 AND status IN ('pending_approval', 'approved') AND ($2 = '' OR created_by::text = $2)`, id, owner)
		if err != nil {
			return n, err
		}
		if tag.RowsAffected() > 0 {
			n++
			_ = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
				return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.cancel", TargetType: "automation_send", TargetID: id, IP: ip})
			})
		}
	}
	return n, nil
}

// Retry (administrador): um envio que falhou volta para a fila já aprovado.
func (s *Service) Retry(ctx context.Context, actorID, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	tag, err := s.db.Exec(ctx, `UPDATE automation_sends SET status = 'approved', status_detail = '', scheduled_for = now(), updated_at = now() WHERE id = $1 AND status = 'failed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrState
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.retry", TargetType: "automation_send", TargetID: id, IP: ip})
	})
}

// --- visão geral -----------------------------------------------------------

type Overview struct {
	Pending       int             `json:"pending"`
	Queued        int             `json:"queued"` // aprovados esperando a vez
	SentToday     int             `json:"sentToday"`
	SentLastHour  int             `json:"sentLastHour"`
	Failed        int             `json:"failed"`
	HourlyCap     int             `json:"hourlyCap"`
	DailyCap      int             `json:"dailyCap"`
	Paused        bool            `json:"paused"`
	PausedReason  string          `json:"pausedReason"`
	NextAllowedAt *time.Time      `json:"nextAllowedAt,omitempty"`
	WhatsAppReady bool            `json:"whatsappReady"`
	MasterOn      bool            `json:"masterOn"`
	WindowOpenNow map[string]bool `json:"windowOpenNow"`
}

func (s *Service) Overview(ctx context.Context, whatsappReady bool) (Overview, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{HourlyCap: st.HourlyCap, DailyCap: st.DailyCap, Paused: st.Paused, PausedReason: st.PausedReason, WhatsAppReady: whatsappReady, WindowOpenNow: map[string]bool{}}
	if o.MasterOn, err = s.masterOn(ctx); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending_approval'), count(*) FILTER (WHERE status IN ('approved', 'sending')),
		       count(*) FILTER (WHERE status = 'sent' AND sent_at > now() - interval '24 hours'),
		       count(*) FILTER (WHERE status = 'sent' AND sent_at > now() - interval '1 hour'),
		       count(*) FILTER (WHERE status = 'failed' AND updated_at > now() - interval '7 days')
		  FROM automation_sends`).Scan(&o.Pending, &o.Queued, &o.SentToday, &o.SentLastHour, &o.Failed); err != nil {
		return o, err
	}
	var next time.Time
	if err := s.db.QueryRow(ctx, `SELECT next_allowed_at FROM automation_state WHERE id`).Scan(&next); err == nil && next.After(s.now()) {
		o.NextAllowedAt = &next
	}
	cats, err := s.Categories(ctx)
	if err != nil {
		return o, err
	}
	for _, c := range cats {
		o.WindowOpenNow[c.Key] = c.Enabled && s.inWindow(c, st, s.now())
	}
	return o, nil
}

// inWindow: dentro do dia da semana e da faixa de horário da categoria, no fuso do escritório.
func (s *Service) inWindow(c Category, st Settings, now time.Time) bool {
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.UTC
	}
	l := now.In(loc)
	wd := int(l.Weekday()) // domingo = 0
	if wd == 0 {
		wd = 7
	}
	ok := false
	for _, d := range c.Weekdays {
		if d == wd {
			ok = true
		}
	}
	if !ok {
		return false
	}
	hm := fmt.Sprintf("%02d:%02d", l.Hour(), l.Minute())
	return hm >= c.WindowStart && hm < c.WindowEnd
}

// --- opt-out manual (equipe) -----------------------------------------------

type ContactState struct {
	OptOut   bool       `json:"optOut"`
	OptOutAt *time.Time `json:"optOutAt,omitempty"`
}

func (s *Service) ContactState(ctx context.Context, contactID string) (ContactState, error) {
	var out ContactState
	if !httpx.ValidUUID(contactID) {
		return out, ErrNotFound
	}
	err := s.db.QueryRow(ctx, `SELECT automation_opt_out_at FROM contacts WHERE id = $1`, contactID).Scan(&out.OptOutAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	out.OptOut = out.OptOutAt != nil
	return out, err
}

// SetOptOut: a equipe marca (ou desmarca) um cliente como "não receber avisos automáticos". Marcar cancela o que está na fila para ele.
func (s *Service) SetOptOut(ctx context.Context, actorID, ip, contactID string, optOut bool) error {
	if !httpx.ValidUUID(contactID) {
		return ErrNotFound
	}
	return database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE contacts SET automation_opt_out_at = CASE WHEN $2 THEN coalesce(automation_opt_out_at, now()) ELSE NULL END WHERE id = $1`, contactID, optOut)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if optOut {
			if _, err := tx.Exec(ctx, `UPDATE automation_sends SET status = 'cancelled', status_detail = 'O cliente não recebe avisos automáticos.', updated_at = now()
				 WHERE contact_id = $1 AND status IN ('pending_approval', 'approved')`, contactID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "contact.automation_opt_out", TargetType: "contact", TargetID: contactID, IP: ip, Metadata: map[string]any{"optOut": optOut, "by": "staff"}})
	})
}
