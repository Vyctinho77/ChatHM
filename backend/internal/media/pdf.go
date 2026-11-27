package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// Prévia de PDF para o balão da conversa, como no WhatsApp: o alto da primeira página como foto e o número de páginas.
// Usa o poppler (pdftoppm/pdfinfo), que lê qualquer PDF (inclusive escaneado). O arquivo é do cliente, então: tamanho máximo,
// prazo curto, no máximo duas execuções ao mesmo tempo, diretório temporário próprio e ambiente vazio.
const (
	pdfPreviewWidth  = 640 // 2x a largura do balão (288 px), nítido em tela de alta densidade
	pdfPreviewHeight = 360 // só o alto da página: o balão mostra uma faixa
	pdfMaxBytes      = 40 << 20
	pdfTimeout       = 15 * time.Second
)

// ErrNoPDFTools: o poppler não está instalado (o ChatHM segue sem prévia de PDF).
var ErrNoPDFTools = errors.New("media: pdftoppm/pdfinfo indisponíveis")

var pdfSlots = make(chan struct{}, 2)

var pagesRE = regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`)

func pdfTool(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", ErrNoPDFTools
	}
	return p, nil
}

// isPDF confere o começo do arquivo ("%PDF-" nos primeiros 1024 bytes, como os leitores fazem).
func isPDF(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(head, []byte("%PDF-"))
}

// pdfWork grava o PDF numa pasta temporária própria, roda f e apaga tudo; respeita o limite de execuções simultâneas.
func pdfWork(ctx context.Context, data []byte, f func(ctx context.Context, dir, file string) error) error {
	if len(data) == 0 || len(data) > pdfMaxBytes || !isPDF(data) {
		return errors.New("media: não é um PDF utilizável")
	}
	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "chathm-pdf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return err
	}
	return f(ctx, dir, file)
}

func run(ctx context.Context, dir string, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LC_ALL=C"}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PDFPages devolve o número de páginas (0 quando não dá para saber: arquivo com senha, quebrado ou sem poppler).
func PDFPages(ctx context.Context, data []byte) int {
	info, err := pdfTool("pdfinfo")
	if err != nil {
		return 0
	}
	pages := 0
	_ = pdfWork(ctx, data, func(ctx context.Context, dir, file string) error {
		out, err := run(ctx, dir, info, file)
		if err != nil {
			return err
		}
		if m := pagesRE.FindSubmatch(out); m != nil {
			pages, _ = strconv.Atoi(string(m[1]))
		}
		return nil
	})
	if pages < 0 || pages > 100000 {
		return 0
	}
	return pages
}

// PDFPreview desenha o alto da primeira página (640 px de largura, até 360 de altura) como JPEG e conta as páginas.
func PDFPreview(ctx context.Context, data []byte) (jpg []byte, pages int, err error) {
	toppm, err := pdfTool("pdftoppm")
	if err != nil {
		return nil, 0, err
	}
	err = pdfWork(ctx, data, func(ctx context.Context, dir, file string) error {
		if _, err := run(ctx, dir, toppm, "-jpeg", "-jpegopt", "quality=72", "-f", "1", "-l", "1",
			"-scale-to-x", strconv.Itoa(pdfPreviewWidth), "-scale-to-y", "-1", "-x", "0", "-y", "0",
			"-W", strconv.Itoa(pdfPreviewWidth), "-H", strconv.Itoa(pdfPreviewHeight), "-singlefile", file, filepath.Join(dir, "out")); err != nil {
			return err
		}
		jpg, err = os.ReadFile(filepath.Join(dir, "out.jpg"))
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return jpg, PDFPages(ctx, data), nil
}
