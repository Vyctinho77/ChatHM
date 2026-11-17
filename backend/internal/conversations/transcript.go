package conversations

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/transcribe"
)

// BuildProbe: constante só para medir o tempo de uma construção incremental (commit desfeito em seguida).
const BuildProbe = 1

// Transcrição das mensagens de voz recebidas. Roda em segundo plano, um áudio por vez, e nunca atrasa a mensagem: ela aparece na hora e o
// texto chega depois (o app é avisado por tempo real). O estado fica no payload: transcriptState = pending | done | empty | failed | toolong.

func (s *Service) startTranscription(msgID, convID string, audio []byte, durationSec int) {
	if s.transcriber == nil {
		return
	}
	if durationSec > s.transcriber.MaxSeconds() {
		s.setTranscript(context.Background(), msgID, convID, "toolong", "")
		return
	}
	s.setTranscript(context.Background(), msgID, convID, "pending", "")
	go s.transcribeNow(msgID, convID, audio, durationSec)
}

func (s *Service) transcribeNow(msgID, convID string, audio []byte, durationSec int) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute) // inclui a espera na fila
	defer cancel()
	text, err := s.transcriber.Transcribe(ctx, audio, durationSec)
	switch {
	case errors.Is(err, transcribe.ErrTooLong):
		s.setTranscript(ctx, msgID, convID, "toolong", "")
	case err != nil:
		slog.Warn("transcrição falhou", "message", msgID, "err", err)
		s.setTranscript(ctx, msgID, convID, "failed", "")
	case text == "":
		s.setTranscript(ctx, msgID, convID, "empty", "")
	default:
		s.setTranscript(ctx, msgID, convID, "done", text)
	}
}

// setTranscript grava o estado e o texto. Não avisa as telas: a equipe não vê a transcrição (só os agentes de IA, pelo MCP).
func (s *Service) setTranscript(ctx context.Context, msgID, _, state, text string) {
	if state == "done" {
		_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('transcriptState', $2::text, 'transcript', $3::text) WHERE id = $1`, msgID, state, text)
	} else {
		_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('transcriptState', $2::text) WHERE id = $1`, msgID, state)
	}
}

// Transcript é o que se sabe da transcrição de uma mensagem de voz: State = pending | done | empty | failed | toolong.
type Transcript struct {
	State string
	Text  string
}

// Transcripts devolve a transcrição das mensagens de voz indicadas (só as que têm estado). Quem chama já conferiu a visibilidade das mensagens.
func (s *Service) Transcripts(ctx context.Context, ids []string) (map[string]Transcript, error) {
	out := map[string]Transcript{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `SELECT id::text, payload->>'transcriptState', coalesce(payload->>'transcript', '') FROM messages
		WHERE id = ANY($1::uuid[]) AND payload ? 'transcriptState'`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var t Transcript
		if rows.Scan(&id, &t.State, &t.Text) == nil {
			out[id] = t
		}
	}
	return out, rows.Err()
}

// BackfillTranscripts transcreve, ao iniciar, os áudios recebidos nos últimos 7 dias que ainda não têm texto (e os que ficaram "pending" por um
// reinício). Um por vez, em segundo plano. Sem transcritor, não faz nada.
func (s *Service) BackfillTranscripts(ctx context.Context) {
	if s.transcriber == nil || s.store == nil {
		return
	}
	// Espera um pouco depois de iniciar: o app atende primeiro, e o trabalho pesado começa com a máquina já calma.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	rows, err := s.db.Query(ctx, `
		SELECT m.id::text, m.conversation_id::text, a.storage_key, coalesce((m.payload->>'durationSec')::int, 0)
		  FROM messages m JOIN attachments a ON a.message_id = m.id
		 WHERE m.kind = 'audio' AND m.direction = 'in' AND m.deleted_at IS NULL AND m.sent_at > now() - interval '7 days'
		   AND coalesce(m.payload->>'transcriptState', 'pending') = 'pending'
		 ORDER BY m.sent_at DESC LIMIT 200`)
	if err != nil {
		return
	}
	type item struct {
		id, conv, key string
		dur           int
	}
	var todo []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.conv, &it.key, &it.dur) == nil {
			todo = append(todo, it)
		}
	}
	rows.Close()
	for _, it := range todo {
		if ctx.Err() != nil {
			return
		}
		data, err := s.store.Get(ctx, it.key)
		if err != nil {
			continue
		}
		s.transcribeNow(it.id, it.conv, data, it.dur)
	}
}
