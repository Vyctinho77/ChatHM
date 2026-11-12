// Package conversations gerencia contatos, conversas e mensagens.
//
// Regras de negócio (blueprint, Fase 3):
//
//   - a mensagem do cliente é classificada por palavras-chave (pacote segments);
//     sem correspondência clara ou com conflito, a conversa fica na fila geral;
//   - conversa classificada chega à fila da equipe do segmento; todos os integrantes
//     enxergam, e o primeiro que responder assume o atendimento de forma atômica;
//   - conversa em andamento mantém responsável e fila (nada é redistribuído por
//     palavras soltas); após o encerramento, uma nova demanda abre nova conversa
//     e é classificada de novo.
//
// Visibilidade (ver visibleExpr): admin vê tudo; o responsável vê a própria conversa;
// sem responsável, a fila geral/não roteada é visível a todos e a fila de uma equipe só
// aos integrantes; depois de assumida, a conversa de uma equipe continua visível à
// equipe, e a da fila geral passa a ser visível só ao responsável (e ao admin).
package conversations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
	"github.com/vyctinho77/chathm/backend/internal/segments"
	"github.com/vyctinho77/chathm/backend/internal/storage"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

var (
	ErrNotFound    = errors.New("conversations: não encontrada")
	ErrClosed      = errors.New("conversations: atendimento encerrado")
	ErrForbidden   = errors.New("conversations: sem permissão")
	ErrUnavailable = errors.New("conversations: WhatsApp indisponível")
	ErrIDConflict  = errors.New("conversations: identificador de mensagem em uso")
	ErrTagNotFound = errors.New("conversations: etiqueta não encontrada")
)

// TakenError indica que outra pessoa já assumiu o atendimento.
type TakenError struct{ Assignee string }

func (e *TakenError) Error() string {
	return "conversations: atendimento já assumido por " + e.Assignee
}

// Publisher é satisfeito por *realtime.Hub.
type Publisher interface {
	Publish(typ string, data any, filter realtime.Filter)
}

// InboundEvent: uma mensagem de cliente acabou de ser gravada.
type InboundEvent struct {
	ConversationID string
	MessageID      string
	Group          bool
}

// InboundNotifier recebe o aviso de cada mensagem nova de cliente (satisfeito pelo serviço de acesso por IA, para o webhook).
// Quem implementa não pode devolver erro nem demorar: o recebimento de mensagens não espera por ele.
type InboundNotifier interface {
	OnInbound(ctx context.Context, ev InboundEvent)
}

// SetInboundNotifier liga o aviso de mensagens novas.
func (s *Service) SetInboundNotifier(n InboundNotifier) { s.inbound = n }

// Sender envia mensagens pelo WhatsApp (satisfeito por *whatsapp.Manager).
type Sender interface {
	Connected() bool
	NewMessageID() string
	SendText(ctx context.Context, chatJID, messageID, text string) error
	SendMedia(ctx context.Context, chatJID, messageID string, m whatsapp.MediaSend) error
	SendLocation(ctx context.Context, chatJID, messageID string, l whatsapp.Location) error
	SendTextQuoted(ctx context.Context, chatJID, messageID, text string, q whatsapp.Quote) error
	EditText(ctx context.Context, chatJID, targetID, text string) error
	Revoke(ctx context.Context, chatJID, targetID string) error
	ProfilePicture(ctx context.Context, jid string, preview bool) ([]byte, error)
}

// Viewer é quem faz a requisição; toda consulta filtra pela visibilidade dele.
type Viewer struct {
	ID    string
	Name  string
	Admin bool
	// Supervisor (gestor): age nas conversas de outras pessoas que o viewer já pode ver. Não amplia a visibilidade.
	Supervisor bool
}

// CanSupervise: administrador ou gestor.
func (v Viewer) CanSupervise() bool { return v.Admin || v.Supervisor }

type Service struct {
	store      storage.Store
	media      *media.Service
	mediaSlots chan struct{}
	// PDFs do histórico têm fila própria (poucos de cada vez) para nunca atrasar a mídia das conversas ao vivo.
	histSlots  chan struct{}
	histBudget int64 // bytes máximos de anexos antes de parar de baixar PDFs do histórico (<= 0 desliga)
	db         *pgxpool.Pool
	pub        Publisher
	sender     Sender
	inbound    InboundNotifier
}

func NewService(db *pgxpool.Pool, pub Publisher) *Service {
	return &Service{db: db, pub: pub, mediaSlots: make(chan struct{}, 4), histSlots: make(chan struct{}, 2)}
}

// SetHistoryMediaBudget define o espaço total de anexos (bytes) até o qual PDFs do histórico são baixados; <= 0 desliga.
func (s *Service) SetHistoryMediaBudget(bytes int64) { s.histBudget = bytes }

// SetSender liga o envio (depois da criação, pois o Manager também depende deste serviço).
func (s *Service) SetSender(sd Sender) { s.sender = sd }

// SetMedia liga o armazenamento privado e o processamento de anexos.
func (s *Service) SetMedia(st storage.Store, m *media.Service) { s.store, s.media = st, m }

// Eventos em tempo real. Só chegam a quem pode ver a conversa.
const (
	EventConversationUpdated = "conversation.updated"
	EventConversationRemoved = "conversation.removed" // saiu da visão (ex.: outra pessoa assumiu)
	EventMessageCreated      = "message.created"
	EventMessageUpdated      = "message.updated" // recibos e falhas de envio
)

// visibleExpr gera a regra de visibilidade como expressão SQL. Espera os aliases
// c (conversations) e s (segments, LEFT JOIN). user e admin são expressões SQL.
func visibleExpr(user, admin string) string {
	return fmt.Sprintf(`(%[2]s
		OR c.assignee_id = %[1]s
		OR EXISTS (SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = c.id AND cp.user_id = %[1]s)
		OR (s.team_id IS NULL AND c.assignee_id IS NULL)
		OR (s.team_id IS NOT NULL AND EXISTS (
		      SELECT 1 FROM team_members tm WHERE tm.team_id = s.team_id AND tm.user_id = %[1]s)))`, user, admin)
}

// forViewer: condição para o viewer, com $1 = id e $2 = admin.
var forViewer = visibleExpr("$1::uuid", "$2::boolean")

// ---------------------------------------------------------------------------
// Visibilidade e eventos

type userSet map[string]struct{}

func (u userSet) filter() realtime.Filter {
	return func(c *realtime.Client) bool { _, ok := u[c.UserID]; return ok }
}

