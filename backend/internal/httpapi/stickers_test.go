package httpapi

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

func TestStickerReceivedStoredAuthorizedAndForwarded(t *testing.T) {
	w := setupWorld(t)
	data, err := os.ReadFile("../media/testdata/sticker-animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	p := itoa(int(phoneSeq.Add(1)))
	m := inbound("sticker-"+p, p, "Cliente Figurinha", "")
	m.Kind = whatsapp.KindSticker
	m.Payload = map[string]any{"mimeType": "image/webp", "animated": true, "width": 128, "height": 128}
	m.Download = func(context.Context) ([]byte, error) { return data, nil }
	w.e.wa.Receive(m)
	var url, id string
	eventually(t, "figurinha disponível", func() bool {
		c := conv(t, w.admin, "Cliente Figurinha")
		if c == nil {
			return false
		}
		msgs := w.admin.do("GET", "/api/v1/conversations/"+c["id"].(string)+"/messages", nil).body["messages"].([]any)
		if len(msgs) == 0 {
			return false
		}
		last := msgs[0].(map[string]any)
		url, _ = last["url"].(string)
		id, _ = last["id"].(string)
		return last["kind"] == "sticker" && last["animated"] == true && url != ""
	})
	status, headers, body := w.admin.raw(url)
	if status != 200 || headers.Get("Content-Type") != "image/webp" || !bytes.Equal(body, data) {
		t.Fatalf("sticker download: %d %s", status, headers.Get("Content-Type"))
	}
	if status, _, _ := w.e.client().raw(url); status != 401 {
		t.Fatalf("anonymous download: %d", status)
	}
	w.receive("Destino Figurinha", "folha de pagamento")
	target := mustConv(t, w.ana, "Destino Figurinha")["id"].(string)
	r := w.admin.do("POST", "/api/v1/messages/"+id+"/forward", map[string]any{"conversationIds": []string{target}})
	expect(t, r, 200)
	if r.body["results"].([]any)[0].(map[string]any)["ok"] != true {
		t.Fatalf("forward failed: %v", r.body)
	}
	media := lastSent(t, w).Media
	if media == nil || media.Kind != whatsapp.KindSticker || !media.Animated || !bytes.Equal(media.Data, data) || media.Caption != "" {
		t.Fatalf("forward changed sticker: %+v", media)
	}
}
