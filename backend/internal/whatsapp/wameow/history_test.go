package wameow

import (
	"errors"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestHistoryMessagesKeepIdentityAndChronology(t *testing.T) {
	own := types.NewJID("5592111111111", types.DefaultUserServer)
	cli := whatsmeow.NewClient(&store.Device{ID: &own, LID: types.NewJID("99999", types.HiddenUserServer)}, nil)
	makeMsg := func(id string, ts uint64, fromMe bool) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
			Key:              &waCommon.MessageKey{ID: proto.String(id), FromMe: proto.Bool(fromMe)},
			MessageTimestamp: proto.Uint64(ts), Message: &waE2E.Message{Conversation: proto.String("mensagem")},
		}}
	}
	conv := &waHistorySync.Conversation{ID: proto.String("123456@lid"), PnJID: proto.String("5592999999999@s.whatsapp.net"), Name: proto.String("Cliente"), Messages: []*waHistorySync.HistorySyncMsg{makeMsg("new", 200, true), makeMsg("old", 100, false), makeMsg("missing-time", 0, false)}}
	got := historyMessages(cli, conv, nil)
	if len(got) != 2 || got[0].ExternalID != "old" || got[1].ExternalID != "new" {
		t.Fatalf("incorrect order/IDs: %+v", got)
	}
	if !got[0].Historical || got[0].FromMe || !got[1].FromMe || got[0].PushName != "Cliente" || got[0].ContactWAID != "5592999999999" {
		t.Fatalf("incorrect history metadata: %+v", got)
	}
	if got[0].Download != nil {
		t.Fatal("history must not auto-download media")
	}
	conv.ID = proto.String("123@g.us")
	if len(historyMessages(cli, conv, nil)) != 0 {
		t.Fatal("unsupported group history imported")
	}
}

type historyRecorder struct {
	whatsapp.Handler
	messages map[string]bool
	progress whatsapp.HistoryProgress
	fail     bool
	contacts []whatsapp.HistoryContact
}

func (h *historyRecorder) OnHistoryMessage(m whatsapp.InboundMessage) (bool, error) {
	if h.fail {
		return false, errors.New("storage unavailable")
	}
	if h.messages[m.ExternalID] {
		return false, nil
	}
	h.messages[m.ExternalID] = true
	return true, nil
}
func (h *historyRecorder) OnHistoryContact(c whatsapp.HistoryContact) error {
	h.contacts = append(h.contacts, c)
	return nil
}
func (h *historyRecorder) OnHistoryProgress(p whatsapp.HistoryProgress) { h.progress = p }

func TestHistoryBatchReportsImportedDuplicatesAndFailures(t *testing.T) {
	h := &historyRecorder{messages: map[string]bool{}}
	cli := whatsmeow.NewClient(&store.Device{}, nil)
	p := &Provider{h: h, cli: cli}
	event := &events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{{
		ID: proto.String("5592999999999@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{{Message: &waWeb.WebMessageInfo{
			Key: &waCommon.MessageKey{ID: proto.String("same-id")}, MessageTimestamp: proto.Uint64(100), Message: &waE2E.Message{Conversation: proto.String("oi")},
		}}},
	}}}}
	p.importHistory(cli, event)
	p.importHistory(cli, event)
	if h.progress.State != "ready" || h.progress.Messages != 1 {
		t.Fatalf("duplicates counted: %+v", h.progress)
	}
	h.fail = true
	p.importHistory(cli, event)
	if h.progress.State != "partial" || h.progress.Failed != 1 {
		t.Fatalf("failed import reported as success: %+v", h.progress)
	}
}
