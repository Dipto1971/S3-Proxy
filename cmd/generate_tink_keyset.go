// cmd/generate_tink_keyset.go
package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
)

func GenerateTinkKeyset() error {
	handle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return fmt.Errorf("failed to generate new keyset handle: %v", err)
	}

	// Serialize the keyset to JSON.
	buf := new(bytes.Buffer)
	writer := keyset.NewJSONWriter(buf)
	err = insecurecleartextkeyset.Write(handle, writer)
	if err != nil {
		return fmt.Errorf("failed to write keyset: %v", err)
	}

	// Encode the JSON keyset to base64.
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	fmt.Println("Base64-encoded JSON keyset:")
	fmt.Println(encoded)

	return nil
}
