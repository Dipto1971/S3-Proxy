// internal/crypto/tink.go
package crypto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/hybrid"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"
)

type TinkCrypt struct {
	encryptor tink.HybridEncrypt
	decryptor tink.HybridDecrypt
}

func NewTinkCrypt(keysetStr *keyset.Handle) (*TinkCrypt, error) {
	keysetJSON, err := base64.StdEncoding.DecodeString(keysetStr.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode keyset: %v", err)
	}

	reader := keyset.NewJSONReader(bytes.NewReader(keysetJSON))

	handle, err := insecurecleartextkeyset.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read keyset: %v", err)
	}

	if a, err := aead.New(handle); err == nil {
		return &TinkCrypt{
			encryptor: a,
			decryptor: a,
		}, nil
	}

	enc, errEnc := hybrid.NewHybridEncrypt(handle)
	dec, errDec := hybrid.NewHybridDecrypt(handle)
	if errEnc == nil && errDec == nil {
		return &TinkCrypt{
			encryptor: enc,
			decryptor: dec,
		}, nil
	}

	return nil, errors.New("unsupported keyset type")
}

func (t *TinkCrypt) Encrypt(plaintext []byte) ([]byte, error) {
	return t.encryptor.Encrypt(plaintext, nil)
}

func (t *TinkCrypt) Decrypt(ciphertext []byte) ([]byte, error) {
	return t.decryptor.Decrypt(ciphertext, nil)
}