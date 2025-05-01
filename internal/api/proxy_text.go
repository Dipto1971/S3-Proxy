package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestProxy_Put(t *testing.T) {
	// Mock S3 client (simplified)
	clients := []*s3.Client{{}}
	creds := map[string]string{"accessKey": "secretKey"}
	proxy := NewProxy(clients, creds)

	req := httptest.NewRequest(http.MethodPut, "/test-object", nil)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=accessKey/...)") // Simplified
	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}