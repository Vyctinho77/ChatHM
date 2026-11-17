package conversations

import (
	"context"
	"strings"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// sameMessage é a condição SQL ($1 = chat, $2 = ID da mensagem no WhatsApp) que acha "esta mensagem" mesmo quando o WhatsApp a entregou
// por outro endereço do mesmo chat. Uma conversa individual tem dois endereços: o número (…@s.whatsapp.net) e o identificador oculto
// (…@lid). Eventos ao vivo costumam vir pelo @lid e o histórico sincronizado pelo número (ou o contrário): conferir só (chat, ID)
// gravava a mesma mensagem duas vezes, com os anexos baixados de novo. O ID de uma mensagem é único na conta, então em chat
// individual basta ele; em grupo o ID só vale dentro do próprio grupo. `alias` é o prefixo da tabela (ex.: "m.") ou vazio.
func sameMessage(alias string) string {
	return alias + "external_id = $2 AND (" + alias + "chat_jid = $1 OR (" + alias + "chat_jid NOT LIKE '%@g.us' AND $1 NOT LIKE '%@g.us'))"
}

// resolveLIDContact troca o contato identificado só por @lid pelo número de telefone, quando o WhatsApp já informou qual é (tabela de
// correspondência do próprio WhatsApp). Sem isso, a mesma pessoa vira dois contatos: um pelo @lid e outro pelo número.
func (s *Service) resolveLIDContact(ctx context.Context, m whatsapp.InboundMessage) whatsapp.InboundMessage {
	if m.IsGroup || !strings.HasSuffix(m.ContactWAID, "@lid") {
		return m
	}
	var pn string
	// A tabela pertence ao WhatsApp e pode não existir (outro provedor, testes): qualquer erro significa "não sei".
	if err := s.db.QueryRow(ctx, `SELECT pn FROM whatsmeow_lid_map WHERE lid = $1 AND pn ~ '^[0-9]{8,15}$' LIMIT 1`, strings.TrimSuffix(m.ContactWAID, "@lid")).Scan(&pn); err != nil || pn == "" {
		return m
	}
	m.ContactWAID, m.ContactPhone = pn, "+"+pn
	return m
}
