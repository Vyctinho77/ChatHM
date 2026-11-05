package auth

import (
	"strings"
	"testing"
	"time"
)

var fastHasher = Hasher{Params: Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}}

func TestHashVerify(t *testing.T) {
	h, err := fastHasher.Hash("senha-muito-segura")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") || strings.Contains(h, "senha-muito-segura") {
		t.Fatalf("formato inesperado: %s", h)
	}
	if ok, rehash, err := fastHasher.Verify("senha-muito-segura", h); !ok || rehash || err != nil {
		t.Fatalf("verify = %v %v %v", ok, rehash, err)
	}
	if ok, _, _ := fastHasher.Verify("senha-errada-123", h); ok {
		t.Fatal("senha errada aceita")
	}
	h2, _ := fastHasher.Hash("senha-muito-segura")
	if h == h2 {
		t.Fatal("salt deveria tornar os hashes diferentes")
	}
	stronger := Hasher{Params: Argon2Params{Memory: 2048, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}}
	if ok, rehash, _ := stronger.Verify("senha-muito-segura", h); !ok || !rehash {
		t.Fatal("deveria pedir rehash quando parâmetros mudam")
	}
	if _, _, err := fastHasher.Verify("x", "$2a$10$bcrypt"); err == nil {
		t.Fatal("hash inválido deveria falhar")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("curta") == nil {
		t.Fatal("senha curta aceita")
	}
	if ValidatePassword(strings.Repeat("a", 129)) == nil {
		t.Fatal("senha longa aceita")
	}
	if err := ValidatePassword("ação-é-válida"); err != nil {
		t.Fatal(err)
	}
}

func TestTOTP(t *testing.T) {
	// Vetor da RFC 6238 (SHA-1, segredo "12345678901234567890", T=59 → 94287082 com 8 dígitos).
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	now := time.Unix(59, 0)
	if got := totpAt([]byte("12345678901234567890"), 1); got != "287082" {
		t.Fatalf("totp = %s", got)
	}
	step, ok := validateTOTP(secret, "287082", now, 0)
	if !ok || step != 1 {
		t.Fatalf("validação falhou: %d %v", step, ok)
	}
	if _, ok := validateTOTP(secret, "287082", now, step); ok {
		t.Fatal("reuso do mesmo código deveria falhar")
	}
	if _, ok := validateTOTP(secret, "000000", now, 0); ok {
		t.Fatal("código errado aceito")
	}
	if _, ok := validateTOTP(secret, "287082", time.Unix(59+120, 0), 0); ok {
		t.Fatal("código expirado aceito")
	}
	u := otpauthURL("ChatHM", "admin@x.com", secret)
	if !strings.HasPrefix(u, "otpauth://totp/ChatHM:admin@x.com?") {
		t.Fatalf("url = %s", u)
	}
}
