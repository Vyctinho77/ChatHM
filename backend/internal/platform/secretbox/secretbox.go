// Package secretbox cifra segredos em repouso com AES-256-GCM.
//
// Formato: versão (1 byte) || nonce (12 bytes) || ciphertext+tag.
// O AAD (dados associados) amarra o ciphertext ao seu contexto (ex.: "mfa:<user_id>"),
// impedindo que um valor seja copiado para outra linha e aceito.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const version byte = 1

var ErrDecrypt = errors.New("secretbox: falha ao decifrar")

type Box struct {
	aead cipher.AEAD
}

// New cria um Box a partir de uma chave de 32 bytes codificada em base64.
func New(keyB64 string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY não é base64 válido: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY deve ter 32 bytes (tem %d)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+b.aead.Overhead())
	out = append(out, version)
	out = append(out, nonce...)
	return b.aead.Seal(out, nonce, plaintext, aad), nil
}

func (b *Box) Open(data, aad []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(data) < 1+ns+b.aead.Overhead() || data[0] != version {
		return nil, ErrDecrypt
	}
	pt, err := b.aead.Open(nil, data[1:1+ns], data[1+ns:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}
