package conversations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/vyctinho77/chathm/backend/internal/audit"
	"github.com/vyctinho77/chathm/backend/internal/platform/database"
)

// Correção de dados repetidos. O mesmo chat chegou ao ChatHM por dois endereços do WhatsApp (o número e o @lid, ver sameMessage): as
// mensagens vieram duas vezes, os anexos foram baixados de novo e a mesma pessoa virou dois contatos. Aqui isso é desfeito:
//   1. contato @lid cujo número o WhatsApp já informou (tabela do próprio WhatsApp): ganha o número ou é unido ao contato desse número;
//   2. mensagens com o mesmo ID em chat individual: fica uma só (a não apagada, a que tem autor da equipe, a que tem arquivo e a mais antiga);
//      os arquivos das cópias saem;
//   3. conversas que ficaram vazias saem; contadores são refeitos.
// Tudo numa transação. A prévia roda o mesmo código e desfaz no fim (nada é gravado).

// DedupeReport resume o que a correção encontrou (prévia) ou fez (aplicada).
type DedupeReport struct {
	Applied              bool `json:"applied"`
	ContactsRenamed      int  `json:"contactsRenamed"`      // contato @lid que ganhou o número de telefone
	ContactsMerged       int  `json:"contactsMerged"`       // contato @lid unido ao contato que já existia com o número
	ContactsSkipped      int  `json:"contactsSkipped"`      // não deu para unir (precisa de revisão manual)
	DuplicateMessages    int  `json:"duplicateMessages"`    // cópias removidas
	FilesRemoved         int  `json:"filesRemoved"`         // arquivos (anexos) das cópias
	ConversationsRemoved int  `json:"conversationsRemoved"` // conversas que ficaram vazias
}

// Nothing diz se não há nada a corrigir.
func (r DedupeReport) Nothing() bool {
	return r.ContactsRenamed+r.ContactsMerged+r.DuplicateMessages+r.ConversationsRemoved == 0
}

var errDedupePreview = errors.New("prévia: nada gravado")

const dedupeLockKey = 7_410_041 // trava para duas correções não rodarem ao mesmo tempo

// Dedupe corrige os dados repetidos. Com apply=false só conta (nada é gravado).
func (s *Service) Dedupe(ctx context.Context, apply bool) (DedupeReport, error) {
	var rep DedupeReport
	var blobKeys []string
	err := database.InTx(ctx, s.db, func(tx pgx.Tx) error {
		rep, blobKeys = DedupeReport{}, nil
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, dedupeLockKey); err != nil {
			return err
		}
		if err := s.unifyContacts(ctx, tx, &rep); err != nil {
			return err
		}
		keys, touched, err := s.removeDuplicateMessages(ctx, tx, &rep)
		if err != nil {
			return err
		}
		blobKeys = keys
		if err := s.tidyConversations(ctx, tx, touched, &rep); err != nil {
			return err
		}
		if !apply {
			return errDedupePreview // desfaz tudo: era só a prévia
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "maintenance.dedupe", TargetType: "system", Metadata: map[string]any{
			"contactsRenamed": rep.ContactsRenamed, "contactsMerged": rep.ContactsMerged, "contactsSkipped": rep.ContactsSkipped,
			"duplicateMessages": rep.DuplicateMessages, "filesRemoved": rep.FilesRemoved, "conversationsRemoved": rep.ConversationsRemoved,
		}})
	})
	if err != nil && !errors.Is(err, errDedupePreview) {
		return DedupeReport{}, err
	}
	rep.Applied = apply
	if apply {
		s.removeBlobs(ctx, blobKeys)
	}
	return rep, nil
}

