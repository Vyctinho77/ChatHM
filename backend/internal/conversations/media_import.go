package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// Importação da mídia do histórico do WhatsApp e das que falharam ao chegar.
//
// O histórico traz só a referência de cada arquivo (endereço, chave e hashes). O servidor a guarda cifrada (media_refs) e, em segundo plano,
// baixa um arquivo a cada poucos segundos, dos mais recentes aos mais antigos, dentro do teto de espaço. O WhatsApp apaga os arquivos dos seus
// servidores depois de um tempo; para esses, o servidor pede ao CELULAR que envie o arquivo de novo (um pedido por vez, com limite por hora) e
// espera a resposta por até 24 horas. Se o celular também não tem mais o arquivo, a mensagem fica marcada como indisponível.

const (
	importEvery    = 3 * time.Second
	retryWindow    = 24 * time.Hour
	retryPerHour   = 120 // pedidos de reenvio por hora: devagar, para não parecer abuso ao WhatsApp
	maxImportTries = 4
)

// Limites por arquivo (o que passar disso fica só como metadados).
var importMaxBytes = map[string]int64{"image": 12 << 20, "audio": 10 << 20, "document": 15 << 20, "sticker": 2 << 20}

// SetBox liga a cifra das referências de mídia (têm a chave do arquivo). Sem ela, o histórico não guarda referências.
func (s *Service) SetBox(b *secretbox.Box) { s.box = b }

// saveMediaRef guarda a referência de uma mensagem (cifrada) para baixar depois. delay adia a primeira tentativa.
func (s *Service) saveMediaRef(ctx context.Context, msgID string, ref *whatsapp.MediaRef, delay time.Duration) {
	if s.box == nil || ref == nil || len(ref.Proto) == 0 || msgID == "" {
		return
	}
	enc, err := s.box.Seal(ref.Proto, []byte(msgID))
	if err != nil {
		return
	}
	_, _ = s.db.Exec(ctx, `
		INSERT INTO media_refs (message_id, kind, ref_enc, next_attempt_at) VALUES ($1, $2, $3, now() + $4::interval)
		ON CONFLICT (message_id) DO NOTHING`, msgID, string(ref.Kind), enc, delay.String())
}

