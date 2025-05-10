// internal/crypto/chacha.go
package crypto

import (
	"golang.org/x/crypto/chacha20poly1305"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type ChaChaCrypt struct {
	aead cipher.AEAD
}

func NewChaChaCrypt(base64Key string) (*ChaChaCrypt, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	return &ChaChaCrypt{aead: aead}, nil
}

func (c *ChaChaCrypt) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *ChaChaCrypt) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return c.aead.Open(nil, nonce, ciphertext, nil)
}
