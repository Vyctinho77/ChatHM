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
	for _, m := range historyMessages(cli, conv, nil) {
		if !m.IsGroup || m.GroupName != "Cliente" || m.ContactWAID != "123@g.us" {
			t.Fatalf("grupo no histórico: %+v", m)
		}
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

// O histórico não baixa nada na hora (seriam milhares de arquivos): toda mídia entra com a REFERÊNCIA guardada para o servidor baixar depois.
func TestHistoryKeepsMediaReferenceInsteadOfDownloading(t *testing.T) {
	own := types.NewJID("5592111111111", types.DefaultUserServer)
	cli := whatsmeow.NewClient(&store.Device{ID: &own, LID: types.NewJID("99999", types.HiddenUserServer)}, nil)
	web := func(id string, m *waE2E.Message) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
			Key: &waCommon.MessageKey{ID: proto.String(id)}, MessageTimestamp: proto.Uint64(100), Message: m,
		}}
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	conv := &waHistorySync.Conversation{ID: proto.String("5592999999999@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{
		web("pdf", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{FileName: proto.String("guia.pdf"), Mimetype: proto.String("application/pdf"), FileLength: proto.Uint64(200 << 10), MediaKey: key, DirectPath: proto.String("/v/t62/x"), JPEGThumbnail: []byte("miniatura")}}),
		web("imagem", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Mimetype: proto.String("image/jpeg"), FileLength: proto.Uint64(10 << 10), MediaKey: key, DirectPath: proto.String("/v/t62/y"), JPEGThumbnail: []byte("miniatura")}}),
		web("audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{Mimetype: proto.String("audio/ogg"), FileLength: proto.Uint64(10 << 10), MediaKey: key, DirectPath: proto.String("/v/t62/z")}}),
		web("figurinha", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{Mimetype: proto.String("image/webp"), FileLength: proto.Uint64(5 << 10), MediaKey: key, DirectPath: proto.String("/v/t62/w")}}),
		web("texto", &waE2E.Message{Conversation: proto.String("oi")}),
	}}
	kinds := map[string]whatsapp.Kind{"pdf": whatsapp.KindDocument, "imagem": whatsapp.KindImage, "audio": whatsapp.KindAudio, "figurinha": whatsapp.KindSticker}
	n := 0
	for _, m := range historyMessages(cli, conv, nil) {
		n++
		if !m.Historical {
			t.Errorf("%s deveria ser marcada como histórico", m.ExternalID)
		}
		if m.Download != nil {
			t.Errorf("%s: o histórico não baixa na hora", m.ExternalID)
		}
		want, isMedia := kinds[m.ExternalID]
		if !isMedia {
			if m.MediaRef != nil {
				t.Errorf("%s: texto não tem referência de mídia", m.ExternalID)
			}
			continue
		}
		if m.MediaRef == nil || m.MediaRef.Kind != want || len(m.MediaRef.Proto) == 0 {
			t.Fatalf("%s: faltou a referência de mídia: %+v", m.ExternalID, m.MediaRef)
		}
		// A referência volta a ser a mensagem de mídia, com a chave e o endereço, e sem a miniatura.
		dm, gotKey, _, err := downloadable(*m.MediaRef)
		if err != nil || dm == nil || string(gotKey) != string(key) {
			t.Errorf("%s: referência não reconstrói a mídia: %v", m.ExternalID, err)
		}
		var back waE2E.Message
		if err := proto.Unmarshal(m.MediaRef.Proto, &back); err != nil {
			t.Fatal(err)
		}
		if back.GetImageMessage().GetJPEGThumbnail() != nil || back.GetDocumentMessage().GetJPEGThumbnail() != nil {
			t.Errorf("%s: a miniatura só pesaria na referência", m.ExternalID)
		}
	}
	if n != 5 {
		t.Fatalf("mensagens importadas = %d, esperado 5 (a mensagem sempre entra)", n)
	}
}