// visibleUsers devolve quem pode ver a conversa no estado atual do banco.
func (s *Service) visibleUsers(ctx context.Context, db database.DBTX, convID string) (userSet, error) {
	rows, err := db.Query(ctx, `
		SELECT u.id FROM users u
		  JOIN conversations c ON c.id = $1
		  LEFT JOIN segments s ON s.id = c.segment_id
		 WHERE u.is_active AND `+visibleExpr("u.id", "(u.role = 'admin')"), convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := userSet{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = struct{}{}
	}
	return set, rows.Err()
}

// publishState envia o estado atual da conversa a quem pode vê-la agora e avisa
// quem deixou de poder vê-la (before − after) para remover da lista.
func (s *Service) publishState(ctx context.Context, convID string, before userSet) userSet {
	if s.pub == nil {
		return nil
	}
	after, err := s.visibleUsers(ctx, s.db, convID)
	if err != nil {
		return nil
	}
	if c, err := s.fetch(ctx, s.db, convID); err == nil {
		if c.Status == "closed" {
			// Encerrada sai da lista de todos que a viam.
			s.pub.Publish(EventConversationRemoved, map[string]string{"id": convID}, after.filter())
		} else {
			s.pub.Publish(EventConversationUpdated, c, after.filter())
		}
	}
	if len(before) > 0 {
		lost := userSet{}
		for id := range before {
			if _, ok := after[id]; !ok {
				lost[id] = struct{}{}
			}
		}
		if len(lost) > 0 {
			s.pub.Publish(EventConversationRemoved, map[string]string{"id": convID}, lost.filter())
		}
	}
	return after
}

func (s *Service) publishMessage(ctx context.Context, typ, msgID string, to userSet) {
	if s.pub == nil || len(to) == 0 {
		return
	}
	if m, err := s.message(ctx, msgID); err == nil {
		s.pub.Publish(typ, m, to.filter())
	}
}

// ---------------------------------------------------------------------------
// Ingestão (mensagens recebidas) + classificação

// Ingest grava uma mensagem vinda do provedor e classifica a conversa quando ainda
// não tem fila. É idempotente: o mesmo (chat, ID externo) nunca gera duas mensagens
// nem conta não lida duas vezes. Retorna created=false para duplicatas.
func (s *Service) Ingest(ctx context.Context, m whatsapp.InboundMessage) (bool, error) {
	if m.ExternalID == "" || m.ChatJID == "" || m.ContactWAID == "" {
		return false, errors.New("conversations: mensagem sem identificadores")
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	if m.EditOf != "" || m.RevokeOf != "" {
		return false, s.ingestChange(ctx, m) // altera uma mensagem existente; não cria nenhuma
	}
	payload := m.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	if m.IsGroup {
		payload["group"] = true // mensagens de grupo mostram o autor
		if m.SenderJID != "" {
			payload["senderJid"] = m.SenderJID
		}
	}

	var convID, msgID string
	var retryID, retryConv string // PDF de histórico já importado, mas ainda sem arquivo
	var before userSet
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var dup bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM messages WHERE chat_jid = $1 AND external_id = $2)`,
			m.ChatJID, m.ExternalID).Scan(&dup); err != nil {
			return err
		}
		if dup {
			// PDF de histórico reentregue (o administrador pediu o histórico de novo) cuja mensagem já existia sem o arquivo:
			// tenta baixar agora. Nada mais da mensagem muda.
			if m.Historical && m.Download != nil && m.Kind == whatsapp.KindDocument {
				err := tx.QueryRow(ctx, `
					SELECT m.id::text, m.conversation_id::text FROM messages m
					 WHERE m.chat_jid = $1 AND m.external_id = $2
					   AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = m.id)
					   AND coalesce(m.payload->>'mediaState', '') <> 'blocked'`, m.ChatJID, m.ExternalID).Scan(&retryID, &retryConv)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			return nil
		}

		var contactID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO contacts (wa_id, phone, name, kind) VALUES ($1, $2, $3, $4)
			ON CONFLICT (wa_id) DO UPDATE SET
			    phone = CASE WHEN contacts.phone = '' THEN EXCLUDED.phone ELSE contacts.phone END,
			    -- o assunto do grupo acompanha renomeações; nomes de pessoas só são preenchidos se vazios
			    name  = CASE WHEN contacts.name = '' OR (EXCLUDED.kind = 'group' AND EXCLUDED.name <> '' AND NOT contacts.name_edited)
			                 THEN EXCLUDED.name ELSE contacts.name END
			RETURNING id`,
			m.ContactWAID, m.ContactPhone, contactName(m), contactKind(m)).Scan(&contactID); err != nil {
			return err
		}

		// Conversa aberta do contato (no máximo uma — índice único parcial).
		created := true
		err := pgx.ErrNoRows
		if m.Historical {
			// Backfill belongs to the existing thread and must not reopen an
			// atendimento that staff already closed.
			err = tx.QueryRow(ctx, `SELECT id FROM conversations WHERE contact_id=$1
				ORDER BY (status <> 'closed') DESC, created_at DESC LIMIT 1 FOR UPDATE`, contactID).Scan(&convID)
			if err == nil {
				created = false
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
			INSERT INTO conversations (contact_id, last_message_at) VALUES ($1, $2)
			ON CONFLICT (contact_id) WHERE status <> 'closed' DO NOTHING
			RETURNING id`, contactID, m.Timestamp).Scan(&convID)
			if errors.Is(err, pgx.ErrNoRows) {
				created = false
				err = tx.QueryRow(ctx,
					`SELECT id FROM conversations WHERE contact_id = $1 AND status <> 'closed' FOR UPDATE`,
					contactID).Scan(&convID)
			}
		}
		if err != nil {
			return err
		}

		dir, status := "in", (*string)(nil)
		if m.FromMe {
			dir = "out"
			sent := "sent"
			status = &sent
		}
		if m.Quote != nil && m.Quote.ExternalID != "" {
			payload["quoted"] = s.incomingQuote(ctx, tx, m)
		}
		pj, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO messages (conversation_id, chat_jid, external_id, direction, kind, body, payload, sender_name, status, sent_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (chat_jid, external_id) DO NOTHING
			RETURNING id`,
			convID, m.ChatJID, m.ExternalID, dir, string(m.Kind), m.Body, pj,
			senderLabel(m), status, m.Timestamp,
		).Scan(&msgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // corrida com outra entrega do mesmo evento
		}
		if err != nil {
			return err
		}

		unread := 0
		if !m.FromMe && !m.Historical {
			unread = 1
			res, err := tx.Exec(ctx, `UPDATE conversations SET snoozed_until = NULL, snoozed_by = NULL WHERE id = $1 AND snoozed_until IS NOT NULL`, convID)
			if err != nil {
				return err
			}
			if res.RowsAffected() > 0 {
				if err := audit.Record(ctx, tx, audit.Entry{Action: "conversation.resume", TargetType: "conversation", TargetID: convID, Metadata: map[string]any{"reason": "inbound_message"}}); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE conversations
			   SET last_message_at = greatest(last_message_at, $2),
			       unread_count = unread_count + $3
			 WHERE id = $1`, convID, m.Timestamp, unread); err != nil {
			return err
		}

		// Classificação: só mensagens do cliente, só enquanto a conversa não tem fila
		// e ninguém assumiu — conversa em andamento nunca muda de fila por palavras isoladas.
		// Documento sem legenda: o nome do arquivo ("Folha_Outubro.pdf") também diz o assunto.
		text := m.Body
		if m.Kind == whatsapp.KindDocument {
			if fn, _ := m.Payload["fileName"].(string); fn != "" {
				text = strings.TrimSpace(text + " " + fn)
			}
		}
		if m.FromMe || m.Historical || text == "" {
			return nil
		}
		var curSeg, assignee *string
		var curStatus string
		if err := tx.QueryRow(ctx, `SELECT segment_id, status, assignee_id FROM conversations WHERE id = $1`, convID).
			Scan(&curSeg, &curStatus, &assignee); err != nil {
			return err
		}
		if curSeg != nil || curStatus != "new" || assignee != nil {
			// Conversa em andamento: não muda de dono nem de fila, mas a equipe do assunto entra como participante.
			if !m.IsGroup && curStatus != "closed" {
				return s.callTopicTeam(ctx, tx, convID, text, curSeg, assignee, &before)
			}
			return nil
		}
		kws, err := segments.LoadKeywords(ctx, tx)
		if err != nil {
			return err
		}
		res := segments.Classify(text, kws)
		switch res.Reason {
		case segments.ReasonKeyword:
			if !created { // a conversa já era visível a todos; avisar quem sai da visão
				if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
					return err
				}
			}
			_, err = tx.Exec(ctx, `UPDATE conversations SET segment_id = $2, classification = 'keyword', classified_at = now() WHERE id = $1`,
				convID, res.SegmentID)
			return err
		case segments.ReasonConflict:
			_, err = tx.Exec(ctx, `UPDATE conversations SET classification = 'conflict', classified_at = now() WHERE id = $1`, convID)
			return err
		}
		return nil
	})
	if err == nil && retryID != "" {
		s.queueHistoricalPDF(retryID, retryConv, m)
	}
	// "PARAR": o cliente não quer avisos automáticos. Vale na hora e é auditado; a mensagem segue o fluxo normal.
	if err == nil && msgID != "" && !m.FromMe && !m.Historical && m.Kind == whatsapp.KindText && IsOptOut(m.Body) {
		s.optOut(ctx, m.ContactWAID)
	}
	if err != nil || msgID == "" {
		return false, err
	}

	// Histórico chega em rajadas de milhares de mensagens: publicar cada uma
	// enche o buffer do WebSocket e derruba o navegador como "cliente lento".
	// O fim de cada lote (whatsapp.sync) já faz o frontend recarregar tudo.
	if !m.Historical {
		after := s.publishState(ctx, convID, before)
		s.publishMessage(ctx, EventMessageCreated, msgID, after)
		if !m.FromMe {
			s.publishTyping(convID, m.SenderID, false, after)
			if s.inbound != nil {
				s.inbound.OnInbound(context.WithoutCancel(ctx), InboundEvent{ConversationID: convID, MessageID: msgID, Group: m.IsGroup})
			}
		}
	}
	if m.Download != nil && s.store != nil && s.media != nil {
		if m.Historical {
			s.queueHistoricalPDF(msgID, convID, m) // o provedor só deixa o download nos PDFs do histórico
		} else {
			go func() {
				s.mediaSlots <- struct{}{} // no máximo alguns downloads simultâneos
				defer func() { <-s.mediaSlots }()
				s.fetchMedia(msgID, convID, m)
			}()
		}
	}
	return true, nil
}

// senderLabel: nome exibido do cliente nas mensagens recebidas.
func senderLabel(m whatsapp.InboundMessage) string {
	if m.FromMe {
		return ""
	}
	if m.IsGroup {
		return strings.TrimSpace(m.SenderName)
	}
	return strings.TrimSpace(m.PushName)
}

func contactKind(m whatsapp.InboundMessage) string {
	if m.IsGroup {
		return "group"
	}
	return "person"
}

// contactName: o assunto para grupos, o nome de perfil para pessoas.
func contactName(m whatsapp.InboundMessage) string {
	if m.IsGroup {
		return strings.TrimSpace(m.GroupName)
	}
	return strings.TrimSpace(m.PushName)
}

// ---------------------------------------------------------------------------
// Consultas

type Contact struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Company string `json:"company,omitempty"`
	Note    string `json:"note"`
	Saved   bool   `json:"saved"`
	// Kind: "person" ou "group". Avatar: URL da foto de perfil (vazio = iniciais).
	Kind   string `json:"kind"`
	Avatar string `json:"avatarUrl,omitempty"`
}

// finish preenche os campos derivados da linha de contacts.
func (c *Contact) finish(name, phone, waID, kind string, avatarVersion int64) {
	c.Kind = kind
	c.Name = displayName(name, phone, waID)
	if kind == "group" && name == "" {
		c.Name = "Grupo"
	}
	c.Phone = FormatPhone(phone)
	if avatarVersion > 0 {
		c.Avatar = fmt.Sprintf("/api/v1/contacts/%s/avatar?v=%d", c.ID, avatarVersion)
	}
}

type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Message é serializada "achatada", no mesmo formato do frontend
// (campos de mídia/localização no nível de cima).
type Message map[string]any

type Conversation struct {
	ID             string     `json:"id"`
	Contact        Contact    `json:"contact"`
	Segment        string     `json:"segment"`     // chave do segmento ("geral" = fila geral)
	SegmentName    string     `json:"segmentName"` // nome para exibição
	Classification string     `json:"classification"`
	Status         string     `json:"status"`
	SnoozedUntil   *time.Time `json:"snoozedUntil,omitempty"`
	Assignee       *Ref       `json:"assignee,omitempty"`
	Participants   []Ref      `json:"participants"` // outras pessoas incluídas no atendimento
	UnreadCount    int        `json:"unreadCount"`
	Tags           []string   `json:"tags"`
	LastMessage    Message    `json:"lastMessage,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

const messageCols = `m.id, m.conversation_id, m.direction, m.kind, m.body, m.payload, m.status,
	m.sent_at, m.sender_name, m.delivered_at, m.read_at, m.played_at,
	(SELECT a.id::text FROM attachments a WHERE a.message_id = m.id), m.edited_at, m.deleted_at, m.sender_user_id::text`

type msgRow struct {
	id, convID, dir, kind, body, sender string
	payload                             []byte
	status                              *string
	sentAt                              time.Time
	deliveredAt, readAt, playedAt       *time.Time
	attachmentID                        *string
	editedAt, deletedAt                 *time.Time
	senderID                            *string // quem enviou (pessoa do escritório); vazio em mensagens do cliente e da IA
}

func (r *msgRow) dest() []any {
	return []any{&r.id, &r.convID, &r.dir, &r.kind, &r.body, &r.payload, &r.status, &r.sentAt, &r.sender, &r.deliveredAt, &r.readAt, &r.playedAt, &r.attachmentID, &r.editedAt, &r.deletedAt, &r.senderID}
}

func (r msgRow) message() Message {
	m := Message{}
	_ = json.Unmarshal(r.payload, &m)
	m["id"], m["conversationId"], m["direction"], m["kind"] = r.id, r.convID, r.dir, r.kind
	m["sentAt"] = r.sentAt.UTC().Format(time.RFC3339Nano)
	if r.status != nil {
		m["status"] = *r.status
	}
	if r.deliveredAt != nil {
		m["deliveredAt"] = r.deliveredAt.UTC().Format(time.RFC3339Nano)
	}
	if r.readAt != nil {
		m["readAt"] = r.readAt.UTC().Format(time.RFC3339Nano)
	}
	if r.senderID != nil && r.dir == "out" {
		m["authorId"] = *r.senderID // identidade estável de quem enviou (o nome pode mudar)
	}
	if r.playedAt != nil {
		m["playedAt"] = r.playedAt.UTC().Format(time.RFC3339Nano)
	}
	if r.attachmentID != nil {
		m["url"] = "/api/v1/attachments/" + *r.attachmentID
	}
	if r.sender != "" && (r.dir == "out" || m["group"] == true) {
		m["authorName"] = r.sender
	}
	delete(m, "senderJid") // interno: só serve para citar a mensagem no WhatsApp
	if q, ok := m["quoted"].(map[string]any); ok {
		delete(q, "senderJid")
	}
	if r.editedAt != nil {
		m["editedAt"] = r.editedAt.UTC().Format(time.RFC3339Nano)
	}
	if r.deletedAt != nil {
		// Apagada para todos: o conteúdo some, a marca fica.
		keep := Message{"id": r.id, "conversationId": r.convID, "direction": r.dir, "kind": r.kind,
			"sentAt": m["sentAt"], "deleted": true}
		for _, k := range []string{"status", "authorName", "group"} {
			if v, ok := m[k]; ok {
				keep[k] = v
			}
		}
		keep["text"] = ""
		return keep
	}
	switch r.kind {
	case "text", "unsupported":
		m["text"] = r.body
	case "image", "document":
		if r.body != "" {
			m["caption"] = r.body
		}
	}
	return m
}

func displayName(name, phone, waID string) string {
	if name != "" {
		return name
	}
	if phone != "" {
		return FormatPhone(phone)
	}
	return waID
}

const listQuery = `
	SELECT c.id, c.status, c.unread_count, c.last_message_at, c.classification, c.snoozed_until,
	       ct.id, ct.name, ct.phone, ct.wa_id, ct.company, ct.note, ct.kind, (CASE WHEN ct.avatar IS NULL THEN 0 ELSE ct.avatar_version END), (ct.name_edited OR ct.whatsapp_saved),
	       u.id, u.name,
	       s.key, s.name::text,
	       coalesce((SELECT array_agg(t.name::text ORDER BY t.name) FROM conversation_tags cg
	                  JOIN tags t ON t.id = cg.tag_id WHERE cg.conversation_id = c.id), '{}'),
	       lm.id, lm.conversation_id, lm.direction, lm.kind, lm.body, lm.payload, lm.status,
	       lm.sent_at, lm.sender_name, lm.delivered_at, lm.read_at, lm.edited_at, lm.deleted_at, lm.sender_user_id::text,
	       coalesce((SELECT json_agg(json_build_object('id', pu.id, 'name', pu.name) ORDER BY cp.created_at)
	                  FROM conversation_participants cp JOIN users pu ON pu.id = cp.user_id WHERE cp.conversation_id = c.id), '[]'::json)
	  FROM conversations c
	  JOIN contacts ct ON ct.id = c.contact_id
	  LEFT JOIN segments s ON s.id = c.segment_id
	  LEFT JOIN users u ON u.id = c.assignee_id
	  LEFT JOIN LATERAL (
	       SELECT m.* FROM messages m WHERE m.conversation_id = c.id
	        ORDER BY m.sent_at DESC, m.created_at DESC LIMIT 1) lm ON true`

func scanConversation(row pgx.Row) (Conversation, error) {
	var (
		c                 Conversation
		name, phone, waID string
		kind              string
		avatarVer         int64
		uID, uName        *string
		segKey, segName   *string
		parts             []byte
		lm                struct {
			id, convID, dir, kind, body, sender *string
			payload                             []byte
			status                              *string
			sentAt, deliveredAt, readAt         *time.Time
			editedAt, deletedAt                 *time.Time
			senderID                            *string
		}
	)
	err := row.Scan(&c.ID, &c.Status, &c.UnreadCount, &c.UpdatedAt, &c.Classification, &c.SnoozedUntil,
		&c.Contact.ID, &name, &phone, &waID, &c.Contact.Company, &c.Contact.Note, &kind, &avatarVer, &c.Contact.Saved,
		&uID, &uName, &segKey, &segName, &c.Tags,
		&lm.id, &lm.convID, &lm.dir, &lm.kind, &lm.body, &lm.payload, &lm.status, &lm.sentAt, &lm.sender, &lm.deliveredAt, &lm.readAt, &lm.editedAt, &lm.deletedAt, &lm.senderID, &parts)
	if err != nil {
		return c, err
	}
	c.Participants = []Ref{}
	_ = json.Unmarshal(parts, &c.Participants)
	c.Contact.finish(name, phone, waID, kind, avatarVer)
	if c.Contact.Phone == "" {
		c.Contact.Phone = waID
	}
	c.Segment, c.SegmentName = "geral", "Não atribuídos"
	if segKey != nil {
		c.Segment, c.SegmentName = *segKey, *segName
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	if uID != nil {
		c.Assignee = &Ref{ID: *uID, Name: *uName}
	}
	if lm.id != nil {
		c.LastMessage = msgRow{
			id: *lm.id, convID: *lm.convID, dir: *lm.dir, kind: *lm.kind, body: *lm.body, sender: *lm.sender,
			payload: lm.payload, status: lm.status, sentAt: *lm.sentAt, deliveredAt: lm.deliveredAt, readAt: lm.readAt,
			editedAt: lm.editedAt, deletedAt: lm.deletedAt, senderID: lm.senderID,
		}.message()
	}
	return c, nil
}

// fetch lê uma conversa sem checar visibilidade (uso interno, p.ex. para publicar eventos).
func (s *Service) fetch(ctx context.Context, db database.DBTX, id string) (Conversation, error) {
	c, err := scanConversation(db.QueryRow(ctx, listQuery+` WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

// List devolve até 300 conversas ativas e 300 adiadas que o viewer pode ver.
// Conversas encerradas não aparecem: o contato só volta à lista quando manda uma nova
// mensagem (que abre uma nova conversa). O histórico continua acessível pelo detalhe.
func (s *Service) List(ctx context.Context, v Viewer) ([]Conversation, error) {
	// Separate limits keep postponed work from displacing the active queue.
	visibleIDs := `SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
		WHERE c.status <> 'closed' AND ` + forViewer
	rows, err := s.db.Query(ctx, listQuery+` WHERE c.id IN ((`+visibleIDs+`
		AND c.snoozed_until IS NULL ORDER BY c.last_message_at DESC LIMIT 300) UNION ALL (`+visibleIDs+`
		AND c.snoozed_until IS NOT NULL ORDER BY c.snoozed_until LIMIT 300))
		ORDER BY c.last_message_at DESC`, v.ID, v.Admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get devolve a conversa se o viewer puder vê-la; caso contrário, ErrNotFound
// (não revela a existência).
func (s *Service) Get(ctx context.Context, v Viewer, id string) (Conversation, error) {
	if !httpx.ValidUUID(id) {
		return Conversation{}, ErrNotFound
	}
	c, err := scanConversation(s.db.QueryRow(ctx, listQuery+` WHERE c.id = $3 AND `+forViewer, v.ID, v.Admin, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

// Messages devolve as mensagens mais recentes (até 300) em ordem cronológica. Inclui as de
// atendimentos anteriores do mesmo contato, mas só os que o viewer poderia ver: quem
// atende o cliente que volta enxerga o histórico, sem expor conversas de outras equipes.
func (s *Service) Messages(ctx context.Context, v Viewer, conversationID string, limit int) ([]Message, error) {
	if limit < 1 || limit > 5000 {
		limit = 300
	}
	// A checagem de acesso e a leitura das mensagens vão ao banco ao mesmo tempo (duas viagens de rede em vez de uma depois da outra).
	access := make(chan error, 1)
	go func() {
		_, err := s.Get(ctx, v, conversationID)
		access <- err
	}()
	rows, err := s.db.Query(ctx, `
		SELECT * FROM (
		    SELECT `+messageCols+`, m.created_at FROM messages m
		      JOIN conversations c ON c.id = m.conversation_id
		      LEFT JOIN segments s ON s.id = c.segment_id
		     WHERE c.contact_id = (SELECT contact_id FROM conversations WHERE id = $3) AND `+forViewer+`
		       AND NOT EXISTS (SELECT 1 FROM message_hidden h WHERE h.message_id = m.id AND h.user_id = $1)
		     ORDER BY m.sent_at DESC, m.created_at DESC LIMIT $4) x
		 ORDER BY sent_at, created_at`, v.ID, v.Admin, conversationID, limit)
	if aerr := <-access; aerr != nil { // sem acesso: nada do que foi lido é devolvido
		if err == nil {
			rows.Close()
		}
		return nil, aerr
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var r msgRow
		var createdAt time.Time
		if err := rows.Scan(append(r.dest(), &createdAt)...); err != nil {
			return nil, err
		}
		out = append(out, r.message())
	}
	return out, rows.Err()
}

func (s *Service) message(ctx context.Context, id string) (Message, error) {
	var r msgRow
	if err := s.db.QueryRow(ctx, `SELECT `+messageCols+` FROM messages m WHERE m.id = $1`, id).Scan(r.dest()...); err != nil {
		return nil, err
	}
	return r.message(), nil
}

func (s *Service) MarkRead(ctx context.Context, v Viewer, id string) error {
	if _, err := s.Get(ctx, v, id); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE conversations SET unread_count = 0 WHERE id = $1 AND unread_count > 0`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.publishState(ctx, id, nil)
	}
	return nil
}

type ContactInput struct {
	Name    *string `json:"name"`
	Company *string `json:"company"`
	Note    *string `json:"note"`
}

func (s *Service) UpdateContact(ctx context.Context, v Viewer, contactID string, in ContactInput) error {
	if !httpx.ValidUUID(contactID) {
		return ErrNotFound
	}
	// O viewer precisa poder ver alguma conversa do contato; o administrador edita também contatos
	// importados que ainda não têm conversa (lista de Clientes).
	var convID string
	var err error
	if v.Admin {
		err = s.db.QueryRow(ctx, `SELECT id::text FROM contacts WHERE id = $1`, contactID).Scan(&convID)
		if err == nil { // conversa mais recente (se houver), só para avisar as telas abertas
			convID = ""
			_ = s.db.QueryRow(ctx, `SELECT id::text FROM conversations WHERE contact_id = $1 ORDER BY last_message_at DESC LIMIT 1`, contactID).Scan(&convID)
		}
	} else {
		err = s.db.QueryRow(ctx, `
		SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
		 WHERE c.contact_id = $3 AND `+forViewer+` ORDER BY c.last_message_at DESC LIMIT 1`,
			v.ID, v.Admin, contactID).Scan(&convID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	f := httpx.FieldErrors{}
	trim := func(p *string) {
		if p != nil {
			*p = strings.TrimSpace(*p)
		}
	}
	trim(in.Name)
	trim(in.Company)
	if in.Name != nil && utf8.RuneCountInString(*in.Name) > 120 {
		f["name"] = "Nome deve ter até 120 caracteres."
	}
	if in.Company != nil && utf8.RuneCountInString(*in.Company) > 120 {
		f["company"] = "Empresa deve ter até 120 caracteres."
	}
	if in.Note != nil && utf8.RuneCountInString(*in.Note) > 2000 {
		f["note"] = "Observação deve ter até 2000 caracteres."
	}
	if err := f.Err(); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE contacts SET name = coalesce($2, name), company = coalesce($3, company), note = coalesce($4, note),
		       -- nome apagado = volta a seguir o nome do WhatsApp; nome digitado fica fixo
		       name_edited = CASE WHEN $2::text IS NULL THEN name_edited ELSE $2::text <> '' END
		 WHERE id = $1`, contactID, in.Name, in.Company, in.Note); err != nil {
		return err
	}
	if convID != "" {
		s.publishState(ctx, convID, nil)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Responder: atribuição atômica + envio

type SendInput struct {
	ID   string // opcional; torna o envio idempotente (reenvio com o mesmo ID não duplica)
	Kind string // text (padrão) | image | audio | document | location
	Text string // texto, ou legenda quando há anexo

	Att      *media.Processed // image | audio | document (já validado)
	HD       bool
	Waveform []float64 // áudio: barras de 0 a 1

	Lat, Lng      float64 // location
	Name, Address string

	ReplyTo string // id da mensagem respondida (opcional)
}

const (
	maxTextRunes    = 4000
	maxCaptionRunes = 1024
)

// SendText envia uma resposta de texto. Responder uma conversa sem responsável a
// assume: o SELECT ... FOR UPDATE na conversa serializa respostas simultâneas, então
// só uma pessoa vence e a outra recebe TakenError. (O índice único parcial em
// conversation_assignments é uma segunda barreira, no próprio banco.)
//
// Retorna a mensagem e created=false quando a requisição repete um envio anterior.
func (s *Service) Send(ctx context.Context, v Viewer, convID string, in SendInput) (Message, bool, error) {
	text := strings.TrimSpace(in.Text)
	kind := in.Kind
	if kind == "" {
		kind = "text"
	}
	payload := map[string]any{}
	switch kind {
	case "text":
		if text == "" || utf8.RuneCountInString(text) > maxTextRunes {
			return nil, false, httpx.FieldErrors{"text": fmt.Sprintf("Digite uma mensagem de até %d caracteres.", maxTextRunes)}
		}
	case "image", "audio", "document", "sticker":
		if in.Att == nil || s.store == nil {
			return nil, false, ErrUnavailable
		}
		if utf8.RuneCountInString(text) > maxCaptionRunes || ((kind == "audio" || kind == "sticker") && text != "") {
			return nil, false, httpx.FieldErrors{"text": fmt.Sprintf("A legenda pode ter até %d caracteres.", maxCaptionRunes)}
		}
		payload["mimeType"], payload["sizeBytes"] = in.Att.MimeType, len(in.Att.Data)
		switch kind {
		case "image", "sticker":
			payload["width"], payload["height"], payload["hd"] = in.Att.Width, in.Att.Height, in.HD
			if kind == "image" {
				if thumb, ok := media.Thumb(in.Att.Data); ok {
					payload["thumb"] = thumb
				}
			}
			if kind == "sticker" {
				payload["animated"] = len(in.Att.Data) > 20 && string(in.Att.Data[12:16]) == "VP8X" && in.Att.Data[20]&2 != 0
			}
		case "audio":
			payload["durationSec"], payload["voiceNote"] = in.Att.DurationSec, true
			if len(in.Waveform) > 0 {
				payload["waveform"] = in.Waveform
			}
		case "document":
			payload["fileName"] = in.Att.FileName
		}
	case "location":
		if in.Lat < -90 || in.Lat > 90 || in.Lng < -180 || in.Lng > 180 || (in.Lat == 0 && in.Lng == 0) {
			return nil, false, httpx.FieldErrors{"lat": "Localização inválida."}
		}
		if utf8.RuneCountInString(in.Name) > 200 || utf8.RuneCountInString(in.Address) > 400 {
			return nil, false, httpx.FieldErrors{"name": "Nome ou endereço longo demais."}
		}
		payload["lat"], payload["lng"], payload["name"], payload["address"] = in.Lat, in.Lng, in.Name, in.Address
		text = ""
	default:
		return nil, false, httpx.FieldErrors{"kind": "Tipo de mensagem inválido."}
	}
	msgID := in.ID
	if msgID == "" {
		msgID = uuid.NewString()
	} else if !httpx.ValidUUID(msgID) {
		return nil, false, httpx.FieldErrors{"id": "Identificador inválido."}
	}
	if !httpx.ValidUUID(convID) {
		return nil, false, ErrNotFound
	}
	if s.sender == nil {
		return nil, false, ErrUnavailable
	}

	var (
		externalID, chatJID string
		created, resend     bool
		claimed             bool
		before              userSet
		attKey              string // objeto a enviar (novo ou, no reenvio, o já guardado)
		storedNew           bool   // objeto gravado nesta chamada (apagar se a mensagem não for criada)
	)
	attID := uuid.NewString()
	if in.Att != nil && kind != "text" && kind != "location" {
		// Grava o binário antes da transação; se a mensagem não nascer, é apagado abaixo.
		key := "attachments/" + attID
		if err := s.store.Put(ctx, key, in.Att.Data); err != nil {
			return nil, false, err
		}
		attKey, storedNew = key, true
		defer func() {
			if storedNew {
				_ = s.store.Delete(context.WithoutCancel(ctx), key)
			}
		}()
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			status, assigneeName string
			assignee             *string
			segID, teamID        *string
			lastJID, contactWAID string
			snoozedUntil         *time.Time
			isParticipant        bool // incluída no atendimento: responde sem assumir
		)
		// Uma ida ao banco só: o chat_jid da última mensagem vem junto (cada consulta
		// custa uma viagem de rede inteira até o Postgres).
		err := tx.QueryRow(ctx, `
			SELECT c.status, c.assignee_id, coalesce(u.name, ''), c.segment_id, s.team_id,
			       coalesce((SELECT m.chat_jid FROM messages m WHERE m.conversation_id = c.id
			                  ORDER BY m.sent_at DESC, m.created_at DESC LIMIT 1), ''),
			       (SELECT ct.wa_id FROM contacts ct WHERE ct.id = c.contact_id), c.snoozed_until,
			       EXISTS (SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = c.id AND cp.user_id = $1)
			  FROM conversations c
			  LEFT JOIN segments s ON s.id = c.segment_id
			  LEFT JOIN users u ON u.id = c.assignee_id
			 WHERE c.id = $3 AND `+forViewer+`
			 FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&status, &assignee, &assigneeName, &segID, &teamID, &lastJID, &contactWAID, &snoozedUntil, &isParticipant)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		// Reenvio da mesma mensagem (duplo clique, "tentar de novo", requisição repetida).
		if snoozedUntil != nil {
			return httpx.FieldErrors{"text": "Retome o atendimento antes de enviar uma mensagem."}
		}
		var ex struct {
			conv, dir, ext string
			sender         *string
			status         *string
		}
		err = tx.QueryRow(ctx, `SELECT conversation_id, direction, external_id, sender_user_id, status FROM messages WHERE id = $1`, msgID).
			Scan(&ex.conv, &ex.dir, &ex.ext, &ex.sender, &ex.status)
		switch {
		case err == nil:
			if ex.conv != convID || ex.dir != "out" || ex.sender == nil || *ex.sender != v.ID {
				return ErrIDConflict
			}
			if ex.status == nil || *ex.status != "failed" {
				return nil // já enviada (ou em envio): devolve como está
			}
			if assignee == nil || (*assignee != v.ID && !isParticipant) {
				return &TakenError{Assignee: assigneeName}
			}
			if !s.sender.Connected() {
				return ErrUnavailable
			}
			if _, err := tx.Exec(ctx, `UPDATE messages SET status = 'pending', error = '' WHERE id = $1`, msgID); err != nil {
				return err
			}
			externalID, resend = ex.ext, true
			var raw []byte
			if err := tx.QueryRow(ctx, `SELECT kind, body, payload FROM messages WHERE id = $1`, msgID).Scan(&kind, &text, &raw); err != nil {
				return err
			}
			payload = map[string]any{}
			_ = json.Unmarshal(raw, &payload)
			var key *string
			_ = tx.QueryRow(ctx, `SELECT storage_key FROM attachments WHERE message_id = $1`, msgID).Scan(&key)
			if key != nil {
				attKey = *key
			}
		case errors.Is(err, pgx.ErrNoRows):
			if status == "closed" {
				return ErrClosed
			}
			if assignee != nil && *assignee != v.ID && !isParticipant {
				return &TakenError{Assignee: assigneeName}
			}
			// Sem WhatsApp conectado a resposta falharia: não assume nem grava nada.
			if !s.sender.Connected() {
				return ErrUnavailable
			}
			// Conversa iniciada por nós (sem mensagens ainda): o destino vem do identificador do contato.
			if chatJID = lastJID; chatJID == "" {
				chatJID = jidOf(contactWAID)
			}
			if chatJID == "" {
				return pgx.ErrNoRows
			}
			if assignee == nil {
				if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE conversations SET assignee_id = $2, status = 'in_progress' WHERE id = $1`, convID, v.ID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO conversation_assignments (conversation_id, user_id, team_id, segment_id) VALUES ($1, $2, $3, $4)`,
					convID, v.ID, teamID, segID); err != nil {
					return err // violação do índice único = outra pessoa assumiu primeiro
				}
				claimed = true
				if err := audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationClaim, TargetType: "conversation", TargetID: convID,
					Metadata: map[string]any{"segmentId": segID, "teamId": teamID}}); err != nil {
					return err
				}
			}
			externalID = s.sender.NewMessageID()
			if in.ReplyTo != "" && kind != "location" {
				q, err := s.resolveReply(ctx, tx, convID, in.ReplyTo)
				if err != nil {
					return err
				}
				payload["quoted"] = q
			}
			pj, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO messages (id, conversation_id, chat_jid, external_id, direction, kind, body, payload, sender_name, sender_user_id, status, sent_at)
				VALUES ($1, $2, $3, $4, 'out', $8, $5, $9, $6, $7, 'pending', now())`,
				msgID, convID, chatJID, externalID, text, v.Name, v.ID, kind, pj); err != nil {
				return err
			}
			if storedNew {
				sum := sha256.Sum256(in.Att.Data)
				if _, err := tx.Exec(ctx, `
					INSERT INTO attachments (id, message_id, storage_key, mime_type, size_bytes, sha256, file_name, scan_status)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
					attID, msgID, attKey, in.Att.MimeType, len(in.Att.Data), hex.EncodeToString(sum[:]), in.Att.FileName, in.Att.ScanStatus); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE conversations SET last_message_at = now(), unread_count = 0 WHERE id = $1`, convID); err != nil {
				return err
			}
			created = true
		default:
			return err
		}
		if chatJID == "" {
			return tx.QueryRow(ctx, `SELECT chat_jid FROM messages WHERE id = $1`, msgID).Scan(&chatJID)
		}
		return nil
	})
	var constraintErr *pgconn.PgError
	if errors.As(err, &constraintErr) && constraintErr.Code == database.UniqueViolation {
		switch constraintErr.ConstraintName {
		case "conversation_assignments_one_active":
			// Only the assignment index indicates a race for the conversation.
			return nil, false, &TakenError{Assignee: s.currentAssignee(ctx, convID)}
		case "messages_pkey":
			return nil, false, ErrIDConflict
		}
	}
	if err != nil {
		return nil, false, err
	}
	if created {
		storedNew = false // a mensagem existe: o objeto é dela
	}
	if !created && !resend {
		m, err := s.message(ctx, msgID)
		return m, false, err
	}

	// Os eventos em tempo real (várias consultas) saem em paralelo: a entrega ao
	// WhatsApp não espera por eles. O canal garante a ordem "pendente" → "enviada"
	// para quem está vendo a conversa em outro navegador.
	bg := context.WithoutCancel(ctx)
	published := make(chan struct{})
	go func() {
		defer close(published)
		after := s.publishState(bg, convID, before)
		if created {
			s.publishMessage(bg, EventMessageCreated, msgID, after)
		} else {
			s.publishMessage(bg, EventMessageUpdated, msgID, after)
		}
	}()
	_ = claimed

	// Envio fora da transação (rede). Concluir o registro mesmo se o cliente desistir da requisição.
	sendCtx, cancel := context.WithTimeout(bg, 45*time.Second)
	defer cancel()
	waStart := time.Now()
	var author string
	if err := s.db.QueryRow(sendCtx, `SELECT sender_name FROM messages WHERE id = $1`, msgID).Scan(&author); err != nil {
		return nil, false, err
	}
	wireText := text
	if kind == "text" || ((kind == "image" || kind == "document") && text != "") {
		wireText = signedText(author, text)
	}
	sendErr := s.dispatch(sendCtx, kind, chatJID, externalID, wireText, payload, attKey, in.Att)
	if d := time.Since(waStart); d > 3*time.Second {
		slog.Warn("envio lento ao WhatsApp", "kind", kind, "duration_ms", d.Milliseconds(), "err", sendErr)
	}
	if sendErr != nil {
		if _, err := s.db.Exec(sendCtx, `UPDATE messages SET status = 'failed', error = $2 WHERE id = $1 AND status = 'pending'`,
			msgID, "Não foi possível enviar pelo WhatsApp."); err != nil {
			return nil, false, err
		}
	} else if _, err := s.db.Exec(sendCtx, `UPDATE messages SET status = 'sent' WHERE id = $1 AND status = 'pending'`, msgID); err != nil {
		return nil, false, err
	}
	go func() {
		<-published
		after := s.publishState(bg, convID, nil)
		s.publishMessage(bg, EventMessageUpdated, msgID, after)
	}()
	m, err := s.message(sendCtx, msgID)
	return m, created, err
}

// A identificação vai só para o WhatsApp; o corpo armazenado permanece limpo no app.
func signedText(author, text string) string {
	author = strings.Join(strings.Fields(strings.ReplaceAll(author, "*", "")), " ")
	if author == "" {
		return text
	}
	return "*" + author + ":*\n" + text
}

// dispatch entrega a mensagem ao WhatsApp. No reenvio de uma mídia, o arquivo é relido do armazenamento.
func (s *Service) dispatch(ctx context.Context, kind, chatJID, externalID, text string, payload map[string]any, attKey string, att *media.Processed) error {
	switch kind {
	case "text":
		if q := quoteFromPayload(payload); q != nil {
			return s.sender.SendTextQuoted(ctx, chatJID, externalID, text, *q)
		}
		return s.sender.SendText(ctx, chatJID, externalID, text)
	case "location":
		f := func(k string) float64 { v, _ := payload[k].(float64); return v }
		str := func(k string) string { v, _ := payload[k].(string); return v }
		return s.sender.SendLocation(ctx, chatJID, externalID, whatsapp.Location{
			Lat: f("lat"), Lng: f("lng"), Name: str("name"), Address: str("address")})
	}
	var data []byte
	if att != nil {
		data = att.Data
	} else {
		b, err := s.store.Get(ctx, attKey)
		if err != nil {
			return err
		}
		data = b
	}
	str := func(k string) string { v, _ := payload[k].(string); return v }
	num := func(k string) int {
		switch v := payload[k].(type) { // int ao enviar agora; float64 ao reler do banco (reenvio)
		case int:
			return v
		case float64:
			return int(v)
		}
		return 0
	}
	md := whatsapp.MediaSend{
		Kind: whatsapp.Kind(kind), Data: data, MimeType: str("mimeType"), FileName: str("fileName"), Caption: text,
		DurationSec: num("durationSec"), Width: num("width"), Height: num("height"), Quote: quoteFromPayload(payload),
	}
	md.Animated, _ = payload["animated"].(bool)
	if wf, ok := payload["waveform"].([]any); ok {
		for _, v := range wf {
			f, _ := v.(float64)
			md.Waveform = append(md.Waveform, byte(max(0, min(100, int(f*100+0.5)))))
		}
	} else if wf, ok := payload["waveform"].([]float64); ok {
		for _, f := range wf {
			md.Waveform = append(md.Waveform, byte(max(0, min(100, int(f*100+0.5)))))
		}
	}
	return s.sender.SendMedia(ctx, chatJID, externalID, md)
}

func (s *Service) currentAssignee(ctx context.Context, convID string) string {
	var name string
	_ = s.db.QueryRow(ctx, `SELECT coalesce(u.name, '') FROM conversations c LEFT JOIN users u ON u.id = c.assignee_id WHERE c.id = $1`, convID).Scan(&name)
	return name
}

// ---------------------------------------------------------------------------
// Encerrar e etiquetas

// Close encerra o atendimento. Quem pode: o responsável e administradores.
func (s *Service) Close(ctx context.Context, v Viewer, convID string) error {
	if !httpx.ValidUUID(convID) {
		return ErrNotFound
	}
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var status string
		var assignee *string
		err := tx.QueryRow(ctx, `
			SELECT c.status, c.assignee_id FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
			 WHERE c.id = $3 AND `+forViewer+` FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&status, &assignee)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return nil
		}
		if !v.CanSupervise() && (assignee == nil || *assignee != v.ID) {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET status = 'closed', closed_at = now(), snoozed_until = NULL, snoozed_by = NULL WHERE id = $1`, convID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE conversation_assignments SET ended_at = now(), ended_reason = 'closed'
			WHERE conversation_id = $1 AND ended_at IS NULL`, convID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationClose, TargetType: "conversation", TargetID: convID})
	})
	if err != nil {
		return err
	}
	s.publishState(ctx, convID, nil) // encerrada: sai da lista de quem a via (ver publishState)
	return nil
}

