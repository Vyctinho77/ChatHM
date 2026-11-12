// Package internalchat é o chat interno do escritório: um grupo, no estilo do WhatsApp, para a equipe tirar dúvidas.
//
// Regras:
//   - toda pessoa autenticada lê e escreve; nada daqui passa pelo WhatsApp, pelo MCP ou pelas sugestões da IA;
//   - cada mensagem mostra quem escreveu (nome, perfil e situação), lidos na hora: se o perfil mudar, a tela acompanha;
//   - a pessoa autora edita (até 24 horas) e apaga as próprias mensagens; gestor e administrador apagam as de qualquer
//     pessoa, e isso é auditado (sem o conteúdo);
//   - cada pessoa tem um marcador de leitura próprio, que alimenta o selo de não lidas;
//   - as mensagens são serializadas no mesmo formato "achatado" do chat principal, para o front reaproveitar a bolha;
//     o que depende de quem olha (se a mensagem é minha) o cliente decide comparando authorId.
package internalchat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
	"github.com/vyctinho77/chathm/backend/internal/realtime"
	"github.com/vyctinho77/chathm/backend/internal/storage"
)

var (
	ErrNotFound    = errors.New("internalchat: não encontrada")
	ErrForbidden   = errors.New("internalchat: sem permissão")
	ErrUnavailable = errors.New("internalchat: anexos indisponíveis")
	ErrIDConflict  = errors.New("internalchat: identificador de mensagem em uso")
)

// Eventos em tempo real. As mensagens vão a todas as sessões; o marcador de leitura só às abas da própria pessoa.
const (
	EventMessageCreated = "internal.message.created"
	EventMessageUpdated = "internal.message.updated" // edição e exclusão
	EventRead           = "internal.read"
)

const (
	maxTextRunes    = 4000
	maxCaptionRunes = 1024
	maxQuoteRunes   = 200
	editWindow      = 24 * time.Hour
	defaultPage     = 50
	maxPage         = 100
)

// Publisher é satisfeito por *realtime.Hub.
type Publisher interface {
	Publish(typ string, data any, filter realtime.Filter)
}

// Viewer é quem faz a requisição.
type Viewer struct {
	ID         string
	Admin      bool
	Supervisor bool // gestor
}

// CanModerate: administrador ou gestor apagam mensagens de outras pessoas.
func (v Viewer) CanModerate() bool { return v.Admin || v.Supervisor }

type Service struct {
	db    *pgxpool.Pool
	pub   Publisher
	store storage.Store
	media *media.Service

	roomMu sync.Mutex
	room   *Room
}

func NewService(db *pgxpool.Pool, pub Publisher) *Service { return &Service{db: db, pub: pub} }

// SetMedia liga o armazenamento privado e o processamento de anexos.
func (s *Service) SetMedia(st storage.Store, m *media.Service) { s.store, s.media = st, m }

// MaxUpload é o limite de tamanho de um anexo (0 sem o serviço de mídia).
func (s *Service) MaxUpload() int64 {
	if s.media == nil {
		return 0
	}
	return s.media.Max
}

// Message é serializada "achatada", como no chat principal.
type Message map[string]any

// Room é a sala do escritório com o estado de leitura de quem pergunta.
type Room struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Unread      int    `json:"unread"`
	LastReadSeq int64  `json:"lastReadSeq"`
	LastSeq     int64  `json:"lastSeq"`
}

func (s *Service) office(ctx context.Context) (Room, error) {
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	if s.room != nil {
		return *s.room, nil
	}
	var r Room
	if err := s.db.QueryRow(ctx, `SELECT id, name FROM internal_rooms WHERE kind = 'office'`).Scan(&r.ID, &r.Name); err != nil {
		return Room{}, err
	}
	s.room = &r
	return r, nil
}

