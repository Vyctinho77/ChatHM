package automation

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Raia da IA (docs/mcp-raia.md). Dentro da raia a IA age livremente: sem horário, sem assuntos e níveis, sem aprovação. As poucas regras que
// sobram são do servidor e valem a cada envio, não dependem da IA:
//
//	entrada: só a conversa da raia, sem atendente humano; foto/documento, dados pessoais, pedido de atendente, reclamação ou rajada de
//	         mensagens do cliente passam para um humano;
//	saída:   sem números/valores soltos (salvo nas raias que precisam), só links da lista, sem termos de pressão, sem pedir senhas;
//	orçamento: no máximo N rodadas por conversa, 2 mensagens seguidas sem resposta do cliente e N por cliente por dia;
//	humano:  quem assume a conversa (ou foi chamado) silencia a IA.
//
// Uma raia nasce desligada e em "sombra": a IA escreve, o servidor guarda o rascunho e nada vai ao cliente.

// Modos da raia.
const (
	LaneShadow = "shadow"
	LaneLive   = "live"
)

// Lane é a configuração de uma raia.
type Lane struct {
	Key               string    `json:"key"`
	Label             string    `json:"label"`
	Enabled           bool      `json:"enabled"`
	Mode              string    `json:"mode"` // shadow | live
	CanInitiate       bool      `json:"canInitiate"`
	SegmentID         string    `json:"segmentId"`
	HandoffSegmentID  string    `json:"handoffSegmentId"`
	MaxTurns          int       `json:"maxTurns"`
	MaxConsecutive    int       `json:"maxConsecutive"`
	ContactDaily      int       `json:"contactDaily"`
	HumanQuietMinutes int       `json:"humanQuietMinutes"`
	AllowDigits       bool      `json:"allowDigits"`
	BlockPressure     bool      `json:"blockPressure"`
	BlockInboundMedia bool      `json:"blockInboundMedia"`
	DiscloseAI        bool      `json:"discloseAi"`
	AllowedDomains    []string  `json:"allowedDomains"`
	Guidelines        string    `json:"guidelines"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// LaneInput: o que o administrador edita (a chave não muda).
type LaneInput struct {
	Label             string   `json:"label"`
	Enabled           bool     `json:"enabled"`
	Mode              string   `json:"mode"`
	CanInitiate       bool     `json:"canInitiate"`
	SegmentID         string   `json:"segmentId"`
	HandoffSegmentID  string   `json:"handoffSegmentId"`
	MaxTurns          int      `json:"maxTurns"`
	MaxConsecutive    int      `json:"maxConsecutive"`
	ContactDaily      int      `json:"contactDaily"`
	HumanQuietMinutes int      `json:"humanQuietMinutes"`
	AllowDigits       bool     `json:"allowDigits"`
	BlockPressure     bool     `json:"blockPressure"`
	BlockInboundMedia bool     `json:"blockInboundMedia"`
	DiscloseAI        bool     `json:"discloseAi"`
	AllowedDomains    []string `json:"allowedDomains"`
	Guidelines        string   `json:"guidelines"`
}

// ErrNoLane: a raia não existe.
var ErrNoLane = errors.New("automation: raia não encontrada")

const laneCols = `key, label, enabled, mode, can_initiate, coalesce(segment_id::text, ''), coalesce(handoff_segment_id::text, ''),
	max_turns, max_consecutive, contact_daily, human_quiet_minutes, allow_digits, block_pressure, block_inbound_media, disclose_ai,
	allowed_domains, guidelines, updated_at`

func scanLane(row pgx.Row) (Lane, error) {
	var l Lane
	err := row.Scan(&l.Key, &l.Label, &l.Enabled, &l.Mode, &l.CanInitiate, &l.SegmentID, &l.HandoffSegmentID, &l.MaxTurns, &l.MaxConsecutive,
		&l.ContactDaily, &l.HumanQuietMinutes, &l.AllowDigits, &l.BlockPressure, &l.BlockInboundMedia, &l.DiscloseAI, &l.AllowedDomains, &l.Guidelines, &l.UpdatedAt)
	if l.AllowedDomains == nil {
		l.AllowedDomains = []string{}
	}
	return l, err
}

// Lane devolve uma raia.
func (s *Service) Lane(ctx context.Context, key string) (Lane, error) {
	l, err := scanLane(s.db.QueryRow(ctx, `SELECT `+laneCols+` FROM ai_lanes WHERE key = $1`, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lane{}, ErrNoLane
	}
	return l, err
}

// Lanes lista as raias.
func (s *Service) Lanes(ctx context.Context) ([]Lane, error) {
	rows, err := s.db.Query(ctx, `SELECT `+laneCols+` FROM ai_lanes ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lane{}
	for rows.Next() {
		l, err := scanLane(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

var reHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// SaveLane (administrador) grava a configuração de uma raia.
func (s *Service) SaveLane(ctx context.Context, actorID, ip, key string, in LaneInput) (Lane, error) {
	if _, err := s.Lane(ctx, key); err != nil {
		return Lane{}, err
	}
	fe := httpx.FieldErrors{}
	in.Label = strings.TrimSpace(in.Label)
	if n := utf8.RuneCountInString(in.Label); n < 1 || n > 60 {
		fe["label"] = "O nome deve ter de 1 a 60 caracteres."
	}
	if in.Mode != LaneShadow && in.Mode != LaneLive {
		fe["mode"] = "Escolha sombra ou valendo."
	}
	check := func(field string, v, min, max int, msg string) {
		if v < min || v > max {
			fe[field] = msg
		}
	}
	check("maxTurns", in.MaxTurns, 1, 50, "De 1 a 50 rodadas.")
	check("maxConsecutive", in.MaxConsecutive, 1, 10, "De 1 a 10 mensagens seguidas.")
	check("contactDaily", in.ContactDaily, 1, 50, "De 1 a 50 mensagens por cliente por dia.")
	check("humanQuietMinutes", in.HumanQuietMinutes, 0, 1440, "De 0 a 1440 minutos.")
	if utf8.RuneCountInString(in.Guidelines) > 6000 {
		fe["guidelines"] = "No máximo 6000 caracteres."
	}
	domains := []string{}
	for _, d := range in.AllowedDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if !reHost.MatchString(d) {
			fe["allowedDomains"] = "Use só o domínio, como asaas.com (sem http:// nem caminho): " + d
			break
		}
		domains = append(domains, d)
	}
	for field, id := range map[string]string{"segmentId": in.SegmentID, "handoffSegmentId": in.HandoffSegmentID} {
		if id != "" && !httpx.ValidUUID(id) {
			fe[field] = "Fila inválida."
		}
	}
	if len(fe) > 0 {
		return Lane{}, fe
	}
	var seg, hand *string
	if in.SegmentID != "" {
		seg = &in.SegmentID
	}
	if in.HandoffSegmentID != "" {
		hand = &in.HandoffSegmentID
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE ai_lanes SET label = $2, enabled = $3, mode = $4, can_initiate = $5, segment_id = $6, handoff_segment_id = $7, max_turns = $8,
			       max_consecutive = $9, contact_daily = $10, human_quiet_minutes = $11, allow_digits = $12, block_pressure = $13,
			       block_inbound_media = $14, disclose_ai = $15, allowed_domains = $16, guidelines = $17, updated_at = now(), updated_by = NULLIF($18, '')::uuid
			 WHERE key = $1`, key, in.Label, in.Enabled, in.Mode, in.CanInitiate, seg, hand, in.MaxTurns, in.MaxConsecutive, in.ContactDaily,
			in.HumanQuietMinutes, in.AllowDigits, in.BlockPressure, in.BlockInboundMedia, in.DiscloseAI, domains, in.Guidelines, actorID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "ai.lane.update", TargetType: "ai_lane", TargetID: key, IP: ip,
			Metadata: map[string]any{"enabled": in.Enabled, "mode": in.Mode, "canInitiate": in.CanInitiate, "maxTurns": in.MaxTurns}})
	})
	if err != nil {
		var pe interface{ SQLState() string }
		if errors.As(err, &pe) && pe.SQLState() == "23503" { // fila que não existe
			return Lane{}, httpx.FieldErrors{"segmentId": "Fila não encontrada."}
		}
		return Lane{}, err
	}
	return s.Lane(ctx, key)
}

// --- fatos da conversa -----------------------------------------------------

type inboundMsg struct {
	Kind string
	Body string
	At   time.Time
}

// laneFacts: o que o servidor sabe da conversa na hora de decidir. A decisão (decideLane) é uma função pura sobre estes fatos.
type laneFacts struct {
	ConvID, ContactID, Status string
	Assignee, SegmentID       string // vazio = nenhum
	Snoozed                   bool
	LastHumanOut              *time.Time // última mensagem enviada por uma pessoa (não pela IA) nesta conversa
	Turns                     int        // mensagens da IA desde a última de uma pessoa
	Consecutive               int        // mensagens da IA desde a última do cliente
	ContactToday              int        // mensagens da IA a este cliente nas últimas 24 h
	OpenHandoff               bool       // passada para um humano e ninguém respondeu ainda
	HasAI                     bool       // a IA já falou nesta conversa
	Inbound                   []inboundMsg
	InboundBurst              int // mensagens do cliente nos últimos 2 minutos
	Now                       time.Time
}

const aiOut = `m.direction = 'out' AND m.deleted_at IS NULL AND m.payload->>'aiMode' = 'attend'`
const humanOut = `m.direction = 'out' AND m.deleted_at IS NULL AND coalesce(m.payload->>'automated', '') <> 'true'`

func (s *Service) laneFacts(ctx context.Context, convID string) (laneFacts, error) {
	f := laneFacts{ConvID: convID, Now: s.now()}
	err := s.db.QueryRow(ctx, `
		SELECT c.contact_id::text, c.status, coalesce(c.assignee_id::text, ''), coalesce(c.segment_id::text, ''),
		       c.snoozed_until IS NOT NULL AND c.snoozed_until > now()
		  FROM conversations c WHERE c.id = $1`, convID).Scan(&f.ContactID, &f.Status, &f.Assignee, &f.SegmentID, &f.Snoozed)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNoConversation
	}
	if err != nil {
		return f, err
	}
	if err := s.db.QueryRow(ctx, `SELECT max(m.sent_at) FROM messages m WHERE m.conversation_id = $1 AND `+humanOut, convID).Scan(&f.LastHumanOut); err != nil {
		return f, err
	}
	err = s.db.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM messages m WHERE m.conversation_id = $1 AND `+aiOut+` AND m.sent_at > coalesce((SELECT max(h.sent_at) FROM messages h WHERE h.conversation_id = $1 AND h.direction = 'out' AND h.deleted_at IS NULL AND coalesce(h.payload->>'automated', '') <> 'true'), '-infinity')),
		  (SELECT count(*) FROM messages m WHERE m.conversation_id = $1 AND `+aiOut+` AND m.sent_at > coalesce((SELECT max(i.sent_at) FROM messages i WHERE i.conversation_id = $1 AND i.direction = 'in' AND i.deleted_at IS NULL), '-infinity')),
		  (SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id WHERE c.contact_id = $2 AND `+aiOut+` AND m.sent_at > $3::timestamptz - interval '24 hours'),
		  EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = $1 AND `+aiOut+`),
		  EXISTS (SELECT 1 FROM ai_handoffs h WHERE h.conversation_id = $1
		           AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = h.conversation_id AND `+humanOut+` AND m.sent_at > h.created_at)),
		  (SELECT count(*) FROM messages m WHERE m.conversation_id = $1 AND m.direction = 'in' AND m.deleted_at IS NULL AND m.sent_at > $3::timestamptz - interval '2 minutes')`,
		convID, f.ContactID, f.Now).Scan(&f.Turns, &f.Consecutive, &f.ContactToday, &f.HasAI, &f.OpenHandoff, &f.InboundBurst)
	if err != nil {
		return f, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT m.kind, m.body, m.sent_at FROM messages m
		 WHERE m.conversation_id = $1 AND m.direction = 'in' AND m.deleted_at IS NULL
		   AND m.sent_at > coalesce((SELECT max(o.sent_at) FROM messages o WHERE o.conversation_id = $1 AND o.direction = 'out' AND o.deleted_at IS NULL), '-infinity')
		 ORDER BY m.sent_at DESC LIMIT 30`, convID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var m inboundMsg
		if err := rows.Scan(&m.Kind, &m.Body, &m.At); err != nil {
			return f, err
		}
		f.Inbound = append(f.Inbound, m)
	}
	return f, rows.Err()
}

// ErrNoConversation: a conversa não existe.
var ErrNoConversation = errors.New("automation: conversa não encontrada")

// --- decisão (pura) --------------------------------------------------------

// laneHandoff: a razão de passar a conversa para um humano. Código curto (aparece na etiqueta e na nota) e um resumo para quem assume.
type laneHandoff struct{ Reason, Summary string }

type laneDecision struct {
	Refusal *Refusal
	Handoff *laneHandoff
	Text    string // o texto a enviar (com a apresentação da IA, se for o caso)
}

const disclosure = "Sou o assistente virtual do escritório."

var (
	reWantsHuman = regexp.MustCompile(`(?i)(?:falar|conversar|chamar|passa(?:r)?|quero|preciso|queria|pode)\b.{0,25}\b(?:atendente|pessoa|humano|gerente|respons[aá]vel|algu[eé]m|funcion[aá]rio)|(?:atendente|humano)\b|reclama|procon|advogad|processo|cancel(?:ar|amento)|golpe|fraude|absurdo|vergonha|n[aã]o (?:quero|aceito|autorizo)|rob[oô]\b|n[aã]o (?:[eé]|sou)\b.{0,12}\b(?:rob[oô]|ia)\b|para de (?:me )?mandar`)
	reSecretIn   = regexp.MustCompile(`(?i)\b(?:senha|password|token|c[oó]digo de (?:verifica|acesso|seguran|confirma)\w*|cvv|cvc|cart[aã]o|chave pix|pix)\b`)
	reSecretOut  = regexp.MustCompile(`(?i)\b(?:senha|password|token|c[oó]digo de (?:verifica|acesso|seguran|confirma)\w*|cvv|cvc|n[uú]mero do cart[aã]o|chave pix)\b`)
	reLong7      = regexp.MustCompile(`\d(?:[\s.\-]?\d){6,}`)
	reRawDoc     = regexp.MustCompile(`\b\d{11}\b|\b\d{14}\b`)
	reMoneyOut   = regexp.MustCompile(`(?i)r\$\s*\d`)
)

// inboundHandoff: o que o cliente mandou obriga a passar para um humano?
func inboundHandoff(l Lane, f laneFacts) *laneHandoff {
	if len(f.Inbound) == 0 {
		return nil
	}
	if f.InboundBurst >= 8 {
		return &laneHandoff{"rajada", "O cliente mandou muitas mensagens em poucos minutos."}
	}
	for _, m := range f.Inbound {
		switch m.Kind {
		case "image", "document":
			if l.BlockInboundMedia {
				return &laneHandoff{"documento", "O cliente enviou foto ou documento. A IA não vê o conteúdo."}
			}
		case "location", "contact":
			return &laneHandoff{"outro_tipo", "O cliente enviou um tipo de mensagem que a IA não trata."}
		}
		if m.Body == "" {
			continue
		}
		if reDoc.MatchString(m.Body) || reRawDoc.MatchString(m.Body) || reDigits.MatchString(m.Body) || reSecretIn.MatchString(m.Body) {
			return &laneHandoff{"dados", "O cliente escreveu dados pessoais, números ou algo sobre senha, código ou cartão."}
		}
		if reWantsHuman.MatchString(m.Body) {
			return &laneHandoff{"pediu_humano", "O cliente pediu um atendente ou fez uma reclamação."}
		}
	}
	return nil
}

// textRefusal: o que a IA quer mandar fere alguma regra da raia?
func textRefusal(l Lane, text string) *Refusal {
	if strings.ContainsAny(text, "\u200b\u202e\ufeff") {
		return refuse("content", "O texto tem caracteres invisíveis.")
	}
	if reEmail.MatchString(text) {
		return refuse("email", "Não escreva e-mails no texto.")
	}
	if reSecretOut.MatchString(text) {
		return refuse("credencial", "Nunca peça nem cite senhas, códigos de verificação, cartão ou chave PIX. Se o cliente precisa informar algo assim, use request_human.")
	}
	if reDoc.MatchString(text) || reRawDoc.MatchString(text) || reUUID.MatchString(text) {
		return refuse("documento", "Não escreva CPF, CNPJ nem identificadores longos no texto.")
	}
	if !l.AllowDigits {
		if reMoneyOut.MatchString(text) || reLong7.MatchString(text) {
			return refuse("digitos", "Nesta raia a IA não escreve valores, documentos nem números longos. Se o assunto exige isso, use request_human para um atendente continuar.")
		}
	}
	for _, link := range reURL.FindAllString(text, -1) {
		if !domainAllowed(link, l.AllowedDomains) {
			return refuse("link", "Link não permitido nesta raia (%s). Só os domínios liberados pelo administrador: %s.", shorten(link), domainList(l.AllowedDomains))
		}
	}
	if l.BlockPressure {
		low := strings.ToLower(text)
		for _, term := range pressureTerms {
			if strings.Contains(low, term) {
				return refuse("pressao", "O texto tem tom de pressão (%q). Escreva com educação e sem ameaça; se o caso é de cobrança séria, use request_human.", term)
			}
		}
	}
	return nil
}

func domainList(d []string) string {
	if len(d) == 0 {
		return "nenhum"
	}
	return strings.Join(d, ", ")
}

// decideLane aplica as regras da raia. Sem efeitos: só devolve o que fazer (recusar, passar para um humano ou enviar este texto).
func decideLane(l Lane, f laneFacts, text string) laneDecision {
	deny := func(code, format string, args ...any) laneDecision {
		return laneDecision{Refusal: refuse(code, format, args...)}
	}
	if !l.Enabled {
		return deny("raia_desligada", "A raia %q está desligada pelo administrador.", l.Label)
	}
	if f.Status == "closed" {
		return deny("encerrada", "A conversa está encerrada.")
	}
	if l.SegmentID != "" && f.SegmentID != l.SegmentID {
		return deny("fora_da_raia", "Esta conversa não é da raia %q.", l.Label)
	}
	if f.OpenHandoff {
		return deny("passada_para_humano", "Esta conversa já foi passada para um atendente. Não escreva: a equipe vai continuar.")
	}
	if f.Assignee != "" {
		return deny("humano_ativo", "Um atendente está com esta conversa. A IA não escreve aqui.")
	}
	if f.LastHumanOut != nil && l.HumanQuietMinutes > 0 && f.Now.Sub(*f.LastHumanOut) < time.Duration(l.HumanQuietMinutes)*time.Minute {
		return deny("humano_ativo", "Um atendente respondeu há pouco nesta conversa. Aguarde antes de escrever.")
	}
	if f.Snoozed {
		return deny("adiada", "A conversa está adiada por um atendente.")
	}
	if h := inboundHandoff(l, f); h != nil {
		return laneDecision{Handoff: h}
	}
	if f.Turns >= l.MaxTurns {
		return laneDecision{Handoff: &laneHandoff{"turnos", "A IA já trocou o máximo de mensagens desta raia nesta conversa."}}
	}
	if f.Consecutive >= l.MaxConsecutive {
		return deny("aguarde_cliente", "A IA já mandou %d mensagem(ns) seguida(s) sem resposta do cliente. Espere ele responder.", f.Consecutive)
	}
	if f.ContactToday >= l.ContactDaily {
		return deny("limite_contato", "Este cliente já recebeu %d mensagens da IA nas últimas 24 horas (limite da raia).", f.ContactToday)
	}
	if r := textRefusal(l, text); r != nil {
		return laneDecision{Refusal: r}
	}
	out := text
	if l.DiscloseAI && !f.HasAI {
		out = disclosure + " " + text
	}
	return laneDecision{Text: out}
}