// Transfer passa o atendimento para outra pessoa do escritório (mesmo de outra equipe).
//
// Quem pode: o responsável atual, um administrador ou, se a conversa ainda não tem
// responsável, qualquer pessoa que a veja. A nova pessoa passa a enxergar e atender a
// conversa; o histórico de atribuições registra a transferência.
func (s *Service) Transfer(ctx context.Context, v Viewer, convID, targetID string) error {
	if !httpx.ValidUUID(convID) {
		return ErrNotFound
	}
	if !httpx.ValidUUID(targetID) {
		return httpx.FieldErrors{"userId": "Escolha uma pessoa."}
	}
	var (
		before  userSet
		changed bool
	)
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var (
			status, assigneeName string
			assignee             *string
			segID, teamID        *string
		)
		err := tx.QueryRow(ctx, `
			SELECT c.status, c.assignee_id, coalesce(u.name, ''), c.segment_id, s.team_id
			  FROM conversations c
			  LEFT JOIN segments s ON s.id = c.segment_id
			  LEFT JOIN users u ON u.id = c.assignee_id
			 WHERE c.id = $3 AND `+forViewer+`
			 FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&status, &assignee, &assigneeName, &segID, &teamID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return ErrClosed
		}
		if assignee != nil && *assignee != v.ID && !v.CanSupervise() {
			return &TakenError{Assignee: assigneeName}
		}

		var targetActive bool
		err = tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, targetID).Scan(&targetActive)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !targetActive) {
			return httpx.FieldErrors{"userId": "Pessoa não encontrada ou desativada."}
		}
		if err != nil {
			return err
		}
		if assignee != nil && *assignee == targetID {
			return nil // já é dela
		}

		if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE conversation_assignments SET ended_at = now(), ended_reason = 'transferred'
			WHERE conversation_id = $1 AND ended_at IS NULL`, convID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE conversations SET assignee_id = $2, status = 'in_progress' WHERE id = $1`, convID, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_assignments (conversation_id, user_id, team_id, segment_id) VALUES ($1, $2, $3, $4)`,
			convID, targetID, teamID, segID); err != nil {
			return err
		}
		changed = true
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.ConversationTransfer, TargetType: "conversation", TargetID: convID,
			Metadata: map[string]any{"from": assignee, "to": targetID}})
	})
	if err != nil {
		return err
	}
	if changed {
		s.publishState(ctx, convID, before)
	}
	return nil
}