// unifyContacts: contato @lid com número conhecido ganha o número ou é unido ao contato do número.
func (s *Service) unifyContacts(ctx context.Context, tx pgx.Tx, rep *DedupeReport) error {
	var has bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('whatsmeow_lid_map') IS NOT NULL`).Scan(&has); err != nil || !has {
		return err // sem a tabela do WhatsApp neste banco: nada a unir
	}
	type pair struct{ lid, pn string }
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (c.id) c.id::text, m.pn
		  FROM contacts c JOIN whatsmeow_lid_map m ON m.lid = split_part(c.wa_id, '@', 1)
		 WHERE c.wa_id LIKE '%@lid' AND c.kind <> 'group' AND m.pn ~ '^[0-9]{8,15}$'
		 ORDER BY c.id, m.pn`)
	if err != nil {
		return err
	}
	var todo []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.lid, &p.pn); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range todo {
		var target string
		err := tx.QueryRow(ctx, `SELECT id::text FROM contacts WHERE wa_id = $1`, p.pn).Scan(&target)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx, `UPDATE contacts SET wa_id = $2, phone = CASE WHEN phone = '' THEN '+' || $2 ELSE phone END, updated_at = now() WHERE id = $1`, p.lid, p.pn); err != nil {
				return err
			}
			rep.ContactsRenamed++
		case err != nil:
			return err
		default:
			ok, err := s.mergeContact(ctx, tx, p.lid, target)
			if err != nil {
				return err
			}
			if ok {
				rep.ContactsMerged++
			} else {
				rep.ContactsSkipped++
			}
		}
	}
	return nil
}

// mergeContact passa tudo do contato `from` (@lid) para `to` (número) e remove `from`. Devolve false (sem mexer) se não der: por
// exemplo, os dois têm conversa aberta (só pode haver uma por contato) ou um vínculo de automação repetiria. Usa um ponto de salvamento
// para um contato problemático não derrubar os demais.
func (s *Service) mergeContact(ctx context.Context, tx pgx.Tx, from, to string) (bool, error) {
	var bothOpen bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM conversations WHERE contact_id = $1 AND status <> 'closed')
		   AND EXISTS (SELECT 1 FROM conversations WHERE contact_id = $2 AND status <> 'closed')`, from, to).Scan(&bothOpen); err != nil {
		return false, err
	}
	if bothOpen {
		return false, nil
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	steps := []string{
		`UPDATE conversations SET contact_id = $2 WHERE contact_id = $1`,
		`UPDATE automation_documents SET owner_contact_id = $2 WHERE owner_contact_id = $1`,
		`UPDATE automation_sends SET contact_id = $2 WHERE contact_id = $1`,
		`UPDATE billing_charges SET contact_id = $2 WHERE contact_id = $1`,
		// O contato do número manda; só preenche o que está vazio com o que o @lid sabia.
		`UPDATE contacts p SET
		        name = CASE WHEN p.name = '' THEN l.name ELSE p.name END,
		        company = CASE WHEN p.company = '' THEN l.company ELSE p.company END,
		        note = CASE WHEN p.note = '' THEN l.note ELSE p.note END,
		        tax_id = CASE WHEN p.tax_id = '' THEN l.tax_id ELSE p.tax_id END,
		        asaas_id = CASE WHEN p.asaas_id = '' THEN l.asaas_id ELSE p.asaas_id END,
		        whatsapp_saved = p.whatsapp_saved OR l.whatsapp_saved,
		        automation_opt_out_at = coalesce(p.automation_opt_out_at, l.automation_opt_out_at),
		        updated_at = now()
		  FROM contacts l WHERE p.id = $2 AND l.id = $1`,
	}
	for _, q := range steps {
		if _, err := sp.Exec(ctx, q, from, to); err != nil {
			_ = sp.Rollback(ctx)
			slog.Warn("correção de duplicatas: contato não unido", "from", from, "to", to, "err", err)
			return false, nil
		}
	}
	if _, err := sp.Exec(ctx, `DELETE FROM contacts WHERE id = $1`, from); err != nil {
		_ = sp.Rollback(ctx)
		slog.Warn("correção de duplicatas: contato não removido", "from", from, "to", to, "err", err)
		return false, nil
	}
	return true, sp.Commit(ctx)
}

// removeDuplicateMessages: deixa uma mensagem por ID do WhatsApp em chat individual. Devolve as chaves dos arquivos a apagar do
// armazenamento (depois de confirmar a transação) e as conversas afetadas.
func (s *Service) removeDuplicateMessages(ctx context.Context, tx pgx.Tx, rep *DedupeReport) (blobKeys, touched []string, err error) {
	rows, err := tx.Query(ctx, `
		SELECT m.external_id, m.id::text, m.conversation_id::text, m.kind, m.direction,
		       EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = m.id)
		  FROM messages m
		 WHERE m.chat_jid NOT LIKE '%@g.us'
		   AND m.external_id IN (SELECT external_id FROM messages WHERE chat_jid NOT LIKE '%@g.us' GROUP BY external_id HAVING count(*) > 1)
		 ORDER BY m.external_id, (m.deleted_at IS NULL) DESC, (m.sender_user_id IS NOT NULL) DESC, 6 DESC, m.created_at, m.id`)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		ext, id, conv, kind, dir string
		att                      bool
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ext, &r.id, &r.conv, &r.kind, &r.dir, &r.att); err != nil {
			rows.Close()
			return nil, nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	touchedSet := map[string]bool{}
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].ext == all[i].ext {
			j++
		}
		keeper, group := all[i], all[i+1:j]
		i = j
		var extras []string
		for _, e := range group {
			if e.kind != keeper.kind || e.dir != keeper.dir {
				continue // não é a mesma mensagem: não mexe
			}
			if !keeper.att && e.att { // a cópia tem o arquivo e o original não: o arquivo passa para o original
				if _, err := tx.Exec(ctx, `UPDATE attachments SET message_id = $1 WHERE message_id = $2`, keeper.id, e.id); err != nil {
					return nil, nil, err
				}
				keeper.att = true
			}
			extras = append(extras, e.id)
			touchedSet[e.conv] = true
		}
		if len(extras) == 0 {
			continue
		}
		krows, err := tx.Query(ctx, `SELECT storage_key FROM attachments WHERE message_id = ANY($1::uuid[])`, extras)
		if err != nil {
			return nil, nil, err
		}
		for krows.Next() {
			var k string
			if err := krows.Scan(&k); err != nil {
				krows.Close()
				return nil, nil, err
			}
			blobKeys = append(blobKeys, k)
			rep.FilesRemoved++
		}
		krows.Close()
		if err := krows.Err(); err != nil {
			return nil, nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM messages WHERE id = ANY($1::uuid[])`, extras); err != nil {
			return nil, nil, err
		}
		rep.DuplicateMessages += len(extras)
	}
	for c := range touchedSet {
		touched = append(touched, c)
	}
	return blobKeys, touched, nil
}

