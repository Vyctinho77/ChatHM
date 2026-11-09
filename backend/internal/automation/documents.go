package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/media"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
	"github.com/vyctinho77/chathm/backend/internal/platform/httpx"
)

// Document: um PDF pronto para enviar. A IA nunca envia bytes: aponta o id, e o servidor confere de quem é.
type Document struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	MimeType       string    `json:"mimeType"`
	SizeBytes      int64     `json:"sizeBytes"`
	OwnerContactID string    `json:"ownerContactId,omitempty"`
	OwnerName      string    `json:"ownerName,omitempty"`
	Shareable      bool      `json:"shareable"`
	CreatedAt      time.Time `json:"createdAt"`
}

const selectDoc = `SELECT d.id::text, d.name, d.mime_type, d.size_bytes, coalesce(d.owner_contact_id::text, ''), coalesce(nullif(ct.name, ''), ct.phone, ''), d.shareable, d.created_at
	FROM automation_documents d LEFT JOIN contacts ct ON ct.id = d.owner_contact_id`

func scanDoc(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.Name, &d.MimeType, &d.SizeBytes, &d.OwnerContactID, &d.OwnerName, &d.Shareable, &d.CreatedAt)
	return d, err
}

// guardDocument: o documento existe e é do próprio destinatário (ou marcado como compartilhável).
func (s *Service) guardDocument(ctx context.Context, docID, contactID string) (Document, *Refusal, error) {
	if !httpx.ValidUUID(docID) {
		return Document{}, refuse("document", "document_id inválido: use um id de list_documents."), nil
	}
	d, err := scanDoc(s.db.QueryRow(ctx, selectDoc+` WHERE d.id = $1`, docID))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, refuse("document", "Esse documento não existe na biblioteca (list_documents)."), nil
	}
	if err != nil {
		return d, nil, err
	}
	if !d.Shareable && d.OwnerContactID != contactID {
		return d, refuse("document_owner", "Esse documento é de outro cliente. Só é possível anexar o PDF do próprio destinatário ou um marcado como compartilhável."), nil
	}
	return d, nil, nil
}

// Documents: a biblioteca. Com contactID, só o que a IA pode anexar àquele cliente (dele + compartilháveis).
func (s *Service) Documents(ctx context.Context, contactID string) ([]Document, error) {
	rows, err := s.db.Query(ctx, selectDoc+` WHERE ($1 = '' OR d.shareable OR d.owner_contact_id::text = $1) ORDER BY d.created_at DESC LIMIT 300`, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UploadDocument guarda um PDF na biblioteca (passa pela validação de mídia e pelo antivírus como qualquer anexo).
func (s *Service) UploadDocument(ctx context.Context, actorID, ip, name string, data []byte, ownerContactID string, shareable bool) (Document, error) {
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < 1 || n > 150 {
		return Document{}, httpx.FieldErrors{"name": "O nome deve ter de 1 a 150 caracteres."}
	}
	if ownerContactID == "" && !shareable {
		return Document{}, httpx.FieldErrors{"ownerContactId": "Escolha o cliente dono do documento ou marque como compartilhável."}
	}
	if ownerContactID != "" {
		var ok bool
		if !httpx.ValidUUID(ownerContactID) {
			return Document{}, httpx.FieldErrors{"ownerContactId": "Cliente inválido."}
		}
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contacts WHERE id = $1)`, ownerContactID).Scan(&ok); err != nil {
			return Document{}, err
		}
		if !ok {
			return Document{}, httpx.FieldErrors{"ownerContactId": "Cliente não encontrado."}
		}
	}
	p, err := s.media.ProcessInbound(ctx, media.Document, data, name, "application/pdf")
	if err != nil {
		var rej *media.RejectError
		if errors.As(err, &rej) {
			return Document{}, httpx.FieldErrors{"file": rej.Msg}
		}
		return Document{}, err
	}
	if p.MimeType != "application/pdf" {
		return Document{}, httpx.FieldErrors{"file": "Só PDFs podem ficar na biblioteca de envios automáticos."}
	}
	key := "automation/" + uuid.NewString()
	if err := s.store.Put(ctx, key, p.Data); err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(p.Data)
	var id string
	err = database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO automation_documents (name, storage_key, mime_type, size_bytes, sha256, owner_contact_id, shareable, uploaded_by)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, $7, $8) RETURNING id::text`,
			name, key, p.MimeType, len(p.Data), hex.EncodeToString(sum[:]), ownerContactID, shareable, actorID).Scan(&id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.document.upload", TargetType: "automation_document", TargetID: id, IP: ip,
			Metadata: map[string]any{"ownerContactId": ownerContactID, "shareable": shareable, "sizeBytes": len(p.Data)}})
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), key)
		return Document{}, err
	}
	return scanDoc(s.db.QueryRow(ctx, selectDoc+` WHERE d.id = $1`, id))
}

// DeleteDocument: não apaga enquanto um envio pendente, aprovado ou em andamento usa o arquivo.
func (s *Service) DeleteDocument(ctx context.Context, actorID, ip, id string) error {
	if !httpx.ValidUUID(id) {
		return ErrNotFound
	}
	var key string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT storage_key FROM automation_documents WHERE id = $1 FOR UPDATE`, id).Scan(&key); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		var inUse bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM automation_sends WHERE document_id = $1 AND status IN ('pending_approval','approved','sending'))`, id).Scan(&inUse); err != nil {
			return err
		}
		if inUse {
			return ErrState
		}
		if _, err := tx.Exec(ctx, `DELETE FROM automation_documents WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "automation.document.delete", TargetType: "automation_document", TargetID: id, IP: ip})
	})
	if err != nil {
		return err
	}
	_ = s.store.Delete(context.WithoutCancel(ctx), key)
	return nil
}
