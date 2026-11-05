package conversations

import (
	"context"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// ImportContact fills missing names without overwriting staff edits.
func (s *Service) ImportContact(ctx context.Context, c whatsapp.HistoryContact) error {
	if c.WAID == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `INSERT INTO contacts (wa_id,phone,name) VALUES ($1,$2,$3)
 ON CONFLICT (wa_id) DO UPDATE SET
 phone=CASE WHEN contacts.phone='' THEN EXCLUDED.phone ELSE contacts.phone END,
 name=CASE WHEN contacts.name='' THEN EXCLUDED.name ELSE contacts.name END`, c.WAID, c.Phone, strings.TrimSpace(c.Name))
	return err
}
