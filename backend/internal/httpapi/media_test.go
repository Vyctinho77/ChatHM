package httpapi

// Fase 4: anexos (imagem, áudio, documento), localização, armazenamento privado e acesso.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/vyctinho77/chathm/backend/internal/auth"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
	"github.com/vyctinho77/chathm/backend/internal/whatsapp/fake"
)

func testPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x += 7 {
		for y := 0; y < h; y += 7 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// testWAV: 1 s de seno a 8 kHz, mono 16 bits.
func testWAV() []byte {
	const rate, n = 8000, 8000
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + n*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(n * 2))
	for i := 0; i < n; i++ {
		w(int16(8000 * math.Sin(2*math.Pi*440*float64(i)/rate)))
	}
	return b.Bytes()
}

func (c *client) upload(path string, fields map[string]string, fileName string, data []byte) resp {
	c.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if data != nil {
		fw, _ := mw.CreateFormFile("file", fileName)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.base+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", c.origin)
	req.Header.Set(auth.CSRFHeader, c.csrf)
	return c.exec(req)
}

// raw faz um GET e devolve status, cabeçalhos e corpo.
func (c *client) raw(path string) (int, http.Header, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.base+path, nil)
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

func (c *client) exec(req *http.Request) resp {
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out := resp{status: res.StatusCode, header: res.Header}
	raw, _ := io.ReadAll(res.Body)
	out.body = map[string]any{}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (w *world) attach(c *client, convID, kind string, fileName string, data []byte, extra map[string]string) resp {
	f := map[string]string{"id": uuid.NewString(), "kind": kind}
	for k, v := range extra {
		f[k] = v
	}
	return c.upload("/api/v1/conversations/"+convID+"/messages", f, fileName, data)
}

func lastSent(t *testing.T, w *world) fake.Sent {
	t.Helper()
	s := w.e.wa.Sent()
	if len(s) == 0 {
		t.Fatal("nada foi enviado ao WhatsApp")
	}
	return s[len(s)-1]
}

func TestSendImageStandardAndHD(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Foto", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Foto")["id"].(string)

	big := testPNG(2400, 1200)
	r := w.attach(w.ana, id, "image", "foto.png", big, map[string]string{"text": "segue"})
	expect(t, r, 201)
	s := lastSent(t, w)
	if s.Media == nil || s.Media.Kind != whatsapp.KindImage || s.Media.Width != 1600 || s.Media.Height != 800 || s.Media.Caption != "*ana:*\nsegue" {
		t.Fatalf("envio comum deveria reduzir a 1600 px: %+v", s.Media)
	}

	r = w.attach(w.ana, id, "image", "foto.png", big, map[string]string{"hd": "true"})
	expect(t, r, 201)
	if m := lastSent(t, w).Media; m.Width != 2400 || len(m.Data) != len(big) {
		t.Fatalf("HD deveria enviar o original: %dx%d", m.Width, m.Height)
	}
	if r.body["hd"] != true {
		t.Fatalf("mensagem HD deve indicar hd: %v", r.body)
	}
}

func TestAttachmentAccessIsAuthorized(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Anexo", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Anexo")["id"].(string)
	r := w.attach(w.ana, id, "document", "relatorio.pdf", []byte("%PDF-1.4\n%fake pdf content"), nil)
	expect(t, r, 201)
	url := r.body["url"].(string)
	if !strings.HasPrefix(url, "/api/v1/attachments/") {
		t.Fatalf("url inesperada: %v", url)
	}

	// Equipe da conversa e admin baixam; outra equipe e anônimo não.
	st, hd, body := w.ana2.raw(url)
	if st != 200 || !bytes.Contains(body, []byte("fake pdf")) {
		t.Fatalf("ana2 deveria baixar: %d", st)
	}
	if hd.Get("Content-Disposition") == "" || !strings.HasPrefix(hd.Get("Content-Disposition"), "attachment") ||
		hd.Get("Content-Type") != "application/pdf" && hd.Get("Content-Type") != "application/octet-stream" ||
		hd.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(hd.Get("Cache-Control"), "no-store") {
		t.Fatalf("cabeçalhos de segurança do download: %v", hd)
	}
	if st, _, _ := w.admin.raw(url); st != 200 {
		t.Fatalf("admin: %d", st)
	}
	if st, _, _ := w.bia.raw(url); st != 404 {
		t.Fatalf("outra equipe não pode baixar: %d", st)
	}
	if st, _, _ := w.e.client().raw(url); st != 401 {
		t.Fatalf("anônimo: %d", st)
	}
	if st, _, _ := w.ana.raw("/api/v1/attachments/" + uuid.NewString()); st != 404 {
		t.Fatalf("inexistente: %d", st)
	}
	if st, _, _ := w.ana.raw("/api/v1/attachments/nao-e-uuid"); st != 404 {
		t.Fatalf("id inválido: %d", st)
	}
}

