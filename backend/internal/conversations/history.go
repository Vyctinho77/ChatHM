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
	_, err := s.db.Exec(ctx, `INSERT INTO contacts (wa_id,phone,name,whatsapp_saved) VALUES ($1,$2,$3,$4)
 ON CONFLICT (wa_id) DO UPDATE SET
 phone=CASE WHEN contacts.phone='' THEN EXCLUDED.phone ELSE contacts.phone END,
 name=CASE WHEN contacts.name='' OR (EXCLUDED.whatsapp_saved AND NOT contacts.name_edited AND EXCLUDED.name<>'') THEN EXCLUDED.name ELSE contacts.name END,
 whatsapp_saved=contacts.whatsapp_saved OR EXCLUDED.whatsapp_saved`, c.WAID, c.Phone, strings.TrimSpace(c.Name), c.Saved)
	return err
}
