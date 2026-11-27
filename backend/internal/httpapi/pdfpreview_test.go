package httpapi

// Prévia dos PDFs no balão (o alto da primeira página) e número de páginas.

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vyctinho77/chathm/backend/internal/whatsapp"
)

// testPDF monta um PDF de n páginas A4 com um retângulo avermelhado no alto.
func testPDF(n int) []byte {
	kids := make([]string, n)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i*2)
	}
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n)}
	for i := 0; i < n; i++ {
		c := "0.8 0.2 0.1 rg 0 642 595 200 re f"
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << >> >>", 4+i*2),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, x)
	return b.Bytes()
}

func TestPDFPreviewAndPages(t *testing.T) {
	for _, bin := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip("poppler-utils não instalado")
		}
	}
	w := setupWorld(t)
	w.receive("Cliente PDF Prévia", "oi")
	id := mustConv(t, w.ana, "Cliente PDF Prévia")["id"].(string)

	// PDF enviado pelo escritório: a mensagem já traz o número de páginas.
	r := w.attach(w.ana, id, "document", "edital.pdf", testPDF(3), nil)
	expect(t, r, 201)
	if r.body["pages"] != float64(3) {
		t.Fatalf("páginas = %v, esperado 3", r.body["pages"])
	}
	url := r.body["url"].(string)

	// A prévia é uma foto do alto da primeira página; a segunda leitura vem do que ficou guardado.
	st, hd, body := w.ana.raw(url + "?size=preview")
	if st != 200 || hd.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("prévia: %d %s", st, hd.Get("Content-Type"))
	}
	img, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil || img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Fatalf("prévia deveria ser 640x360, veio %v (%v)", img.Bounds(), err)
	}
	if rr, _, _, _ := img.At(20, 20).RGBA(); rr>>8 < 150 {
		t.Fatalf("o alto da página deveria ser avermelhado")
	}
	st2, _, body2 := w.ana.raw(url + "?size=preview")
	if st2 != 200 || !bytes.Equal(body, body2) {
		t.Fatalf("a segunda leitura deveria ser igual (guardada): %d", st2)
	}

	// Quem não enxerga a conversa não recebe a prévia (nem descobre que ela existe).
	if st, _, _ := w.bia.raw(url + "?size=preview"); st != 404 {
		t.Fatalf("outra equipe: %d, esperado 404", st)
	}
	// O arquivo inteiro continua sendo baixado como antes.
	if st, hd, b := w.ana.raw(url); st != 200 || !bytes.HasPrefix(b, []byte("%PDF")) || !strings.HasPrefix(hd.Get("Content-Disposition"), "attachment") {
		t.Fatalf("o PDF original mudou: %d", st)
	}

	// PDF que não dá para desenhar: sem prévia (404, a tela esconde o espaço), e o arquivo segue baixando.
	r = w.attach(w.ana, id, "document", "quebrado.pdf", []byte("%PDF-1.4\nlixo sem estrutura"), nil)
	expect(t, r, 201)
	if _, has := r.body["pages"]; has {
		t.Fatalf("PDF ilegível não deveria ter páginas: %v", r.body["pages"])
	}
	if st, _, _ := w.ana.raw(r.body["url"].(string) + "?size=preview"); st != 404 {
		t.Fatalf("PDF ilegível: prévia %d, esperado 404", st)
	}
	if st, _, _ := w.ana.raw(r.body["url"].(string)); st != 200 {
		t.Fatalf("PDF ilegível deve baixar normalmente: %d", st)
	}
}

func TestInboundPDFCarriesPagesFromWhatsApp(t *testing.T) {
	w := setupWorld(t)
	phone := "5592955550777"
	m := whatsapp.InboundMessage{ExternalID: "DOC-PAGES-1", ChatJID: phone + "@s.whatsapp.net", ContactWAID: phone, ContactPhone: "+" + phone,
		PushName: "Cliente Páginas", Timestamp: time.Now(), Kind: whatsapp.KindDocument,
		Payload: map[string]any{"fileName": "resumo.pdf", "mimeType": "application/pdf", "sizeBytes": 398000, "pages": 11}}
	w.e.wa.Receive(m)
	cid := mustConv(t, w.ana, "Cliente Páginas")["id"].(string)
	ms := msgList(t, w.ana, cid)
	if len(ms) != 1 || ms[0]["pages"] != float64(11) {
		t.Fatalf("páginas vindas do WhatsApp = %v", ms)
	}
}
