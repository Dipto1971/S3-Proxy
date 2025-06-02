// internal/api/auth.go
package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

func AuthenticateRequest(p *Proxy, r *http.Request) error {
	log.Printf("Checking Authorization header for request: %s %s", r.Method, r.RequestURI)
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing Authorization header")
	}
	// log.Printf("Authorization header found: %s", authHeader)

	if p.headerFormat == "" {
		return fmt.Errorf("no authorization header format configured")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) < 2 || parts[0] != p.headerFormat {
		return fmt.Errorf("invalid Authorization header format, expected %s", p.headerFormat)
	}
	// log.Printf("Authorization header format valid: %s", p.headerFormat)

	credentialParts := strings.Split(parts[1], "=")
	if len(credentialParts) < 2 || !strings.HasPrefix(credentialParts[0], "Credential") {
		return fmt.Errorf("invalid credential format")
	}
	credential := strings.Split(credentialParts[1], "/")
	if len(credential) < 1 {
		return fmt.Errorf("invalid credential structure")
	}
	accessKey := credential[0]
	// log.Printf("Extracted access key: %s", accessKey)

	if !p.auth[accessKey] {
		return fmt.Errorf("invalid access key")
	}
	// log.Printf("Access key validated successfully: %s", accessKey)

	return nil
}