package wameow

import (
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// convertReceipt traduz recibos de entrega/leitura de conversas 1:1. Ignora recibos
// de grupos, dos nossos outros aparelhos ("sender"/"read-self") e de tentativas de reenvio.
func convertReceipt(e *events.Receipt) (whatsapp.Receipt, bool) {
	if e.IsGroup || e.IsFromMe || len(e.MessageIDs) == 0 {
		return whatsapp.Receipt{}, false
	}
	var status whatsapp.ReceiptStatus
	switch e.Type {
	case types.ReceiptTypeDelivered:
		status = whatsapp.ReceiptDelivered
	case types.ReceiptTypeRead, types.ReceiptTypePlayed:
		status = whatsapp.ReceiptRead
	default:
		return whatsapp.Receipt{}, false
	}
	ids := make([]string, len(e.MessageIDs))
	for i, id := range e.MessageIDs {
		ids[i] = string(id)
	}
	return whatsapp.Receipt{MessageIDs: ids, Status: status, Timestamp: e.Timestamp}, true
}
