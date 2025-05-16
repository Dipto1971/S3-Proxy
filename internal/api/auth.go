package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// AuthenticateRequest checks if the incoming HTTP request has a valid Authorization header
// and a recognized access key stored in the Proxy's auth map.

func AuthenticateRequest(p *Proxy, r *http.Request) error {
	log.Printf("Checking Authorization header for request: %s %s", r.Method, r.RequestURI)
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing Authorization header")
	}
	log.Printf("Authorization header found: %s", authHeader)

	parts := strings.Split(authHeader, " ")
	if len(parts) < 2 || parts[0] != "AWS4-HMAC-SHA256" {
		return fmt.Errorf("invalid Authorization header format")
	}
	log.Printf("Authorization header format valid: AWS4-HMAC-SHA256")

	credentialParts := strings.Split(parts[1], "=")
	if len(credentialParts) < 2 || !strings.HasPrefix(credentialParts[0], "Credential") {
		return fmt.Errorf("invalid credential format")
	}
	credential := strings.Split(credentialParts[1], "/")
	if len(credential) < 1 {
		return fmt.Errorf("invalid credential structure")
	}
	accessKey := credential[0]
	log.Printf("Extracted access key: %s", accessKey)

	if !p.auth[accessKey] {
		return fmt.Errorf("invalid access key")
	}
	log.Printf("Access key validated successfully: %s", accessKey)

	return nil
}