func TestAttachmentValidation(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Valida", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Valida")["id"].(string)

	// O tipo vem do conteúdo: texto com nome de imagem é recusado.
	expect(t, w.attach(w.ana, id, "image", "foto.png", []byte("isto não é uma imagem"), nil), 422)
	// Executáveis são recusados.
	expect(t, w.attach(w.ana, id, "document", "setup.exe", []byte("MZ....."), nil), 422)
	// Vazio e acima do limite (5 MB no teste).
	expect(t, w.attach(w.ana, id, "document", "a.pdf", []byte{}, nil), 422)
	expect(t, w.attach(w.ana, id, "document", "grande.pdf", bytes.Repeat([]byte("a"), 6<<20), nil), 413)
	// Tipo inválido / sem arquivo.
	expect(t, w.attach(w.ana, id, "video", "a.mp4", []byte("x"), nil), 422)
	expect(t, w.ana.upload("/api/v1/conversations/"+id+"/messages", map[string]string{"id": uuid.NewString(), "kind": "document"}, "", nil), 422)
	// Nada disso gerou envio nem mensagem.
	if len(w.e.wa.Sent()) != 0 {
		t.Fatal("recusas não podem enviar nada")
	}
	// JSON não aceita anexo.
	expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", map[string]any{"id": uuid.NewString(), "kind": "image", "text": "x"}), 422)
	// Outra pessoa já assumiu: não vaza nem grava.
	expect(t, w.attach(w.ana, id, "document", "ok.pdf", []byte("%PDF-1.4 ok"), nil), 201)
	expect(t, w.attach(w.ana2, id, "document", "ok2.pdf", []byte("%PDF-1.4 ok"), nil), 409, "conversation_taken")
	expect(t, w.attach(w.bia, id, "document", "ok3.pdf", []byte("%PDF-1.4 ok"), nil), 404)
}

func TestSendAudioIsConvertedToOpus(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	w := setupWorld(t)
	w.receive("Cliente Voz", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Voz")["id"].(string)
	r := w.attach(w.ana, id, "audio", "gravacao.wav", testWAV(), map[string]string{"waveform": "[0.1,0.5,1,0.3]"})
	expect(t, r, 201)
	m := lastSent(t, w).Media
	if m.Kind != whatsapp.KindAudio || !strings.HasPrefix(m.MimeType, "audio/ogg") || !bytes.HasPrefix(m.Data, []byte("OggS")) {
		t.Fatalf("áudio deveria ir em Ogg/Opus: %s %q", m.MimeType, m.Data[:4])
	}
	if m.DurationSec != 1 || len(m.Waveform) != 4 || m.Waveform[2] != 100 {
		t.Fatalf("duração/forma de onda: %d %v", m.DurationSec, m.Waveform)
	}
	if r.body["kind"] != "audio" || r.body["durationSec"] != float64(1) || r.body["url"] == nil {
		t.Fatalf("mensagem de áudio: %v", r.body)
	}
	st, hd, body := w.ana.raw(r.body["url"].(string))
	if st != 200 || !strings.HasPrefix(hd.Get("Content-Type"), "audio/ogg") || !bytes.HasPrefix(body, []byte("OggS")) {
		t.Fatalf("download do áudio: %d %s", st, hd.Get("Content-Type"))
	}
	// Lixo não vira áudio.
	expect(t, w.attach(w.ana, id, "audio", "x.wav", []byte("não é áudio nenhum"), nil), 422)
}

func TestSendLocation(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Local", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Local")["id"].(string)
	r := w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", map[string]any{
		"id": uuid.NewString(), "kind": "location", "lat": -3.119, "lng": -60.021, "name": "Escritório", "address": "Av. Exemplo, 100"})
	expect(t, r, 201)
	s := lastSent(t, w)
	if s.Location == nil || s.Location.Lat != -3.119 || s.Location.Name != "Escritório" {
		t.Fatalf("localização enviada: %+v", s.Location)
	}
	if r.body["kind"] != "location" || r.body["lat"] != -3.119 {
		t.Fatalf("mensagem: %v", r.body)
	}
	for _, bad := range []map[string]any{{"lat": 91.0, "lng": 0.0}, {"lat": 0.0, "lng": 0.0}, {"lat": 1.0, "lng": 181.0}} {
		bad["id"], bad["kind"] = uuid.NewString(), "location"
		expect(t, w.ana.do("POST", "/api/v1/conversations/"+id+"/messages", bad), 422)
	}
}

