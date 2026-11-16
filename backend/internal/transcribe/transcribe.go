// Package transcribe transcreve áudios (mensagens de voz) para texto, localmente e sem custo, com o whisper.cpp (código aberto).
// Nada sai do servidor: o áudio é convertido pelo ffmpeg e lido pelo whisper-cli, e só o texto volta. Sem o programa ou o modelo
// instalados, New devolve nil e o recurso fica desligado (o resto do sistema não muda).
package transcribe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Bin        string        // whisper-cli (caminho ou nome no PATH); vazio = desligado
	Model      string        // arquivo do modelo ggml (ex.: ggml-small-q5_1.bin); vazio = desligado
	FFmpeg     string        // padrão "ffmpeg"
	Lang       string        // idioma falado (padrão "pt"); "auto" detecta
	Threads    int           // padrão: metade dos núcleos (mínimo 1), para não travar o resto do sistema
	MaxSeconds int           // áudios mais longos que isto não são transcritos (padrão 600)
	Timeout    time.Duration // por áudio (padrão 5 min)
}

type Service struct {
	cfg Config
	sem chan struct{} // um áudio por vez: a transcrição usa os núcleos e muita memória
}

// ErrTooLong: o áudio passa do limite configurado.
var ErrTooLong = errors.New("transcribe: áudio longo demais")

// New devolve nil se o programa ou o modelo não existem (recurso desligado).
func New(c Config) *Service {
	if c.Bin == "" || c.Model == "" {
		return nil
	}
	bin, err := exec.LookPath(c.Bin)
	if err != nil {
		return nil
	}
	if st, err := os.Stat(c.Model); err != nil || st.IsDir() {
		return nil
	}
	c.Bin = bin
	if c.FFmpeg == "" {
		c.FFmpeg = "ffmpeg"
	}
	if c.Lang == "" {
		c.Lang = "pt"
	}
	if c.Threads <= 0 {
		c.Threads = max(1, runtime.NumCPU()/2)
	}
	if c.MaxSeconds <= 0 {
		c.MaxSeconds = 600
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Minute
	}
	return &Service{cfg: c, sem: make(chan struct{}, 1)}
}

// MaxSeconds: o maior áudio que será transcrito.
func (s *Service) MaxSeconds() int { return s.cfg.MaxSeconds }

// Transcribe devolve o texto falado no áudio ("" se não há fala). durationSec <= 0 = duração desconhecida.
func (s *Service) Transcribe(ctx context.Context, audio []byte, durationSec int) (string, error) {
	if durationSec > s.cfg.MaxSeconds {
		return "", ErrTooLong
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "chathm-transcribe-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	in, wav := filepath.Join(dir, "in"), filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(in, audio, 0o600); err != nil {
		return "", err
	}
	// O whisper lê WAV de 16 kHz, mono. -t corta no limite caso a duração informada esteja errada.
	conv := exec.CommandContext(ctx, s.cfg.FFmpeg, "-nostdin", "-y", "-loglevel", "error", "-i", in, "-t", strconv.Itoa(s.cfg.MaxSeconds), "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	if out, err := conv.CombinedOutput(); err != nil {
		return "", fmt.Errorf("transcribe: ffmpeg: %w: %s", err, tail(out))
	}
	// -nt sem horários, -np só o texto no stdout.
	var stdout, stderr bytes.Buffer
	run := exec.CommandContext(ctx, s.cfg.Bin, "-m", s.cfg.Model, "-f", wav, "-l", s.cfg.Lang, "-t", strconv.Itoa(s.cfg.Threads), "-nt", "-np")
	run.Stdout, run.Stderr = &stdout, &stderr
	if err := run.Run(); err != nil {
		return "", fmt.Errorf("transcribe: whisper: %w: %s", err, tail(stderr.Bytes()))
	}
	return clean(stdout.String()), nil
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}

// Marcadores que o whisper escreve no lugar de silêncio, música e ruído (variam com o idioma): não são fala.
var noise = regexp.MustCompile(`(?i)[\[(]\s*(blank_audio|silêncio|silencio|música|musica|music|ruído|ruido|noise|aplausos|applause|risos|laughter|inaudível|inaudivel)\s*[\])]`)

// clean junta as linhas num texto corrido e tira os marcadores de silêncio/música.
func clean(raw string) string {
	raw = noise.ReplaceAllString(raw, " ")
	var parts []string
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
