package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

// ErrNotFound: o objeto não existe.
var ErrNotFound = errors.New("storage: objeto não encontrado")

// Store guarda objetos privados. Nunca há URL pública: os arquivos só saem pela API,
// depois de verificada a autorização de quem pede.
type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// ---------------------------------------------------------------------------
// Disco local (desenvolvimento e instalações simples). Cada arquivo é cifrado
// com AES-256-GCM (a mesma chave dos demais segredos).

type FS struct {
	dir string
	box *secretbox.Box
}

func NewFS(dir string, box *secretbox.Box) (*FS, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &FS{dir: dir, box: box}, nil
}

func (f *FS) path(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("storage: chave inválida")
	}
	return filepath.Join(f.dir, filepath.FromSlash(key)), nil
}

func (f *FS) Put(_ context.Context, key string, data []byte) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	sealed, err := f.box.Seal(data, []byte(key))
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (f *FS) Get(_ context.Context, key string) ([]byte, error) {
	p, err := f.path(key)
	if err != nil {
		return nil, err
	}
	sealed, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f.box.Open(sealed, []byte(key))
}

func (f *FS) Delete(_ context.Context, key string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// S3 compatível (MinIO, AWS S3…). Bucket privado; cifragem em repouso do lado do
// servidor (SSE) fica a cargo do provedor — em produção habilite-a no bucket.

type S3 struct {
	c      *minio.Client
	bucket string
}

type S3Config struct {
	Endpoint, Region, Bucket, AccessKey, SecretKey string
	UseSSL                                         bool
}

func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	ok, err := c.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: verificando bucket: %w", err)
	}
	if !ok {
		if err := c.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("storage: criando bucket: %w", err)
		}
	}
	return &S3{c: c, bucket: cfg.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytesReader(data), int64(len(data)), minio.PutObjectOptions{})
	return err
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	o, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	b, err := io.ReadAll(o)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return b, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.c.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// Memory é um Store em memória para testes.
type Memory struct {
	mu sync.Mutex
	m  map[string][]byte
}

func NewMemory() *Memory { return &Memory{m: map[string][]byte{}} }

func (m *Memory) Put(_ context.Context, k string, d []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[k] = append([]byte(nil), d...)
	return nil
}
func (m *Memory) Get(_ context.Context, k string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.m[k]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}
func (m *Memory) Delete(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, k)
	return nil
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