// AddTag aplica uma etiqueta (idempotente). Etiquetas não alteram o segmento.
func (s *Service) AddTag(ctx context.Context, v Viewer, convID, tagID string) error {
	if _, err := s.Get(ctx, v, convID); err != nil {
		return err
	}
	if !httpx.ValidUUID(tagID) {
		return ErrTagNotFound
	}
	_, err := s.db.Exec(ctx, `INSERT INTO conversation_tags (conversation_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, convID, tagID)
	if database.IsCode(err, database.ForeignKeyViolation) {
		return ErrTagNotFound
	}
	if err != nil {
		return err
	}
	s.publishState(ctx, convID, nil)
	return nil
}

func (s *Service) RemoveTag(ctx context.Context, v Viewer, convID, tagID string) error {
	if _, err := s.Get(ctx, v, convID); err != nil {
		return err
	}
	if !httpx.ValidUUID(tagID) {
		return ErrTagNotFound
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM conversation_tags WHERE conversation_id = $1 AND tag_id = $2`, convID, tagID); err != nil {
		return err
	}
	s.publishState(ctx, convID, nil)
	return nil
}

// ---------------------------------------------------------------------------
// Recibos

// ApplyReceipt aplica recibos de entrega, leitura e reprodução. O status só avança
// (enviada → entregue → lida), nunca retrocede. "Ouvido" (mensagem de voz tocada pelo cliente) é
// registrado à parte em played_at: abrir a conversa só gera "lido"; tocar o áudio gera "ouvido", que também
// implica "lido".
func (s *Service) ApplyReceipt(ctx context.Context, r whatsapp.Receipt) {
	if len(r.MessageIDs) == 0 {
		return
	}
	ts := r.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	rank := func(col string) string {
		return `(CASE ` + col + ` WHEN 'sent' THEN 1 WHEN 'delivered' THEN 2 WHEN 'read' THEN 3 ELSE 0 END)`
	}
	target := map[whatsapp.ReceiptStatus]int{whatsapp.ReceiptDelivered: 2, whatsapp.ReceiptRead: 3, whatsapp.ReceiptPlayed: 3}[r.Status]
	if target == 0 {
		return
	}
	var rows pgx.Rows
	var err error
	if r.Status == whatsapp.ReceiptPlayed {
		// Só mensagem de voz. Pode chegar depois do "lido" (a conversa já estava aberta): por isso não depende do rank do status.
		rows, err = s.db.Query(ctx, `
			UPDATE messages
			   SET status = CASE WHEN `+rank("status")+` < 3 THEN 'read' ELSE status END,
			       delivered_at = coalesce(delivered_at, $2),
			       read_at = coalesce(read_at, $2),
			       played_at = coalesce(played_at, $2)
			 WHERE external_id = ANY($1) AND direction = 'out' AND kind = 'audio' AND played_at IS NULL
			RETURNING id, conversation_id`, r.MessageIDs, ts)
	} else {
		rows, err = s.db.Query(ctx, `
			UPDATE messages
			   SET status = $2,
			       delivered_at = coalesce(delivered_at, $3),
			       read_at = CASE WHEN $2 = 'read' THEN coalesce(read_at, $3) ELSE read_at END
			 WHERE external_id = ANY($1) AND direction = 'out'
			   AND `+rank("status")+` < $4
			RETURNING id, conversation_id`, r.MessageIDs, string(r.Status), ts, target)
	}
	if err != nil {
		return
	}
	type pair struct{ msg, conv string }
	var changed []pair
	for rows.Next() {
		var p pair
		if rows.Scan(&p.msg, &p.conv) == nil {
			changed = append(changed, p)
		}
	}
	rows.Close()
	seen := map[string]userSet{}
	for _, p := range changed {
		after, ok := seen[p.conv]
		if !ok {
			after = s.publishState(ctx, p.conv, nil)
			seen[p.conv] = after
		}
		s.publishMessage(ctx, EventMessageUpdated, p.msg, after)
	}
}

