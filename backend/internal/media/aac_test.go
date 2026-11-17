package media

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func sineOpus(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	dst := filepath.Join(t.TempDir(), "s.ogg")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-ac", "1", "-c:a", "libopus", "-b:a", "32k", "-f", "ogg", dst).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg sem libopus: %v %s", err, out)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestToAACConverteOpusParaM4A(t *testing.T) {
	in := sineOpus(t)
	s := NewService(1<<20, nil)
	out, err := s.ToAAC(context.Background(), in, "")
	if err != nil {
		t.Fatal(err)
	}
	// MP4/M4A: "ftyp" nos bytes 4..8.
	if len(out) < 12 || string(out[4:8]) != "ftyp" {
		t.Fatalf("saída não parece MP4: %q", out[:min(len(out), 12)])
	}
	if bytes.Equal(out, in) {
		t.Fatal("deveria ter convertido")
	}
	// "moov" antes de "mdat" (faststart): toca sem baixar tudo.
	if bytes.Index(out, []byte("moov")) > bytes.Index(out, []byte("mdat")) {
		t.Fatal("moov deveria vir antes de mdat (faststart)")
	}
	if d := s.duration(context.Background(), writeTemp(t, out)); d != 1 {
		t.Fatalf("duração = %ds, esperava 1s", d)
	}
}

func writeTemp(t *testing.T, b []byte) string {
	p := filepath.Join(t.TempDir(), "x.m4a")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestToAACUsaOCache(t *testing.T) {
	in := sineOpus(t)
	s := NewService(1<<20, nil)
	first, err := s.ToAAC(context.Background(), in, "chave-teste")
	if err != nil {
		t.Fatal(err)
	}
	// Com o conteúdo no cache, nem precisa do ffmpeg: aponta para um programa que não existe.
	s.FFmpeg = "ffmpeg-que-nao-existe"
	again, err := s.ToAAC(context.Background(), []byte("lixo"), "chave-teste")
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("cache não foi usado: err=%v", err)
	}
}

func TestToAACSemFFmpegDevolveIndisponivel(t *testing.T) {
	s := NewService(1<<20, nil)
	s.FFmpeg = "ffmpeg-que-nao-existe"
	if _, err := s.ToAAC(context.Background(), []byte("x"), ""); err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("esperava ErrUnavailable, veio %v", err)
	}
}

func TestToAACLixoDaErro(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg não instalado")
	}
	s := NewService(1<<20, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := s.ToAAC(ctx, []byte("isto não é áudio"), ""); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestAACCacheDescartaOMaisAntigo(t *testing.T) {
	for i := 0; i < aacCacheMaxEntries+5; i++ {
		aacCachePut(string(rune('A'+i%26))+string(rune('a'+i/26))+"-cap", []byte{1, 2, 3})
	}
	aacCache.mu.Lock()
	n := len(aacCache.data)
	aacCache.mu.Unlock()
	if n > aacCacheMaxEntries {
		t.Fatalf("cache com %d entradas, limite %d", n, aacCacheMaxEntries)
	}
}
