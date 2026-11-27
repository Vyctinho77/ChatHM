package media

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os/exec"
	"strings"
	"testing"
)

// miniPDF monta um PDF de n páginas (w x h pontos) com um retângulo colorido no alto de cada uma.
func miniPDF(n, w, h int) []byte {
	var objs []string
	kids := make([]string, n)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i*2)
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	for i := 0; i < n; i++ {
		pg := 3 + i*2
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Contents %d 0 R /Resources << >> >>", w, h, pg+1))
		c := fmt.Sprintf("0.8 0.2 0.1 rg 0 %d %d %d re f", h-200, w, 200)
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c))
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

func needPoppler(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"pdftoppm", "pdfinfo"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skip("poppler-utils não instalado")
		}
	}
}

func TestPDFPreviewIsTheTopOfThePageAndCountsPages(t *testing.T) {
	needPoppler(t)
	jpg, pages, err := PDFPreview(context.Background(), miniPDF(3, 595, 842))
	if err != nil || pages != 3 {
		t.Fatalf("pages=%d err=%v", pages, err)
	}
	img, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 360 {
		t.Fatalf("tamanho = %v, esperado 640x360 (só o alto da página)", b)
	}
	// O retângulo avermelhado está no alto da página: o canto superior esquerdo não é branco.
	r, g, bl, _ := img.At(20, 20).RGBA()
	if r>>8 < 150 || g>>8 > 120 || bl>>8 > 90 {
		t.Fatalf("o alto da página deveria ser vermelho, veio rgb(%d,%d,%d)", r>>8, g>>8, bl>>8)
	}
}

func TestPDFPreviewShortPageIsNotStretched(t *testing.T) {
	needPoppler(t)
	jpg, _, err := PDFPreview(context.Background(), miniPDF(1, 842, 200))
	if err != nil {
		t.Fatal(err)
	}
	img, _ := jpeg.Decode(bytes.NewReader(jpg))
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() >= 360 {
		t.Fatalf("página baixa deveria dar uma faixa mais baixa que 360, veio %v", b)
	}
}

func TestPDFPagesAndRefusals(t *testing.T) {
	needPoppler(t)
	if n := PDFPages(context.Background(), miniPDF(7, 595, 842)); n != 7 {
		t.Fatalf("páginas = %d", n)
	}
	for name, data := range map[string][]byte{
		"vazio":         nil,
		"texto":         []byte("isto não é um PDF"),
		"quebrado":      []byte("%PDF-1.4\nlixo sem estrutura"),
		"grande demais": append([]byte("%PDF-1.4\n"), make([]byte, pdfMaxBytes+1)...),
	} {
		if n := PDFPages(context.Background(), data); n != 0 {
			t.Errorf("%s: páginas = %d, esperado 0", name, n)
		}
		if _, _, err := PDFPreview(context.Background(), data); err == nil {
			t.Errorf("%s: deveria recusar", name)
		}
	}
}

func TestPDFPreviewHonorsCancelledContext(t *testing.T) {
	needPoppler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := PDFPreview(ctx, miniPDF(1, 595, 842)); err == nil {
		t.Fatal("contexto cancelado deveria falhar")
	}
}