// FormatPhone formata números brasileiros E.164 para exibição (+55 92 99811-2040).
func FormatPhone(e164 string) string {
	d := strings.TrimPrefix(e164, "+")
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		ddd, rest := d[2:4], d[4:]
		cut := len(rest) - 4
		return "+55 " + ddd + " " + rest[:cut] + "-" + rest[cut:]
	}
	return e164
}

// ---------------------------------------------------------------------------
// Anexos

// AttachmentFile é o arquivo de um anexo.
type AttachmentFile struct {
	Data               []byte
	MimeType, FileName string
	SHA256             string // validador para o cache do navegador (ETag)
	Preview            bool   // Data é a versão reduzida
}

type attachmentMeta struct {
	key, mimeType, name, sha string
}

// attachmentMeta confere o acesso (a mesma regra de visibilidade da conversa) e devolve os dados do anexo, sem ler o arquivo.
func (s *Service) attachmentMeta(ctx context.Context, v Viewer, id string) (attachmentMeta, error) {
	if !httpx.ValidUUID(id) || s.store == nil {
		return attachmentMeta{}, ErrNotFound
	}
	var m attachmentMeta
	err := s.db.QueryRow(ctx, `
		SELECT a.storage_key, a.mime_type, a.file_name, a.sha256
		  FROM attachments a
		  JOIN messages m ON m.id = a.message_id
		  JOIN conversations c ON c.id = m.conversation_id
		  LEFT JOIN segments s ON s.id = c.segment_id
		 WHERE a.id = $3 AND `+forViewer, v.ID, v.Admin, id).Scan(&m.key, &m.mimeType, &m.name, &m.sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return attachmentMeta{}, ErrNotFound
	}
	return m, err
}

