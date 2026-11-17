package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Áudio para aparelhos antigos. O ChatHM guarda mensagens de voz em Ogg/Opus (o formato do WhatsApp), que navegadores
// mais velhos (principalmente iPhones antigos) não tocam. Para esses, o áudio é convertido para AAC em MP4 (.m4a), que todo
// aparelho toca. A conversão é feita só quando alguém pede (?format=m4a), sem mexer no arquivo guardado.

const (
	aacCacheMaxBytes   = 24 << 20 // memória máxima do cache de conversões
	aacCacheMaxEntries = 64
)

// A VPS tem um núcleo só: no máximo duas conversões ao mesmo tempo; as outras esperam (ou desistem se a requisição for cancelada).
var aacSlots = make(chan struct{}, 2)

var aacCache = struct {
	mu    sync.Mutex
	data  map[string][]byte
	order []string // do mais antigo para o mais novo
	size  int
}{data: map[string][]byte{}}

func aacCacheGet(key string) ([]byte, bool) {
	if key == "" {
		return nil, false
	}
	aacCache.mu.Lock()
	defer aacCache.mu.Unlock()
	b, ok := aacCache.data[key]
	return b, ok
}

func aacCachePut(key string, b []byte) {
	if key == "" || len(b) > aacCacheMaxBytes/2 {
		return
	}
	aacCache.mu.Lock()
	defer aacCache.mu.Unlock()
	if _, ok := aacCache.data[key]; ok {
		return
	}
	for len(aacCache.order) > 0 && (aacCache.size+len(b) > aacCacheMaxBytes || len(aacCache.order) >= aacCacheMaxEntries) {
		old := aacCache.order[0]
		aacCache.order = aacCache.order[1:]
		aacCache.size -= len(aacCache.data[old])
		delete(aacCache.data, old)
	}
	aacCache.data[key] = b
	aacCache.order = append(aacCache.order, key)
	aacCache.size += len(b)
}

// ToAAC converte um áudio (normalmente Ogg/Opus) para AAC mono em MP4, com o índice no começo do arquivo (toca antes de baixar tudo e
// aceita avançar). `cacheKey` identifica o conteúdo (o SHA-256 do anexo); vazio = sem cache.
func (s *Service) ToAAC(ctx context.Context, in []byte, cacheKey string) ([]byte, error) {
	if b, ok := aacCacheGet(cacheKey); ok {
		return b, nil
	}
	ff, err := exec.LookPath(s.FFmpeg)
	if err != nil {
		return nil, fmt.Errorf("%w: ffmpeg não instalado", ErrUnavailable)
	}
	select {
	case aacSlots <- struct{}{}:
		defer func() { <-aacSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if b, ok := aacCacheGet(cacheKey); ok { // outra requisição pode ter terminado enquanto esta esperava
		return b, nil
	}
	dir, err := os.MkdirTemp("", "chathm-aac-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	src, dst := filepath.Join(dir, "in"), filepath.Join(dir, "out.m4a")
	if err := os.WriteFile(src, in, 0o600); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, ff, "-nostdin", "-v", "error", "-y", "-i", src, "-vn", "-map_metadata", "-1",
		"-ac", "1", "-c:a", "aac", "-b:a", "64k", "-movflags", "+faststart", "-f", "mp4", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("media: aac: %w: %s", err, tailBytes(out, 300))
	}
	b, err := os.ReadFile(dst)
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("media: aac: saída vazia")
	}
	aacCachePut(cacheKey, b)
	return b, nil
}

func tailBytes(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}
