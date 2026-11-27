package conversations

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/storage"
)

// Prévia dos PDFs no balão (o alto da primeira página) e número de páginas. A prévia é criada na primeira vez que alguém a pede e fica
// guardada ao lado do original (".preview"), como a das fotos; as próximas leituras não rodam o poppler de novo.

// failedPreviews lembra os anexos que não deram prévia (PDF com senha, quebrado, sem poppler), para não tentar a cada rolagem da tela.
var failedPreviews = struct {
	sync.Mutex
	ids map[string]time.Time
}{ids: map[string]time.Time{}}

func previewFailedRecently(id string) bool {
	failedPreviews.Lock()
	defer failedPreviews.Unlock()
	at, ok := failedPreviews.ids[id]
	return ok && time.Since(at) < time.Hour
}

func rememberPreviewFailure(id string) {
	failedPreviews.Lock()
	defer failedPreviews.Unlock()
	if len(failedPreviews.ids) > 5000 {
		failedPreviews.ids = map[string]time.Time{} // lista só de apoio: recomeça se encher
	}
	failedPreviews.ids[id] = time.Now()
}

// pdfPreview devolve a prévia (JPEG) do PDF; sem prévia possível, ErrNotFound (a tela esconde o espaço da prévia).
func (s *Service) pdfPreview(ctx context.Context, m attachmentMeta, id string) (AttachmentFile, error) {
	pkey := m.key + ".preview"
	if data, err := s.store.Get(ctx, pkey); err == nil {
		return AttachmentFile{Data: data, MimeType: "image/jpeg", FileName: m.name, SHA256: m.sha + "-p", Preview: true}, nil
	}
	if previewFailedRecently(id) {
		return AttachmentFile{}, ErrNotFound
	}
	orig, err := s.store.Get(ctx, m.key)
	if errors.Is(err, storage.ErrNotFound) {
		return AttachmentFile{}, ErrNotFound
	}
	if err != nil {
		return AttachmentFile{}, err
	}
	jpg, pages, err := media.PDFPreview(ctx, orig)
	if err != nil {
		if ctx.Err() == nil {
			rememberPreviewFailure(id)
		}
		return AttachmentFile{}, ErrNotFound
	}
	_ = s.store.Put(ctx, pkey, jpg) // se não gravar, só perde o ganho da próxima vez
	if pages > 0 {
		s.setPDFPagesByAttachment(ctx, id, pages)
	}
	return AttachmentFile{Data: jpg, MimeType: "image/jpeg", FileName: m.name, SHA256: m.sha + "-p", Preview: true}, nil
}

// recordPDFPages grava o número de páginas na mensagem (se ela ainda não tem), a partir do arquivo.
func (s *Service) recordPDFPages(ctx context.Context, msgID string, data []byte) {
	n := media.PDFPages(ctx, data)
	if n <= 0 {
		return
	}
	_, _ = s.db.Exec(ctx, `UPDATE messages SET payload = payload || jsonb_build_object('pages', $2::int) WHERE id = $1 AND NOT (payload ? 'pages')`, msgID, n)
}

func (s *Service) setPDFPagesByAttachment(ctx context.Context, attachmentID string, pages int) {
	_, _ = s.db.Exec(ctx, `
		UPDATE messages SET payload = payload || jsonb_build_object('pages', $2::int)
		 WHERE id = (SELECT message_id FROM attachments WHERE id = $1) AND NOT (payload ? 'pages')`, attachmentID, pages)
}

// BackfillPDFPages conta as páginas dos PDFs que ainda não têm o número (os recebidos antes deste recurso). Roda uma vez ao iniciar, devagar:
// um PDF por vez, para nunca atrapalhar o atendimento. PDF que não dá para ler fica sem número (a tela só não mostra as páginas).
func (s *Service) BackfillPDFPages(ctx context.Context) {
	if s.store == nil {
		return
	}
	skip := map[string]bool{}
	for ctx.Err() == nil {
		rows, err := s.db.Query(ctx, `
			SELECT m.id::text, a.storage_key FROM messages m JOIN attachments a ON a.message_id = m.id
			 WHERE m.kind = 'document' AND a.mime_type = 'application/pdf' AND NOT (m.payload ? 'pages') AND m.deleted_at IS NULL
			 ORDER BY m.sent_at DESC LIMIT 60`)
		if err != nil {
			return
		}
		type item struct{ id, key string }
		var batch []item
		for rows.Next() {
			var it item
			if rows.Scan(&it.id, &it.key) == nil && !skip[it.id] {
				batch = append(batch, it)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			return
		}
		for _, it := range batch {
			if ctx.Err() != nil {
				return
			}
			skip[it.id] = true // nesta rodada não tenta de novo (arquivo ilegível); na próxima partida sim
			data, err := s.store.Get(ctx, it.key)
			if err != nil {
				continue
			}
			s.recordPDFPages(ctx, it.id, data)
			select {
			case <-ctx.Done():
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
	}
}