// Attachment devolve o arquivo se o viewer pode ver a conversa da mensagem.
func (s *Service) Attachment(ctx context.Context, v Viewer, id string) (AttachmentFile, error) {
	m, err := s.attachmentMeta(ctx, v, id)
	if err != nil {
		return AttachmentFile{}, err
	}
	data, err := s.store.Get(ctx, m.key)
	if errors.Is(err, storage.ErrNotFound) {
		return AttachmentFile{}, ErrNotFound
	}
	return AttachmentFile{Data: data, MimeType: m.mimeType, FileName: m.name, SHA256: m.sha}, err
}

// AttachmentPreview devolve a versão reduzida de uma foto (a do balão); sem derivado possível (não é foto, já é pequena),
// devolve o original. O derivado é criado na primeira vez e guardado ao lado do original, então as próximas leituras são leves.
func (s *Service) AttachmentPreview(ctx context.Context, v Viewer, id string) (AttachmentFile, error) {
	m, err := s.attachmentMeta(ctx, v, id)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !strings.HasPrefix(m.mimeType, "image/") || m.mimeType == "image/gif" {
		return s.Attachment(ctx, v, id)
	}
	pkey := m.key + ".preview"
	if data, err := s.store.Get(ctx, pkey); err == nil {
		return AttachmentFile{Data: data, MimeType: "image/jpeg", FileName: m.name, SHA256: m.sha + "-p", Preview: true}, nil
	}
	orig, err := s.store.Get(ctx, m.key)
	if errors.Is(err, storage.ErrNotFound) {
		return AttachmentFile{}, ErrNotFound
	}
	if err != nil {
		return AttachmentFile{}, err
	}
	prev, ok := media.Preview(orig)
	if !ok {
		return AttachmentFile{Data: orig, MimeType: m.mimeType, FileName: m.name, SHA256: m.sha}, nil
	}
	_ = s.store.Put(ctx, pkey, prev) // se não gravar, só perde o ganho da próxima vez
	return AttachmentFile{Data: prev, MimeType: "image/jpeg", FileName: m.name, SHA256: m.sha + "-p", Preview: true}, nil
}

