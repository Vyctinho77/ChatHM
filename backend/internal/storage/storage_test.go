package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyctinho77/chathm/backend/internal/platform/secretbox"
)

func TestFSEncryptsAtRestAndRejectsBadKeys(t *testing.T) {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, err := secretbox.New(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fs, err := NewFS(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	secret := []byte("CONTEÚDO CONFIDENCIAL DO CLIENTE")
	if err := fs.Put(ctx, "attachments/a1", secret); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "attachments", "a1"))
	if bytes.Contains(raw, []byte("CONFIDENCIAL")) {
		t.Fatal("o arquivo em disco deveria estar cifrado")
	}
	got, err := fs.Get(ctx, "attachments/a1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("leitura: %v", err)
	}
	// Trocar o arquivo de lugar (outra chave) não decifra: a chave do objeto vai no AAD.
	_ = os.WriteFile(filepath.Join(dir, "attachments", "a2"), raw, 0o600)
	if _, err := fs.Get(ctx, "attachments/a2"); err == nil {
		t.Fatal("objeto copiado para outra chave não deveria abrir")
	}
	if _, err := fs.Get(ctx, "attachments/nao-existe"); err != ErrNotFound {
		t.Fatalf("esperava ErrNotFound: %v", err)
	}
	for _, bad := range []string{"../fora", "/etc/passwd", "a/../../b", ""} {
		if err := fs.Put(ctx, bad, []byte("x")); err == nil {
			t.Fatalf("chave %q deveria ser recusada", bad)
		}
	}
	if err := fs.Delete(ctx, "attachments/a1"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete(ctx, "attachments/a1"); err != nil {
		t.Fatal("apagar de novo é idempotente")
	}
}