// registerDupRef: o histórico foi entregue de novo (nova sincronização) e a mensagem já existia SEM o arquivo: guarda a referência agora.
// É assim que uma nova sincronização recupera as referências das mensagens antigas.
func (s *Service) registerDupRef(ctx context.Context, m whatsapp.InboundMessage) {
	if s.box == nil || m.MediaRef == nil {
		return
	}
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT m.id::text FROM messages m
		 WHERE `+sameMessage("m.")+` AND m.deleted_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = m.id)
		   AND coalesce(m.payload->>'mediaState', '') <> 'blocked'`, m.ChatJID, m.ExternalID).Scan(&id)
	if err != nil {
		return
	}
	s.saveMediaRef(ctx, id, m.MediaRef, 0)
}

type refRow struct {
	msgID, convID, kind, chatJID, extID, dir string
	enc                                      []byte
	attempts                                 int
	payload                                  map[string]any
}

// RunMediaImporter baixa, em segundo plano, a mídia guardada por referência. Roda até o contexto acabar.
func (s *Service) RunMediaImporter(ctx context.Context) {
	if s.box == nil || s.store == nil || s.media == nil || s.sender == nil {
		return
	}
	t := time.NewTicker(importEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.importTick(ctx)
		}
	}
}

func (s *Service) fetcher() (whatsapp.MediaFetcher, bool) {
	if s.sender == nil || !s.sender.Connected() {
		return nil, false
	}
	f, ok := s.sender.(whatsapp.MediaFetcher)
	return f, ok
}

// ImportMediaOnce faz uma rodada do importador (uma mídia). RunMediaImporter a repete; os testes a chamam direto.
func (s *Service) ImportMediaOnce(ctx context.Context) { s.importTick(ctx) }

func (s *Service) importTick(ctx context.Context) {
	f, ok := s.fetcher()
	if !ok {
		return
	}
	s.expireRetries(ctx)
	s.importNext(ctx, f)
}

// expireRetries: o celular não respondeu ao pedido de reenvio em 24 horas.
func (s *Service) expireRetries(ctx context.Context) {
	_, _ = s.db.Exec(ctx, `
		WITH x AS (UPDATE media_refs SET status = 'unavailable', last_error = 'o celular não respondeu', updated_at = now()
		            WHERE status = 'retry_requested' AND retry_requested_at < now() - $1::interval RETURNING message_id)
		UPDATE messages SET payload = payload || '{"mediaState":"expired"}'::jsonb FROM x WHERE messages.id = x.message_id`, retryWindow.String())
}

func (s *Service) loadRef(ctx context.Context, where string, args ...any) (refRow, error) {
	var r refRow
	var payload []byte
	err := s.db.QueryRow(ctx, `
		SELECT r.message_id::text, m.conversation_id::text, r.kind, r.ref_enc, r.attempts, m.chat_jid, m.external_id, m.direction, m.payload
		  FROM media_refs r JOIN messages m ON m.id = r.message_id WHERE `+where, args...).
		Scan(&r.msgID, &r.convID, &r.kind, &r.enc, &r.attempts, &r.chatJID, &r.extID, &r.dir, &payload)
	if err != nil {
		return r, err
	}
	r.payload = map[string]any{}
	_ = json.Unmarshal(payload, &r.payload)
	return r, nil
}

func (s *Service) openRef(r refRow) (whatsapp.MediaRef, bool) {
	b, err := s.box.Open(r.enc, []byte(r.msgID))
	if err != nil {
		return whatsapp.MediaRef{}, false
	}
	return whatsapp.MediaRef{Kind: whatsapp.Kind(r.kind), Proto: b}, true
}

// importNext tenta o próximo arquivo devido: o mais recente primeiro.
func (s *Service) importNext(ctx context.Context, f whatsapp.MediaFetcher) {
	r, err := s.loadRef(ctx, `r.status = 'pending' AND r.next_attempt_at <= now() ORDER BY m.sent_at DESC LIMIT 1`)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return
	}
	var has bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE message_id = $1)`, r.msgID).Scan(&has); err == nil && has {
		s.finishRef(ctx, r, "done", "", "")
		return
	}
	// Tamanho e teto de espaço.
	size, _ := r.payload["sizeBytes"].(float64)
	if max := importMaxBytes[r.kind]; max > 0 && int64(size) > max {
		s.finishRef(ctx, r, "skipped", "arquivo grande demais para importar", "")
		return
	}
	var used int64
	if err := s.db.QueryRow(ctx, `SELECT coalesce(sum(size_bytes), 0)::bigint FROM attachments`).Scan(&used); err == nil && s.histBudget > 0 && used+int64(size) > s.histBudget {
		s.finishRef(ctx, r, "skipped", "teto de espaço atingido", "")
		return
	}
	ref, ok := s.openRef(r)
	if !ok {
		s.finishRef(ctx, r, "unavailable", "referência ilegível", "expired")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	data, err := f.FetchMedia(cctx, ref)
	switch {
	case err == nil:
		s.storeAndFinish(ctx, r, data)
	case errors.Is(err, whatsapp.ErrMediaGone):
		s.requestRetry(ctx, f, r, ref)
	default:
		s.backoff(ctx, r, err)
	}
}

// storeAndFinish guarda o arquivo baixado e fecha a referência.
func (s *Service) storeAndFinish(ctx context.Context, r refRow, data []byte) {
	name, _ := r.payload["fileName"].(string)
	declared, _ := r.payload["mimeType"].(string)
	switch s.storeFetched(ctx, r.msgID, r.convID, whatsapp.Kind(r.kind), data, name, declared) {
	case "":
		s.finishRef(ctx, r, "done", "", "ok")
	case "blocked":
		s.finishRef(ctx, r, "unavailable", "arquivo recusado pela validação de segurança", "blocked")
	default:
		s.backoff(ctx, r, errors.New("não foi possível guardar o arquivo"))
	}
}

