package conversations

import "context"

type ContactRecord struct {
	Contact
	ConversationID *string `json:"conversationId,omitempty"`
}

// Admin sees the imported directory. Agents only see contacts associated with
// conversations they can already access, including the history visibility rules.
func (s *Service) ListContacts(ctx context.Context, v Viewer) ([]ContactRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT ct.id,ct.name,ct.phone,ct.wa_id,ct.company,ct.note,lc.id::text
 FROM contacts ct LEFT JOIN LATERAL (
  SELECT c.id FROM conversations c LEFT JOIN segments s ON s.id=c.segment_id
  WHERE c.contact_id=ct.id AND `+forViewer+`
  ORDER BY c.last_message_at DESC LIMIT 1
 ) lc ON true WHERE ($2::boolean OR lc.id IS NOT NULL)
 ORDER BY lower(coalesce(nullif(ct.name,''),nullif(ct.phone,''),ct.wa_id)),ct.id`, v.ID, v.Admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContactRecord{}
	for rows.Next() {
		var c ContactRecord
		var name, phone, waID string
		if err := rows.Scan(&c.ID, &name, &phone, &waID, &c.Company, &c.Note, &c.ConversationID); err != nil {
			return nil, err
		}
		c.Name = displayName(name, phone, waID)
		c.Phone = FormatPhone(phone)
		out = append(out, c)
	}
	return out, rows.Err()
}
