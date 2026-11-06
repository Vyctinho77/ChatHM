// Package media valida, normaliza e verifica anexos antes de guardá-los ou enviá-los.
//
// Regras: o tipo vem do conteúdo (não do nome nem do cabeçalho enviado), há limite de
// tamanho, extensões executáveis são recusadas, áudio é convertido para Ogg/Opus (o
// formato das mensagens de voz do WhatsApp) e, havendo clamd configurado, todo arquivo
// é verificado por antivírus.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Kind string

const (
	Image    Kind = "image"
	Audio    Kind = "audio"
	Document Kind = "document"
)

// Erros de validação: a mensagem é mostrada ao usuário.
type RejectError struct{ Msg string }

func (e *RejectError) Error() string { return e.Msg }

func reject(format string, a ...any) error { return &RejectError{Msg: fmt.Sprintf(format, a...)} }

// ErrUnavailable: serviço de apoio indisponível (ex.: ffmpeg ausente).
var ErrUnavailable = errors.New("media: recurso indisponível")

// Processed é um anexo pronto para guardar/enviar.
type Processed struct {
	Kind        Kind
	Data        []byte
	MimeType    string
	FileName    string
	Width       int
	Height      int
	DurationSec int
	ScanStatus  string // clean | skipped
}

type Service struct {
	Max     int64
	Scanner *Scanner // opcional
	// FFmpeg/FFprobe: caminhos (vazio = procurar no PATH).
	FFmpeg  string
	FFprobe string
}

func NewService(max int64, scanner *Scanner) *Service {
	return &Service{Max: max, Scanner: scanner, FFmpeg: "ffmpeg", FFprobe: "ffprobe"}
}

var blockedExt = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true, ".msi": true, ".dll": true,
	".js": true, ".jse": true, ".vbs": true, ".vbe": true, ".ps1": true, ".jar": true, ".apk": true,
	".lnk": true, ".sh": true, ".hta": true, ".wsf": true, ".reg": true, ".pif": true, ".cpl": true,
}

// CleanFileName remove caminhos e caracteres de controle; limita o tamanho.
func CleanFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "." || out == ".." || out == "/" || out == "" {
		return "arquivo"
	}
	if rs := []rune(out); len(rs) > 120 {
		ext := filepath.Ext(out)
		if len([]rune(ext)) > 12 {
			ext = ""
		}
		out = string(rs[:120-len([]rune(ext))]) + ext
	}
	return out
}

// Process valida e normaliza. `kind` é o que o usuário pretendia enviar.
//
// Imagens: sem `hd`, a maior dimensão é reduzida a 1600 px (como o WhatsApp faz no envio comum);
// com `hd` o original é enviado, respeitando o limite de tamanho.
func (s *Service) Process(ctx context.Context, kind Kind, data []byte, fileName string, hd bool) (Processed, error) {
	if len(data) == 0 {
		return Processed{}, reject("O arquivo está vazio.")
	}
	if int64(len(data)) > s.Max {
		return Processed{}, reject("O arquivo passa do limite de %d MB.", s.Max>>20)
	}
	name := CleanFileName(fileName)
	if blockedExt[strings.ToLower(filepath.Ext(name))] {
		return Processed{}, reject("Este tipo de arquivo não pode ser enviado por segurança.")
	}
	p := Processed{Kind: kind, Data: data, FileName: name}
	sniffed := http.DetectContentType(data)

	switch kind {
	case Image:
		switch {
		case strings.HasPrefix(sniffed, "image/jpeg"), strings.HasPrefix(sniffed, "image/png"), isWebP(data):
		default:
			return Processed{}, reject("Formato de imagem não suportado. Use JPEG, PNG ou WebP, ou envie como documento.")
		}
		if isWebP(data) {
			p.MimeType = "image/webp"
		} else {
			p.MimeType = strings.SplitN(sniffed, ";", 2)[0]
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return Processed{}, reject("A imagem está corrompida.")
		}
		if cfg.Width*cfg.Height > 100_000_000 {
			return Processed{}, reject("A imagem é grande demais.")
		}
		p.Width, p.Height = cfg.Width, cfg.Height
		if !hd && (cfg.Width > maxStdDim || cfg.Height > maxStdDim) {
			if err := shrink(&p); err != nil {
				return Processed{}, reject("A imagem está corrompida.")
			}
		}
	case Audio:
		ogg, dur, err := s.toOpus(ctx, data)
		if err != nil {
			return Processed{}, err
		}
		p.Data, p.MimeType, p.DurationSec = ogg, "audio/ogg; codecs=opus", dur
		p.FileName = "audio.ogg"
	case Document:
		p.MimeType = strings.SplitN(sniffed, ";", 2)[0]
		if p.MimeType == "application/octet-stream" || strings.HasPrefix(p.MimeType, "text/plain") {
			// O sniffing é genérico demais para estes; a extensão só orienta o download.
			if t := extMime[strings.ToLower(filepath.Ext(name))]; t != "" {
				p.MimeType = t
			}
		}
	default:
		return Processed{}, reject("Tipo de anexo inválido.")
	}

	p.ScanStatus = "skipped"
	if s.Scanner != nil {
		if err := s.Scanner.Scan(ctx, p.Data); err != nil {
			return Processed{}, err
		}
		p.ScanStatus = "clean"
	}
	return p, nil
}

