// cmd/generate_tink_keyset.go
package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
)


func GenerateKeyset() error {
	if err := generateTinkKeyset(); err != nil {
		return fmt.Errorf("Tink keyset generation failed: %v", err)
	}

	if err := generateAESKey(); err != nil {
		return fmt.Errorf("AES key generation failed: %v", err)
	}

	if err := generateChaChaKey(); err != nil {
		return fmt.Errorf("ChaCha20-Poly1305 key generation failed: %v", err)
	}

	return nil
}

func generateTinkKeyset() error {
	handle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return fmt.Errorf("failed to generate new keyset handle: %v", err)
	}

	buf := new(bytes.Buffer)
	writer := keyset.NewJSONWriter(buf)
	err = insecurecleartextkeyset.Write(handle, writer)
	if err != nil {
		return fmt.Errorf("failed to write keyset: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	fmt.Println("Base64-encoded JSON keyset:")
	fmt.Println(encoded)

	return nil
}

func generateAESKey() error {
	key := make([]byte, 32) 
	if _, err := rand.Read(key); err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	fmt.Println("\n🔐 AES-256-GCM Key (Base64):")
	fmt.Println(encoded)
	return nil
}

func generateChaChaKey() error {
	key := make([]byte, 32) 
	if _, err := rand.Read(key); err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	fmt.Println("\n🔐 ChaCha20-Poly1305 Key (Base64):")
	fmt.Println(encoded)
	return nil
}