// BackfillThumbs cria a miniatura das fotos que ainda não têm (as recebidas antes deste recurso). Roda uma vez ao iniciar.
func (s *Service) BackfillThumbs(ctx context.Context) {
	if s.store == nil {
		return
	}
	for ctx.Err() == nil {
		rows, err := s.db.Query(ctx, `
			SELECT m.id::text, a.storage_key FROM messages m JOIN attachments a ON a.message_id = m.id
			 WHERE m.kind = 'image' AND a.mime_type LIKE 'image/%' AND NOT (m.payload ? 'thumb') LIMIT 100`)
		if err != nil {
			return
		}
		type item struct{ id, key string }
		var batch []item
		for rows.Next() {
			var it item
			if rows.Scan(&it.id, &it.key) == nil {
				batch = append(batch, it)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			return
		}
		for _, it := range batch {
			thumb := "" // vazio = sem miniatura (não tenta de novo)
			if data, err := s.store.Get(ctx, it.key); err == nil {
				thumb, _ = media.Thumb(data)
			}
			_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('thumb', $2::text) WHERE id = $1`, it.id, thumb)
		}
	}
}

// fetchMedia baixa a mídia recebida, valida, guarda e avisa a interface. Roda em segundo
// plano: a mensagem já apareceu (sem arquivo) e ganha o anexo quando ele fica pronto.
// queueHistoricalPDF baixa um PDF do histórico em segundo plano, em fila própria e pequena, respeitando o teto de espaço.
func (s *Service) queueHistoricalPDF(msgID, convID string, m whatsapp.InboundMessage) {
	if s.store == nil || s.media == nil || s.histBudget <= 0 {
		return
	}
	go func() {
		s.histSlots <- struct{}{}
		defer func() { <-s.histSlots }()
		var used int64
		if err := s.db.QueryRow(context.Background(), `SELECT coalesce(sum(size_bytes), 0)::bigint FROM attachments`).Scan(&used); err != nil {
			return
		}
		if size, _ := m.Payload["sizeBytes"].(uint64); used+int64(size) > s.histBudget {
			_, _ = s.db.Exec(context.Background(), `UPDATE messages SET payload = payload || '{"mediaState":"skipped"}'::jsonb WHERE id = $1`, msgID)
			return
		}
		s.fetchMedia(msgID, convID, m)
	}()
}

func (s *Service) fetchMedia(msgID, convID string, m whatsapp.InboundMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	state := "failed"
	defer func() {
		if state != "" {
			_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('mediaState', $2::text) WHERE id = $1`, msgID, state)
		}
		if to, err := s.visibleUsers(ctx, s.db, convID); err == nil {
			s.publishMessage(ctx, EventMessageUpdated, msgID, to)
		}
	}()
	data, err := m.Download(ctx)
	if err != nil {
		return
	}
	name, _ := m.Payload["fileName"].(string)
	declared, _ := m.Payload["mimeType"].(string)
	p, err := s.media.ProcessInbound(ctx, media.Kind(m.Kind), data, name, declared)
	if err != nil {
		var rej *media.RejectError
		if errors.As(err, &rej) {
			state = "blocked"
		}
		return
	}
	key := "attachments/" + uuid.NewString()
	if err := s.store.Put(ctx, key, p.Data); err != nil {
		return
	}
	sum := sha256.Sum256(p.Data)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO attachments (message_id, storage_key, mime_type, size_bytes, sha256, file_name, scan_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (message_id) DO NOTHING`,
		msgID, key, p.MimeType, len(p.Data), hex.EncodeToString(sum[:]), p.FileName, p.ScanStatus); err != nil {
		_ = s.store.Delete(ctx, key)
		return
	}
	if m.Kind == whatsapp.KindImage {
		thumb, _ := media.Thumb(p.Data)
		_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('thumb', $2::text) WHERE id = $1`, msgID, thumb)
	}
	state = ""
}