// requestRetry pede ao celular que envie o arquivo de novo (o WhatsApp já o apagou do servidor). Um pedido só por mensagem.
func (s *Service) requestRetry(ctx context.Context, f whatsapp.MediaFetcher, r refRow, ref whatsapp.MediaRef) {
	if !s.retryAllowed() {
		_, _ = s.db.Exec(ctx, `UPDATE media_refs SET next_attempt_at = now() + interval '10 minutes', updated_at = now() WHERE message_id = $1`, r.msgID)
		return
	}
	sender, _ := r.payload["senderJid"].(string)
	err := f.RequestMediaRetry(ctx, ref, whatsapp.MediaMessage{
		ExternalID: r.extID, ChatJID: r.chatJID, SenderJID: sender, FromMe: r.dir == "out", IsGroup: strings.HasSuffix(r.chatJID, "@g.us"),
	})
	if err != nil {
		s.backoff(ctx, r, err)
		return
	}
	_, _ = s.db.Exec(ctx, `UPDATE media_refs SET status = 'retry_requested', retry_requested_at = now(), updated_at = now() WHERE message_id = $1`, r.msgID)
	s.setMediaState(ctx, r, "retrying")
}

// retryAllowed: no máximo retryPerHour pedidos de reenvio por hora.
func (s *Service) retryAllowed() bool {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	cut := time.Now().Add(-time.Hour)
	kept := s.retrySent[:0]
	for _, t := range s.retrySent {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	s.retrySent = kept
	if len(s.retrySent) >= retryPerHour {
		return false
	}
	s.retrySent = append(s.retrySent, time.Now())
	return true
}

var backoffs = []time.Duration{5 * time.Minute, 30 * time.Minute, 3 * time.Hour, 12 * time.Hour}

// backoff: falha passageira (rede, WhatsApp fora do ar): tenta de novo mais tarde; depois de várias, desiste.
func (s *Service) backoff(ctx context.Context, r refRow, cause error) {
	if r.attempts+1 >= maxImportTries {
		s.finishRef(ctx, r, "unavailable", short(cause), "expired")
		return
	}
	_, _ = s.db.Exec(ctx, `UPDATE media_refs SET attempts = attempts + 1, next_attempt_at = now() + $2::interval, last_error = $3, updated_at = now() WHERE message_id = $1`,
		r.msgID, backoffs[r.attempts%len(backoffs)].String(), short(cause))
}

func short(err error) string {
	s := err.Error()
	if len(s) > 190 {
		s = s[:190]
	}
	return s
}

// finishRef fecha a referência. mediaState: "ok" limpa o estado da mensagem; "expired"/"blocked" o marca; "" não mexe.
func (s *Service) finishRef(ctx context.Context, r refRow, status, reason, mediaState string) {
	_, _ = s.db.Exec(ctx, `UPDATE media_refs SET status = $2, last_error = $3, updated_at = now() WHERE message_id = $1`, r.msgID, status, reason)
	switch mediaState {
	case "ok":
		_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload - 'mediaState' WHERE id = $1`, r.msgID)
		s.announce(ctx, r)
	case "expired", "blocked":
		s.setMediaState(ctx, r, mediaState)
	}
}

func (s *Service) setMediaState(ctx context.Context, r refRow, state string) {
	_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('mediaState', $2::text) WHERE id = $1`, r.msgID, state)
	s.announce(ctx, r)
}

// announce avisa as telas (um arquivo a cada poucos segundos: não enche o canal como o histórico em rajada).
func (s *Service) announce(ctx context.Context, r refRow) {
	if to, err := s.visibleUsers(ctx, s.db, r.convID); err == nil {
		s.publishMessage(ctx, EventMessageUpdated, r.msgID, to)
	}
}

// OnMediaRetry recebe a resposta do celular a um pedido de reenvio: baixa o arquivo ou marca como indisponível.
func (s *Service) OnMediaRetry(ev whatsapp.MediaRetryEvent) {
	if s.box == nil || s.store == nil || s.media == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := s.loadRef(ctx, sameMessage("m.")+` AND r.status = 'retry_requested'`, ev.ChatJID, ev.MessageID)
	if err != nil {
		return // não foi um pedido nosso (ou já tratado)
	}
	ref, ok := s.openRef(r)
	if !ok {
		s.finishRef(ctx, r, "unavailable", "referência ilegível", "expired")
		return
	}
	data, err := ev.Complete(ctx, ref)
	switch {
	case err == nil:
		s.storeAndFinish(ctx, r, data)
	case errors.Is(err, whatsapp.ErrMediaNotOnPhone):
		s.finishRef(ctx, r, "unavailable", "o celular não tem mais o arquivo", "expired")
	default:
		slog.Warn("reenvio de mídia falhou", "message", r.msgID, "err", err)
		s.finishRef(ctx, r, "unavailable", short(err), "expired")
	}
}

var _ = sync.Mutex{}
