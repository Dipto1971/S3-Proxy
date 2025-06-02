// cmd/generate_tink_keyset.go
package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
)

type Keysets struct {
	TinkKeyset string
	AESKey     string
	ChaChaKey  string
}

func GenerateKeyset() error {
	keysets, err := generateAllKeysets()
	if err != nil {
		return err
	}

	// // Print the keysets
	// fmt.Println("Base64-encoded JSON keyset:")
	// fmt.Println(keysets.TinkKeyset)
	// fmt.Println("\n🔐 AES-256-GCM Key (Base64):")
	// fmt.Println(keysets.AESKey)
	// fmt.Println("\n🔐 ChaCha20-Poly1305 Key (Base64):")
	// fmt.Println(keysets.ChaChaKey)

	// Update .env file
	if err := updateEnvFile(keysets); err != nil {
		return fmt.Errorf("failed to update .env file: %v", err)
	}

	return nil
}

func generateAllKeysets() (*Keysets, error) {
	tinkKeyset, err := generateTinkKeyset()
	if err != nil {
		return nil, fmt.Errorf("Tink keyset generation failed: %v", err)
	}

	aesKey, err := generateAESKey()
	if err != nil {
		return nil, fmt.Errorf("AES key generation failed: %v", err)
	}

	chaChaKey, err := generateChaChaKey()
	if err != nil {
		return nil, fmt.Errorf("ChaCha20-Poly1305 key generation failed: %v", err)
	}

	return &Keysets{
		TinkKeyset: tinkKeyset,
		AESKey:     aesKey,
		ChaChaKey:  chaChaKey,
	}, nil
}

func generateTinkKeyset() (string, error) {
	handle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return "", fmt.Errorf("failed to generate new keyset handle: %v", err)
	}

	buf := new(bytes.Buffer)
	writer := keyset.NewJSONWriter(buf)
	err = insecurecleartextkeyset.Write(handle, writer)
	if err != nil {
		return "", fmt.Errorf("failed to write keyset: %v", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func generateAESKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func generateChaChaKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func updateEnvFile(keysets *Keysets) error {
	envPath := ".env"
	var existingContent string

	// Read existing .env file if it exists
	if _, err := os.Stat(envPath); err == nil {
		content, err := os.ReadFile(envPath)
		if err != nil {
			return fmt.Errorf("failed to read existing .env file: %v", err)
		}
		existingContent = string(content)
	}

	// Split content into lines
	lines := strings.Split(existingContent, "\n")
	
	// Create new content with updated keyset lines
	var newContent strings.Builder
	
	// Update first three lines with new keysets
	newContent.WriteString(fmt.Sprintf("TINK_KEYSET=\"%s\"\n", keysets.TinkKeyset))
	newContent.WriteString(fmt.Sprintf("AES_KEY=\"%s\"\n", keysets.AESKey))
	newContent.WriteString(fmt.Sprintf("CHACHA_KEY=\"%s\"\n", keysets.ChaChaKey))
	
	// Add the rest of the content if it exists
	if len(lines) > 3 {
		newContent.WriteString(strings.Join(lines[3:], "\n"))
		if !strings.HasSuffix(existingContent, "\n") {
			newContent.WriteString("\n")
		}
	}

	// Write the content to the .env file
	if err := os.WriteFile(envPath, []byte(newContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write .env file: %v", err)
	}

	fmt.Println("\n✅ Successfully updated keysets in .env file")
	return nil
}
