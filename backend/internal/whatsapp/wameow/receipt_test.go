package wameow

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestConvertReceiptKeepsReadAndPlayedApart(t *testing.T) {
	now := time.Now()
	mk := func(typ types.ReceiptType) *events.Receipt {
		e := &events.Receipt{MessageIDs: []types.MessageID{"A1", "A2"}, Type: typ}
		e.Timestamp = now
		return e
	}
	for typ, want := range map[types.ReceiptType]whatsapp.ReceiptStatus{
		types.ReceiptTypeDelivered: whatsapp.ReceiptDelivered,
		types.ReceiptTypeRead:      whatsapp.ReceiptRead,
		types.ReceiptTypePlayed:    whatsapp.ReceiptPlayed,
	} {
		got, ok := convertReceipt(mk(typ))
		if !ok || got.Status != want || len(got.MessageIDs) != 2 || !got.Timestamp.Equal(now) {
			t.Errorf("%q: got %+v ok=%v, quer %q", typ, got, ok, want)
		}
	}
	// Outros aparelhos nossos e recibos de grupo não contam.
	for _, typ := range []types.ReceiptType{types.ReceiptTypeReadSelf, types.ReceiptTypePlayedSelf, types.ReceiptTypeSender} {
		if _, ok := convertReceipt(mk(typ)); ok {
			t.Errorf("%q deveria ser ignorado", typ)
		}
	}
	grp := mk(types.ReceiptTypePlayed)
	grp.IsGroup = true
	if _, ok := convertReceipt(grp); ok {
		t.Error("recibo de grupo deveria ser ignorado")
	}
}
