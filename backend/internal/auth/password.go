package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2Params define o custo do Argon2id. Os parâmetros ficam codificados no hash,
// então podem ser elevados no futuro sem invalidar senhas existentes (rehash no login).
type Argon2Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

// DefaultParams: 64 MiB, t=3, p=2 — acima do mínimo recomendado pela OWASP.
var DefaultParams = Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 2, SaltLen: 16, KeyLen: 32}

const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

var (
	ErrPasswordTooShort = fmt.Errorf("a senha deve ter pelo menos %d caracteres", MinPasswordLen)
	ErrPasswordTooLong  = fmt.Errorf("a senha deve ter no máximo %d caracteres", MaxPasswordLen)
	errInvalidHash      = errors.New("hash de senha em formato inválido")
)

// ValidatePassword aplica a política mínima de senha.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLen {
		return ErrPasswordTooLong
	}
	return nil
}

type Hasher struct {
	Params Argon2Params
}

func NewHasher() Hasher { return Hasher{Params: DefaultParams} }

// Hash devolve a senha no formato PHC: $argon2id$v=19$m=..,t=..,p=..$salt$hash
func (h Hasher) Hash(pw string) (string, error) {
	p := h.Params
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify compara a senha com o hash em tempo constante. needsRehash indica que
// os parâmetros do hash armazenado diferem dos atuais.
func (h Hasher) Verify(pw, encoded string) (ok, needsRehash bool, err error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(key)))
	ok = subtle.ConstantTimeCompare(got, key) == 1
	needsRehash = p.Memory != h.Params.Memory || p.Iterations != h.Params.Iterations ||
		p.Parallelism != h.Params.Parallelism || uint32(len(key)) != h.Params.KeyLen
	return ok, needsRehash, nil
}

func decodeHash(encoded string) (Argon2Params, []byte, []byte, error) {
	var p Argon2Params
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, errInvalidHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return p, nil, nil, errInvalidHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return p, nil, nil, errInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, errInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return p, nil, nil, errInvalidHash
	}
	p.SaltLen, p.KeyLen = uint32(len(salt)), uint32(len(key))
	return p, salt, key, nil
}