// tidyConversations: conversa que ficou sem mensagem sai (se ninguém assumiu nem a colocou numa fila); os contadores das demais são refeitos.
func (s *Service) tidyConversations(ctx context.Context, tx pgx.Tx, touched []string, rep *DedupeReport) error {
	if len(touched) == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM conversations c
		 WHERE c.id = ANY($1::uuid[]) AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id)
		   AND (c.status = 'closed' OR (c.assignee_id IS NULL AND c.segment_id IS NULL))`, touched)
	if err != nil {
		return err
	}
	rep.ConversationsRemoved = int(tag.RowsAffected())
	_, err = tx.Exec(ctx, `
		UPDATE conversations c SET
		       unread_count = LEAST(c.unread_count, (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id AND m.direction = 'in' AND m.deleted_at IS NULL)),
		       last_message_at = coalesce((SELECT max(m.sent_at) FROM messages m WHERE m.conversation_id = c.id), c.last_message_at)
		 WHERE c.id = ANY($1::uuid[])`, touched)
	return err
}

// removeBlobs apaga do armazenamento os arquivos das cópias (e as prévias), se nenhum anexo restante usa a mesma chave.
func (s *Service) removeBlobs(ctx context.Context, keys []string) {
	if s.store == nil {
		return
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		var used bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE storage_key = $1)`, k).Scan(&used); err != nil || used {
			continue
		}
		for _, key := range []string{k, k + ".preview"} {
			if err := s.store.Delete(ctx, key); err != nil && !strings.Contains(err.Error(), "não encontrado") {
				slog.Warn("correção de duplicatas: arquivo não removido", "key", key, "err", fmt.Sprint(err))
			}
		}
	}
}
