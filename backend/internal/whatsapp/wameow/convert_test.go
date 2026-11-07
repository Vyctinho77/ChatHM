package wameow

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func msgEvent(chat types.JID, m *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: chat},
			ID:            "3EB0ABC", PushName: "Rafael", Timestamp: time.Unix(1_700_000_000, 0),
		},
		Message: m,
	}
}

func TestConvertMessage(t *testing.T) {
	pn := types.NewJID("5592998112040", types.DefaultUserServer)
	ctx := context.Background()

	got, ok := convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{Conversation: proto.String("quero emitir uma nota")}))
	if !ok || got.Kind != whatsapp.KindText || got.Body != "quero emitir uma nota" || got.ContactWAID != "5592998112040" ||
		got.ContactPhone != "+5592998112040" || got.ExternalID != "3EB0ABC" || got.PushName != "Rafael" {
		t.Fatalf("texto = %+v", got)
	}

	got, ok = convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		FileName: proto.String("PGDAS.pdf"), Mimetype: proto.String("application/pdf"), FileLength: proto.Uint64(5200),
	}}))
	if !ok || got.Kind != whatsapp.KindDocument || got.Payload["fileName"] != "PGDAS.pdf" || got.Payload["sizeBytes"] != uint64(5200) {
		t.Fatalf("documento = %+v", got)
	}

	got, ok = convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		Seconds: proto.Uint32(12), PTT: proto.Bool(true), Waveform: []byte{0, 50, 100},
	}}))
	if !ok || got.Kind != whatsapp.KindAudio || got.Payload["durationSec"] != uint32(12) || len(got.Payload["waveform"].([]float64)) != 3 {
		t.Fatalf("áudio = %+v", got)
	}

	got, ok = convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(-3.1), DegreesLongitude: proto.Float64(-60.0), Name: proto.String("Escritório"),
	}}))
	if !ok || got.Kind != whatsapp.KindLocation || got.Payload["lat"] != -3.1 {
		t.Fatalf("localização = %+v", got)
	}
	if _, has := got.Payload["thumb"]; has {
		t.Fatalf("sem miniatura não deve haver thumb: %+v", got.Payload)
	}

	got, ok = convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(-3.1), DegreesLongitude: proto.Float64(-60.0), JPEGThumbnail: []byte{0xff, 0xd8, 0xff},
	}}))
	if th, _ := got.Payload["thumb"].(string); !ok || th != "data:image/jpeg;base64,/9j/" {
		t.Fatalf("miniatura = %+v", got.Payload)
	}

	got, ok = convertMessage(ctx, nil, msgEvent(pn, &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}))
	if !ok || got.Kind != whatsapp.KindUnsupported || got.Payload["label"] != "Vídeo" {
		t.Fatalf("vídeo = %+v", got)
	}

	// Conversa endereçada por LID usa o telefone alternativo.
	lid := types.NewJID("123456789", types.HiddenUserServer)
	ev := msgEvent(lid, &waE2E.Message{Conversation: proto.String("oi")})
	ev.Info.SenderAlt = pn
	got, ok = convertMessage(ctx, nil, ev)
	if !ok || got.ContactWAID != "5592998112040" || got.ChatJID != "123456789@lid" {
		t.Fatalf("LID = %+v", got)
	}

	// Mensagem enviada pelo próprio celular.
	ev = msgEvent(pn, &waE2E.Message{Conversation: proto.String("Bom dia!")})
	ev.Info.IsFromMe = true
	got, _ = convertMessage(ctx, nil, ev)
	if !got.FromMe || got.PushName != "" {
		t.Fatalf("fromMe = %+v", got)
	}

	// Ignorados: grupo, status, reação e edição.
	group := types.NewJID("120363", types.GroupServer)
	ignored := []*events.Message{
		msgEvent(types.StatusBroadcastJID, &waE2E.Message{Conversation: proto.String("x")}),
		msgEvent(pn, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}}),
		func() *events.Message {
			e := msgEvent(pn, &waE2E.Message{Conversation: proto.String("x")})
			e.IsEdit = true
			return e
		}(),
	}
	// Grupos agora são importados: o contato é o JID do grupo e o autor vai em SenderName.
	ge := msgEvent(group, &waE2E.Message{Conversation: proto.String("oi grupo")})
	ge.Info.IsGroup = true
	ge.Info.PushName = "Marta"
	if g, ok := convertMessage(ctx, nil, ge); !ok || !g.IsGroup || g.ContactWAID != group.String() || g.SenderName != "Marta" || g.PushName != "" || g.ChatJID != group.String() {
		t.Fatalf("grupo = %+v ok=%v", g, ok)
	}
	for i, e := range ignored {
		if _, ok := convertMessage(ctx, nil, e); ok {
			t.Fatalf("caso %d deveria ser ignorado", i)
		}
	}
}