// ProcessInbound normaliza mídia recebida: o tipo vem do conteúdo; áudio é guardado como veio
// (os navegadores tocam Ogg/Opus; outros formatos são convertidos).
func (s *Service) ProcessInbound(ctx context.Context, kind Kind, data []byte, fileName, declaredMime string) (Processed, error) {
	if len(data) == 0 {
		return Processed{}, reject("Arquivo vazio.")
	}
	if int64(len(data)) > s.Max {
		return Processed{}, reject("Arquivo acima do limite.")
	}
	name := CleanFileName(fileName)
	p := Processed{Kind: kind, Data: data, FileName: name, MimeType: strings.SplitN(http.DetectContentType(data), ";", 2)[0]}
	switch kind {
	case Image:
		if !(strings.HasPrefix(p.MimeType, "image/jpeg") || strings.HasPrefix(p.MimeType, "image/png") || isWebP(data) || strings.HasPrefix(p.MimeType, "image/gif")) {
			return Processed{}, reject("Imagem recebida em formato desconhecido.")
		}
		if isWebP(data) {
			p.MimeType = "image/webp"
		}
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			p.Width, p.Height = cfg.Width, cfg.Height
		}
	case Audio:
		if ogg, dur, err := s.toOpus(ctx, data); err == nil {
			p.Data, p.MimeType, p.DurationSec, p.FileName = ogg, "audio/ogg; codecs=opus", dur, "audio.ogg"
		} else if errors.Is(err, ErrUnavailable) {
			return Processed{}, err
		} else {
			return Processed{}, reject("Áudio recebido inválido.")
		}
	default:
		if t := extMime[strings.ToLower(filepath.Ext(name))]; t != "" && (p.MimeType == "application/octet-stream" || strings.HasPrefix(p.MimeType, "text/plain")) {
			p.MimeType = t
		}
	}
	p.ScanStatus = "skipped"
	if s.Scanner != nil {
		if err := s.Scanner.Scan(ctx, p.Data); err != nil {
			return Processed{}, err
		}
		p.ScanStatus = "clean"
	}
	return p, nil
}

var extMime = map[string]string{
	".pdf": "application/pdf", ".txt": "text/plain", ".csv": "text/csv", ".xml": "application/xml",
	".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls": "application/vnd.ms-excel", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt": "application/vnd.ms-powerpoint", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".zip": "application/zip", ".ofx": "application/x-ofx", ".json": "application/json",
}

func isWebP(b []byte) bool {
	return len(b) > 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

// toOpus converte qualquer áudio que o ffmpeg decodifique em Ogg/Opus mono e mede a duração.
func (s *Service) toOpus(ctx context.Context, in []byte) ([]byte, int, error) {
	ff, err := exec.LookPath(s.FFmpeg)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: ffmpeg não instalado", ErrUnavailable)
	}
	dir, err := os.MkdirTemp("", "chathm-audio-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	src, dst := filepath.Join(dir, "in"), filepath.Join(dir, "out.ogg")
	if err := os.WriteFile(src, in, 0o600); err != nil {
		return nil, 0, err
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Opus mono (gravação do navegador, mensagem de voz do WhatsApp) só troca de
	// "envelope" para Ogg, sem decodificar nem codificar de novo: zero perda de qualidade.
	// Qualquer outro formato é convertido (reencodar é inevitável aqui).
	copied := false
	if s.isMonoOpus(cctx, src) {
		cmd := exec.CommandContext(cctx, ff, "-nostdin", "-v", "error", "-y", "-i", src, "-vn", "-map_metadata", "-1",
			"-c:a", "copy", "-f", "ogg", dst)
		copied = cmd.Run() == nil
	}
	if !copied {
		cmd := exec.CommandContext(cctx, ff, "-nostdin", "-v", "error", "-y", "-i", src, "-vn", "-map_metadata", "-1",
			"-ac", "1", "-ar", "48000", "-c:a", "libopus", "-b:a", "48k", "-application", "audio", "-f", "ogg", dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = out
			return nil, 0, reject("Não foi possível processar o áudio.")
		}
	}
	b, err := os.ReadFile(dst)
	if err != nil || len(b) == 0 {
		return nil, 0, reject("Não foi possível processar o áudio.")
	}
	return b, s.duration(cctx, dst), nil
}

// isMonoOpus diz se o primeiro stream de áudio já é Opus mono (sem ffprobe: false).
func (s *Service) isMonoOpus(ctx context.Context, path string) bool {
	fp, err := exec.LookPath(s.FFprobe)
	if err != nil {
		return false
	}
	out, err := exec.CommandContext(ctx, fp, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name,channels", "-of", "csv=p=0", path).Output()
	return err == nil && strings.TrimSpace(string(out)) == "opus,1"
}

func (s *Service) duration(ctx context.Context, path string) int {
	fp, err := exec.LookPath(s.FFprobe)
	if err != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, fp, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f < 0 {
		return 0
	}
	return int(f + 0.5)
}

const maxStdDim = 1600

// shrink reduz a imagem mantendo a proporção. PNG continua PNG; os demais viram JPEG.
func shrink(p *Processed) error {
	src, _, err := image.Decode(bytes.NewReader(p.Data))
	if err != nil {
		return err
	}
	w, h := p.Width, p.Height
	scale := float64(maxStdDim) / float64(max(w, h))
	nw, nh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if p.MimeType == "image/png" {
		err = png.Encode(&buf, dst)
	} else {
		p.MimeType = "image/jpeg"
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82})
	}
	if err != nil {
		return err
	}
	p.Data, p.Width, p.Height = buf.Bytes(), nw, nh
	return nil
}
