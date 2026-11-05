package secretbox

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newKey(t *testing.T) string {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return base64.StdEncoding.EncodeToString(k)
}

func TestSealOpen(t *testing.T) {
	b, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := b.Seal([]byte("segredo"), []byte("mfa:1"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := b.Open(ct, []byte("mfa:1"))
	if err != nil || !bytes.Equal(pt, []byte("segredo")) {
		t.Fatalf("open = %q, %v", pt, err)
	}
	if _, err := b.Open(ct, []byte("mfa:2")); err == nil {
		t.Fatal("AAD diferente deveria falhar")
	}
	ct[len(ct)-1] ^= 1
	if _, err := b.Open(ct, []byte("mfa:1")); err == nil {
		t.Fatal("ciphertext adulterado deveria falhar")
	}
	other, _ := New(newKey(t))
	ct2, _ := b.Seal([]byte("x"), nil)
	if _, err := other.Open(ct2, nil); err == nil {
		t.Fatal("chave diferente deveria falhar")
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	for _, k := range []string{"", "não-base64", base64.StdEncoding.EncodeToString([]byte("curta"))} {
		if _, err := New(k); err == nil {
			t.Fatalf("chave %q deveria ser rejeitada", k)
		}
	}
}