// Room devolve a sala e quantas mensagens de outras pessoas a pessoa ainda não leu. Quem entra agora no escritório
// começa sem não lidas: o histórico anterior não vira selo.
func (s *Service) Room(ctx context.Context, v Viewer) (Room, error) {
	r, err := s.office(ctx)
	if err != nil {
		return Room{}, err
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO internal_reads (room_id, user_id, last_read_seq)
		SELECT $1, $2, coalesce(max(seq), 0) FROM internal_messages WHERE room_id = $1
		ON CONFLICT DO NOTHING`, r.ID, v.ID); err != nil {
		return Room{}, err
	}
	err = s.db.QueryRow(ctx, `
		SELECT rd.last_read_seq,
		       coalesce((SELECT max(seq) FROM internal_messages WHERE room_id = $1), 0),
		       (SELECT count(*) FROM internal_messages m
		         WHERE m.room_id = $1 AND m.deleted_at IS NULL AND m.author_id <> $2 AND m.seq > rd.last_read_seq)
		  FROM internal_reads rd WHERE rd.room_id = $1 AND rd.user_id = $2`, r.ID, v.ID).Scan(&r.LastReadSeq, &r.LastSeq, &r.Unread)
	return r, err
}

const messageCols = `m.id, m.room_id, m.seq, m.kind, m.body, m.payload, m.created_at, m.edited_at, m.deleted_at,
	m.author_id, u.name, u.role, u.is_active,
	(SELECT a.id::text FROM internal_attachments a WHERE a.message_id = m.id),
	q.id, q.kind, q.body, q.deleted_at, q.author_id, qu.name`

const messageFrom = `
	FROM internal_messages m
	JOIN users u ON u.id = m.author_id
	LEFT JOIN internal_messages q ON q.id = m.quoted_id
	LEFT JOIN users qu ON qu.id = q.author_id`

func scanMessage(row pgx.Row) (Message, error) {
	var (
		id, roomID, kind, body, authorID, authorName, authorRole string
		seq                                                      int64
		payload                                                  []byte
		createdAt                                                time.Time
		editedAt, deletedAt                                      *time.Time
		active                                                   bool
		attachmentID                                             *string
		qID, qKind, qBody, qAuthorID, qAuthorName                *string
		qDeletedAt                                               *time.Time
	)
	if err := row.Scan(&id, &roomID, &seq, &kind, &body, &payload, &createdAt, &editedAt, &deletedAt,
		&authorID, &authorName, &authorRole, &active, &attachmentID,
		&qID, &qKind, &qBody, &qDeletedAt, &qAuthorID, &qAuthorName); err != nil {
		return nil, err
	}
	m := Message{}
	_ = json.Unmarshal(payload, &m)
	m["id"], m["conversationId"], m["seq"], m["kind"] = id, roomID, seq, kind
	m["sentAt"] = createdAt.UTC().Format(time.RFC3339Nano)
	m["authorId"], m["authorName"], m["authorRole"], m["authorActive"] = authorID, authorName, authorRole, active
	if deletedAt != nil {
		// Apagada: o conteúdo some, a marca fica.
		return Message{"id": id, "conversationId": roomID, "seq": seq, "kind": kind, "sentAt": m["sentAt"], "deleted": true, "text": "",
			"authorId": authorID, "authorName": authorName, "authorRole": authorRole, "authorActive": active}, nil
	}
	if editedAt != nil {
		m["editedAt"] = editedAt.UTC().Format(time.RFC3339Nano)
	}
	if attachmentID != nil {
		m["url"] = "/api/v1/internal-chat/attachments/" + *attachmentID
	}
	switch kind {
	case "text":
		m["text"] = body
	case "image", "document":
		if body != "" {
			m["caption"] = body
		}
	}
	if qID != nil {
		q := map[string]any{"id": *qID}
		if qDeletedAt != nil {
			q["deleted"] = true
		} else {
			q["kind"], q["authorId"], q["author"] = *qKind, *qAuthorID, *qAuthorName
			text := ""
			if qBody != nil {
				text = *qBody
			}
			if r := []rune(text); len(r) > maxQuoteRunes {
				text = string(r[:maxQuoteRunes]) + "…"
			}
			q["text"] = text
		}
		m["quoted"] = q
	}
	return m, nil
}

func (s *Service) message(ctx context.Context, db database.DBTX, id string) (Message, error) {
	m, err := scanMessage(db.QueryRow(ctx, `SELECT `+messageCols+messageFrom+` WHERE m.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// Messages devolve até `limit` mensagens em ordem cronológica, as mais recentes (ou as anteriores a `before`, um seq).
// hasMore diz se existem mensagens ainda mais antigas.
func (s *Service) Messages(ctx context.Context, before int64, limit int) ([]Message, bool, error) {
	r, err := s.office(ctx)
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		limit = defaultPage
	}
	if limit > maxPage {
		limit = maxPage
	}
	rows, err := s.db.Query(ctx, `SELECT `+messageCols+messageFrom+`
		 WHERE m.room_id = $1 AND ($2 <= 0 OR m.seq < $2)
		 ORDER BY m.seq DESC LIMIT $3`, r.ID, before, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var desc []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, false, err
		}
		desc = append(desc, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(desc) > limit
	if hasMore {
		desc = desc[:limit]
	}
	out := make([]Message, len(desc))
	for i, m := range desc {
		out[len(desc)-1-i] = m
	}
	return out, hasMore, nil
}

type SendInput struct {
	ID   string // opcional; torna o envio idempotente
	Kind string // text (padrão) | image | audio | document | location
	Text string // texto, ou legenda quando há anexo

	Att      *media.Processed // image | audio | document (já validado)
	HD       bool
	Waveform []float64

	Lat, Lng      float64
	Name, Address string

	ReplyTo string // id da mensagem respondida
}

// Send grava e distribui uma mensagem. created=false quando repete um envio anterior (idempotente).
func (s *Service) Send(ctx context.Context, v Viewer, in SendInput) (Message, bool, error) {
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
	case "image", "audio", "document":
		if in.Att == nil || s.store == nil {
			return nil, false, ErrUnavailable
		}
		if utf8.RuneCountInString(text) > maxCaptionRunes || (kind == "audio" && text != "") {
			return nil, false, httpx.FieldErrors{"text": fmt.Sprintf("A legenda pode ter até %d caracteres.", maxCaptionRunes)}
		}
		payload["mimeType"], payload["sizeBytes"] = in.Att.MimeType, len(in.Att.Data)
		switch kind {
		case "image":
			payload["width"], payload["height"], payload["hd"] = in.Att.Width, in.Att.Height, in.HD
			if thumb, ok := media.Thumb(in.Att.Data); ok {
				payload["thumb"] = thumb
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
	if in.ReplyTo != "" && !httpx.ValidUUID(in.ReplyTo) {
		return nil, false, httpx.FieldErrors{"replyTo": "Mensagem citada inválida."}
	}
	room, err := s.office(ctx)
	if err != nil {
		return nil, false, err
	}
	pj, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}

	attID := uuid.NewString()
	var attKey string
	storedNew := false
	if in.Att != nil && kind != "text" && kind != "location" {
		// O binário vai antes da transação; se a mensagem não nascer, é apagado abaixo.
		attKey = "internal/" + attID
		if err := s.store.Put(ctx, attKey, in.Att.Data); err != nil {
			return nil, false, err
		}
		storedNew = true
		defer func() {
			if storedNew {
				_ = s.store.Delete(context.WithoutCancel(ctx), attKey)
			}
		}()
	}

	created := false
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var existingAuthor string
		switch err := tx.QueryRow(ctx, `SELECT author_id::text FROM internal_messages WHERE id = $1`, msgID).Scan(&existingAuthor); {
		case err == nil:
			if existingAuthor != v.ID {
				return ErrIDConflict
			}
			return nil // repetição do mesmo envio
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		var quoted *string
		if in.ReplyTo != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT true FROM internal_messages WHERE id = $1 AND room_id = $2`, in.ReplyTo, room.ID).Scan(&ok); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return httpx.FieldErrors{"replyTo": "A mensagem citada não existe mais."}
				}
				return err
			}
			quoted = &in.ReplyTo
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO internal_messages (id, room_id, author_id, kind, body, payload, quoted_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, msgID, room.ID, v.ID, kind, text, pj, quoted); err != nil {
			return err
		}
		if storedNew {
			sum := sha256.Sum256(in.Att.Data)
			if _, err := tx.Exec(ctx, `
				INSERT INTO internal_attachments (id, message_id, storage_key, mime_type, size_bytes, sha256, file_name, scan_status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				attID, msgID, attKey, in.Att.MimeType, len(in.Att.Data), hex.EncodeToString(sum[:]), in.Att.FileName, in.Att.ScanStatus); err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if created {
		storedNew = false // a mensagem existe: o objeto fica
	}
	m, err := s.message(ctx, s.db, msgID)
	if err != nil {
		return nil, false, err
	}
	if created && s.pub != nil {
		s.pub.Publish(EventMessageCreated, m, nil)
	}
	return m, created, nil
}

// Edit altera o texto de uma mensagem própria (até 24 horas depois do envio).
func (s *Service) Edit(ctx context.Context, v Viewer, id, text string) (Message, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxTextRunes {
		return nil, httpx.FieldErrors{"text": fmt.Sprintf("Digite uma mensagem de até %d caracteres.", maxTextRunes)}
	}
	if !httpx.ValidUUID(id) {
		return nil, ErrNotFound
	}
	var (
		author, kind, body string
		createdAt          time.Time
		deletedAt          *time.Time
	)
	err := s.db.QueryRow(ctx, `SELECT author_id::text, kind, body, created_at, deleted_at FROM internal_messages WHERE id = $1`, id).
		Scan(&author, &kind, &body, &createdAt, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	switch {
	case author != v.ID:
		return nil, ErrForbidden
	case kind != "text" || deletedAt != nil:
		return nil, httpx.FieldErrors{"text": "Esta mensagem não pode ser editada."}
	case time.Since(createdAt) > editWindow:
		return nil, httpx.FieldErrors{"text": "Só dá para editar até 24 horas depois do envio."}
	}
	if text != body {
		if _, err := s.db.Exec(ctx, `UPDATE internal_messages SET body = $2, edited_at = now() WHERE id = $1 AND deleted_at IS NULL`, id, text); err != nil {
			return nil, err
		}
	}
	m, err := s.message(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if text != body && s.pub != nil {
		s.pub.Publish(EventMessageUpdated, m, nil)
	}
	return m, nil
}

// Delete apaga para todos: o conteúdo e o anexo somem, a marca "apagada" fica. A autora apaga a própria; gestor e
// administrador apagam a de qualquer pessoa (auditado, sem o conteúdo).
func (s *Service) Delete(ctx context.Context, v Viewer, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	var attKey *string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		var author string
		var deletedAt *time.Time
		err := tx.QueryRow(ctx, `SELECT author_id::text, deleted_at FROM internal_messages WHERE id = $1 FOR UPDATE`, id).Scan(&author, &deletedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if author != v.ID && !v.CanModerate() {
			return ErrForbidden
		}
		if deletedAt != nil {
			return nil
		}
		if err := tx.QueryRow(ctx, `DELETE FROM internal_attachments WHERE message_id = $1 RETURNING storage_key`, id).Scan(&attKey); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE internal_messages SET deleted_at = now(), deleted_by = $2, body = '', payload = '{}' WHERE id = $1`, id, v.ID); err != nil {
			return err
		}
		if author != v.ID {
			return audit.Record(ctx, tx, audit.Entry{ActorID: v.ID, Action: audit.InternalMessageModerate, TargetType: "internal_message", TargetID: id,
				Metadata: map[string]any{"authorId": author}})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if attKey != nil && s.store != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), *attKey)
	}
	if s.pub != nil {
		if m, err := s.message(ctx, s.db, id); err == nil {
			s.pub.Publish(EventMessageUpdated, m, nil)
		}
	}
	return nil
}

// MarkRead marca tudo o que existe agora como lido e avisa as outras abas da própria pessoa.
func (s *Service) MarkRead(ctx context.Context, v Viewer) (Room, error) {
	r, err := s.office(ctx)
	if err != nil {
		return Room{}, err
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO internal_reads (room_id, user_id, last_read_seq)
		VALUES ($1, $2, (SELECT coalesce(max(seq), 0) FROM internal_messages WHERE room_id = $1))
		ON CONFLICT (room_id, user_id) DO UPDATE
		   SET last_read_seq = greatest(internal_reads.last_read_seq, EXCLUDED.last_read_seq), updated_at = now()`, r.ID, v.ID); err != nil {
		return Room{}, err
	}
	room, err := s.Room(ctx, v)
	if err == nil && s.pub != nil {
		uid := v.ID
		s.pub.Publish(EventRead, map[string]any{"unread": room.Unread, "lastReadSeq": room.LastReadSeq}, func(c *realtime.Client) bool { return c.UserID == uid })
	}
	return room, err
}

// AttachmentFile é o arquivo de um anexo.
type AttachmentFile struct {
	Data               []byte
	MimeType, FileName string
	SHA256             string
}

// Attachment devolve o arquivo; qualquer pessoa autenticada do escritório pode ler, mensagem apagada não.
func (s *Service) Attachment(ctx context.Context, id string) (AttachmentFile, error) {
	if !httpx.ValidUUID(id) || s.store == nil {
		return AttachmentFile{}, ErrNotFound
	}
	var key, mimeType, name, sha string
	err := s.db.QueryRow(ctx, `
		SELECT a.storage_key, a.mime_type, a.file_name, a.sha256
		  FROM internal_attachments a JOIN internal_messages m ON m.id = a.message_id
		 WHERE a.id = $1 AND m.deleted_at IS NULL`, id).Scan(&key, &mimeType, &name, &sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttachmentFile{}, ErrNotFound
	}
	if err != nil {
		return AttachmentFile{}, err
	}
	data, err := s.store.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return AttachmentFile{}, ErrNotFound
	}
	return AttachmentFile{Data: data, MimeType: mimeType, FileName: name, SHA256: sha}, err
}
