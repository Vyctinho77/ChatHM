package conversations

import "context"

type ContactRecord struct {
	Contact
	ConversationID *string `json:"conversationId,omitempty"`
}

// Admin sees the imported directory. Agents only see contacts associated with
// conversations they can already access, including the history visibility rules.
func (s *Service) ListContacts(ctx context.Context, v Viewer) ([]ContactRecord, error) {
	// O administrador vê todos os contatos: a checagem de acesso (EXISTS) só roda para quem não é administrador.
	rows, err := s.db.Query(ctx, `SELECT ct.id,ct.name,ct.phone,ct.wa_id,ct.company,ct.note,ct.kind,(CASE WHEN ct.avatar IS NULL THEN 0 ELSE ct.avatar_version END),(ct.name_edited OR ct.whatsapp_saved),lc.id::text
 FROM contacts ct LEFT JOIN LATERAL (
  SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id=c.segment_id
  WHERE c.contact_id=ct.id AND c.status <> 'closed' AND `+forViewer+`
  ORDER BY c.last_message_at DESC LIMIT 1
 ) lc ON true
 WHERE $2::boolean OR EXISTS (
  SELECT 1 FROM conversations c LEFT JOIN segments s ON s.id=c.segment_id
  WHERE c.contact_id=ct.id AND `+forViewer+`)
 ORDER BY lower(coalesce(nullif(ct.name,''),nullif(ct.phone,''),ct.wa_id)),ct.id`, v.ID, v.Admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContactRecord{}
	for rows.Next() {
		var c ContactRecord
		var name, phone, waID, kind string
		var ver int64
		if err := rows.Scan(&c.ID, &name, &phone, &waID, &c.Company, &c.Note, &kind, &ver, &c.Saved, &c.ConversationID); err != nil {
			return nil, err
		}
		c.finish(name, phone, waID, kind, ver)
		out = append(out, c)
	}
	return out, rows.Err()
}
