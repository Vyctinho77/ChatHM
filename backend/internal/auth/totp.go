package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238): SHA-1, 6 dígitos, período de 30s — compatível com
// Google Authenticator, Microsoft Authenticator, 1Password, Authy etc.
const (
	totpPeriod = 30
	totpDigits = 6
	totpSkew   = 1 // aceita ±1 período para tolerar relógio dessincronizado
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, code%1_000_000)
}

// validateTOTP verifica o código e devolve o passo (time step) correspondente.
// Só aceita passos maiores que lastStep, impedindo reuso do mesmo código.
func validateTOTP(secretB32, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	secret, err := b32.DecodeString(strings.ToUpper(secretB32))
	if err != nil {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for s := cur - totpSkew; s <= cur+totpSkew; s++ {
		if s <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func otpauthURL(issuer, account, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpPeriod))
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

// GenerateTOTP calcula o código para o instante t (útil em testes e diagnósticos).
func GenerateTOTP(secretB32 string, t time.Time) (string, error) {
	secret, err := b32.DecodeString(strings.ToUpper(secretB32))
	if err != nil {
		return "", err
	}
	return totpAt(secret, t.Unix()/totpPeriod), nil
}