func TestMediaSendFailureKeepsMessageAndRetryDoesNotDuplicate(t *testing.T) {
	w := setupWorld(t)
	w.receive("Cliente Falha", "folha de pagamento")
	id := mustConv(t, w.ana, "Cliente Falha")["id"].(string)
	data := testPNG(300, 200)
	msgID := uuid.NewString()
	f := map[string]string{"id": msgID, "kind": "image"}

	w.e.wa.FailSends(fake.SimulateFailure)
	r := w.ana.upload("/api/v1/conversations/"+id+"/messages", f, "a.png", data)
	expect(t, r, 201)
	if r.body["status"] != "failed" || r.body["url"] == nil {
		t.Fatalf("falha não deve apagar a mensagem nem o anexo: %v", r.body)
	}

	w.e.wa.FailSends(nil)
	// "Tentar de novo": o mesmo id; o arquivo é relido do armazenamento (nem precisa reenviar o corpo).
	r = w.ana.upload("/api/v1/conversations/"+id+"/messages", f, "a.png", data)
	expect(t, r, 200)
	if r.body["status"] != "sent" {
		t.Fatalf("reenvio: %v", r.body)
	}
	if n := len(w.e.wa.Sent()); n != 1 {
		t.Fatalf("deveria ter ido ao WhatsApp uma vez: %d", n)
	}
	if m := lastSent(t, w).Media; m == nil || !bytes.Equal(m.Data, data) {
		t.Fatal("o reenvio deveria usar o arquivo guardado")
	}
	// Repetir uma mensagem já enviada não duplica.
	expect(t, w.ana.upload("/api/v1/conversations/"+id+"/messages", f, "a.png", data), 200)
	msgs := w.ana.do("GET", "/api/v1/conversations/"+id+"/messages", nil).body["messages"].([]any)
	count := 0
	for _, m := range msgs {
		if m.(map[string]any)["kind"] == "image" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("uma imagem só: %d", count)
	}
	// Um id de outra conversa/pessoa não pode ser reaproveitado.
	w.receive("Outro Cliente", "folha de pagamento")
	other := mustConv(t, w.ana, "Outro Cliente")["id"].(string)
	expect(t, w.ana.upload("/api/v1/conversations/"+other+"/messages", f, "a.png", data), 409, "id_conflict")
}

func TestInboundMediaIsDownloadedStoredAndProtected(t *testing.T) {
	w := setupWorld(t)
	img := testPNG(64, 48)
	p := itoa(int(phoneSeq.Add(1)))
	m := inbound("M-"+p, p, "Cliente Recebe", "folha de pagamento, segue a foto")
	m.Kind = whatsapp.KindImage
	m.Payload = map[string]any{"mimeType": "image/png"}
	m.Download = func(context.Context) ([]byte, error) { return img, nil }
	w.e.wa.Receive(m)

	var url string
	eventually(t, "anexo disponível", func() bool {
		c := conv(t, w.ana, "Cliente Recebe")
		if c == nil {
			return false
		}
		msgs := w.ana.do("GET", "/api/v1/conversations/"+c["id"].(string)+"/messages", nil).body["messages"].([]any)
		for _, x := range msgs {
			if u, _ := x.(map[string]any)["url"].(string); u != "" {
				url = u
			}
		}
		return url != ""
	})
	st, hd, body := w.ana.raw(url)
	if st != 200 || !bytes.Equal(body, img) || hd.Get("Content-Type") != "image/png" {
		t.Fatalf("download: %d %s", st, hd.Get("Content-Type"))
	}
	if st, _, _ := w.bia.raw(url); st != 404 {
		t.Fatalf("outra equipe: %d", st)
	}
	// Chegou classificada pela legenda.
	if mustConv(t, w.ana, "Cliente Recebe")["segment"] != "folha" {
		t.Fatal("a legenda deveria classificar a conversa")
	}

	// Falha no download: a mensagem fica, marcada como indisponível, sem derrubar nada.
	p2 := itoa(int(phoneSeq.Add(1)))
	m2 := inbound("M-"+p2, p2, "Cliente Quebrado", "")
	m2.Kind = whatsapp.KindAudio
	m2.Payload = map[string]any{"durationSec": 3}
	m2.Download = func(context.Context) ([]byte, error) { return nil, errors.New("expirou") }
	w.e.wa.Receive(m2)
	eventually(t, "estado da mídia", func() bool {
		c := conv(t, w.admin, "Cliente Quebrado")
		return c != nil && c["lastMessage"].(map[string]any)["mediaState"] == "failed"
	})

	// Arquivo inválido (conteúdo não é imagem) é bloqueado.
	p3 := itoa(int(phoneSeq.Add(1)))
	m3 := inbound("M-"+p3, p3, "Cliente Ruim", "")
	m3.Kind = whatsapp.KindImage
	m3.Download = func(context.Context) ([]byte, error) { return []byte("<html>script</html>"), nil }
	w.e.wa.Receive(m3)
	eventually(t, "bloqueada", func() bool {
		c := conv(t, w.admin, "Cliente Ruim")
		return c != nil && c["lastMessage"].(map[string]any)["mediaState"] == "blocked"
	})
}