// AddParticipant inclui uma pessoa (de outra equipe, ou um administrador) no atendimento: ela passa a ver a conversa e a responder,
// sem tirar o responsável. Quem inclui: o responsável, quem já participa, ou administrador/gestor.
func (s *Service) AddParticipant(ctx context.Context, v Viewer, convID, userID string) error {
	if !httpx.ValidUUID(convID) {
		return ErrNotFound
	}
	if !httpx.ValidUUID(userID) {
		return httpx.FieldErrors{"userId": "Escolha uma pessoa."}
	}
	var before userSet
	var changed bool
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var status string
		var assignee *string
		var isPart bool
		err := tx.QueryRow(ctx, `
			SELECT c.status, c.assignee_id::text, EXISTS (SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = c.id AND cp.user_id = $1)
			  FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
			 WHERE c.id = $3 AND `+forViewer+` FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&status, &assignee, &isPart)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return ErrClosed
		}
		if !(v.CanSupervise() || isPart || (assignee != nil && *assignee == v.ID)) {
			return &TakenError{Assignee: ""}
		}
		var active bool
		err = tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, userID).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
			return httpx.FieldErrors{"userId": "Pessoa não encontrada ou desativada."}
		}
		if err != nil {
			return err
		}
		if assignee != nil && *assignee == userID {
			return httpx.FieldErrors{"userId": "Esta pessoa já é a responsável pelo atendimento."}
		}
		if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO conversation_participants (conversation_id, user_id, added_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, convID, userID, v.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		changed = true
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: "conversation.participant_add", TargetType: "conversation", TargetID: convID, Metadata: map[string]any{"userId": userID}})
	})
	if err != nil {
		return err
	}
	if changed {
		s.publishState(ctx, convID, before)
	}
	return nil
}

// RemoveParticipant tira uma pessoa do atendimento (quem participa também pode sair). Quem perde o acesso pela equipe deixa de ver a conversa.
func (s *Service) RemoveParticipant(ctx context.Context, v Viewer, convID, userID string) error {
	if !httpx.ValidUUID(convID) || !httpx.ValidUUID(userID) {
		return ErrNotFound
	}
	var before userSet
	var changed bool
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var assignee *string
		var isPart bool
		err := tx.QueryRow(ctx, `
			SELECT c.assignee_id::text, EXISTS (SELECT 1 FROM conversation_participants cp WHERE cp.conversation_id = c.id AND cp.user_id = $1)
			  FROM conversations c LEFT JOIN segments s ON s.id = c.segment_id
			 WHERE c.id = $3 AND `+forViewer+` FOR UPDATE OF c`, v.ID, v.Admin, convID).Scan(&assignee, &isPart)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !(v.CanSupervise() || isPart || userID == v.ID || (assignee != nil && *assignee == v.ID)) {
			return &TakenError{Assignee: ""}
		}
		if before, err = s.visibleUsers(ctx, tx, convID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM conversation_participants WHERE conversation_id = $1 AND user_id = $2`, convID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		changed = true
		return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: "conversation.participant_remove", TargetType: "conversation", TargetID: convID, Metadata: map[string]any{"userId": userID}})
	})
	if err != nil {
		return err
	}
	if changed {
		s.publishState(ctx, convID, before)
	}
	return nil
}
