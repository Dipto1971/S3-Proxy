package e2e

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"s3-proxy/internal/api"
	"s3-proxy/internal/config"

	"github.com/joho/godotenv"
)

// Helper function to load configuration and set up the proxy handler for tests
// In a real-world scenario, you might have different config files for different test setups.
func setupProxy(t *testing.T) http.Handler {
	// Attempt to load .env file from the project root.
	// Adjust path if your test execution directory is different.
	// This is to ensure environment variables used in main.yaml are available.
	envPath := filepath.Join("..", "..", ".env")
	if _, err := os.Stat(envPath); err == nil {
		if err := godotenv.Load(envPath); err != nil {
			t.Logf("Warning: could not load .env file from %s: %v", envPath, err)
		}
	} else {
		t.Logf("Warning: .env file not found at %s, relying on pre-set environment variables", envPath)
	}

	cfgPath := filepath.Join("..", "..", "configs", "main.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Failed to load configuration from %s: %v", cfgPath, err)
	}

	proxy, err := api.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}
	return proxy
}

func TestHealthCheck(t *testing.T) {
	proxyHandler := setupProxy(t)
	server := httptest.NewServer(proxyHandler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("Health check request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK (200), got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read health check response body: %v", err)
	}
	if string(bodyBytes) != "ok" {
		t.Errorf("Expected body 'ok', got '%s'", string(bodyBytes))
	}
}

// TestPutObject_ValidAuth assumes your configs/main.yaml defines:
// - A proxy bucket named "test-bucket".
// - Authentication that accepts "VALID_ACCESS_KEY" (e.g., from ALLOWED_ACCESS_KEY1 env var).
// - AUTH_HEADER_FORMAT is set (e.g., "AWS4-HMAC-SHA256").
// - "test-bucket" in main.yaml should have at least one backend configured.
// To truly test replication, you'd inspect the backend S3 service(s) after this test.
func TestPutObject_ValidAuth(t *testing.T) {
	proxyHandler := setupProxy(t)
	server := httptest.NewServer(proxyHandler)
	defer server.Close()

	authHeaderValue := os.Getenv("AUTH_HEADER_FORMAT") + " Credential=" + os.Getenv("ALLOWED_ACCESS_KEY1") + "/20250516/us-east-1/s3/aws4_request"

	reqBody := "hello world from test"
	req, err := http.NewRequest("PUT", server.URL+"/test-bucket/e2e-test-object.txt", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to create PUT request: %v", err)
	}
	req.Header.Set("Authorization", authHeaderValue)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	defer resp.Body.Close()

	// Consider it a success if at least one backend succeeded
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status OK (200) or Partial Content (206) for PUT, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}
	t.Logf("PUT to /test-bucket/e2e-test-object.txt returned status %d", resp.StatusCode)
}

func TestPutObject_InvalidAuth(t *testing.T) {
	proxyHandler := setupProxy(t)
	server := httptest.NewServer(proxyHandler)
	defer server.Close()

	authHeaderValue := os.Getenv("AUTH_HEADER_FORMAT") + " Credential=INVALIDACCESSKEY/20250516/us-east-1/s3/aws4_request"

	reqBody := "test data for invalid auth"
	req, err := http.NewRequest("PUT", server.URL+"/test-bucket/e2e-invalid-auth.txt", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("Failed to create PUT request: %v", err)
	}
	req.Header.Set("Authorization", authHeaderValue)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status Unauthorized (401) for PUT with invalid key, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}
}

// TestGetObject_ValidAuth_FirstBackend assumes an object "e2e-get-test.txt" was previously PUT
// or is known to exist on the first configured backend for "test-bucket".
func TestGetObject_ValidAuth_FirstBackend(t *testing.T) {
	proxyHandler := setupProxy(t)
	server := httptest.NewServer(proxyHandler)
	defer server.Close()

	// Step 1: PUT an object to ensure it exists
	authHeaderValue := os.Getenv("AUTH_HEADER_FORMAT") + " Credential=" + os.Getenv("ALLOWED_ACCESS_KEY1") + "/20250516/us-east-1/s3/aws4_request"
	objectKey := "e2e-get-test.txt"
	putUrl := server.URL + "/test-bucket/" + objectKey
	expectedData := "data for GET test"

	putReq, err := http.NewRequest("PUT", putUrl, strings.NewReader(expectedData))
	if err != nil {
		t.Fatalf("Failed to create PUT request for GET test setup: %v", err)
	}
	putReq.Header.Set("Authorization", authHeaderValue)
	putReq.Header.Set("Content-Type", "text/plain")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("PUT request for GET test setup failed: %v", err)
	}
	defer putResp.Body.Close()

	// Continue if at least one backend succeeded
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusPartialContent {
		bodyBytes, _ := io.ReadAll(putResp.Body)
		t.Fatalf("All PUT operations failed with status %d. Body: %s", putResp.StatusCode, string(bodyBytes))
	}

	// Step 2: GET the object
	getReq, err := http.NewRequest("GET", putUrl, nil)
	if err != nil {
		t.Fatalf("Failed to create GET request: %v", err)
	}
	getReq.Header.Set("Authorization", authHeaderValue)

	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer getResp.Body.Close()

	// Consider it a success if we get the object from any backend
	if getResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(getResp.Body)
		t.Errorf("Expected status OK (200) for GET, got %d. Body: %s", getResp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("Failed to read GET response body: %v", err)
	}
	if string(bodyBytes) != expectedData {
		t.Errorf("Expected GET body '%s', got '%s'", expectedData, string(bodyBytes))
	}
}

// TestGetObject_Failover requires a more complex setup:
// 1. Configure "test-bucket" with at least two backends in main.yaml.
// 2. Ensure an object exists *only* on the second backend.
// 3. Have a way to make the first backend S3 service *fail* for the GetObject call.
// This typically requires a mock S3 client that can be instructed to fail.
// For now, this is a placeholder for that concept.
func TestGetObject_Failover(t *testing.T) {
	t.Skip("Skipping GET failover test: requires mock S3 backend or specific setup for backend failure.")
	// proxyHandler := setupProxy(t)
	// server := httptest.NewServer(proxyHandler)
	// defer server.Close()
	//
	// Setup:
	// - Modify configs/main.yaml to have test-bucket with backends:
	//   1. s3_bucket_name: "primary-failover" (will be made to fail)
	//   2. s3_bucket_name: "secondary-failover" (will have the object)
	// - Ensure "failover-object.txt" exists ONLY in "secondary-failover" on your S3 service.
	// - Ensure your S3 client mock (if you had one) can simulate GetObject failure for "primary-failover".
	//
	// authHeaderValue := os.Getenv("AUTH_HEADER_FORMAT") + " Credential=" + os.Getenv("ALLOWED_ACCESS_KEY1") + "/20250516/us-east-1/s3/aws4_request"
	// req, err := http.NewRequest("GET", server.URL+"/test-bucket/failover-object.txt", nil)
	// if err != nil {
	// 	t.Fatalf("Failed to create GET request: %v", err)
	// }
	// req.Header.Set("Authorization", authHeaderValue)
	//
	// resp, err := http.DefaultClient.Do(req)
	// if err != nil {
	// 	t.Fatalf("GET request for failover failed: %v", err)
	// }
	// defer resp.Body.Close()
	//
	// if resp.StatusCode != http.StatusOK {
	// 	bodyBytes, _ := io.ReadAll(resp.Body)
	// 	t.Errorf("Expected status OK (200) for GET failover, got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	// }
	// // Verify body content matches what's in "secondary-failover"
}

// TestDeleteObject_ValidAuth assumes "test-bucket" is configured for replication.
// It will PUT an object, then DELETE it.
// To truly test replication, you'd check that the object is gone from all backends.
func TestDeleteObject_ValidAuth(t *testing.T) {
	proxyHandler := setupProxy(t)
	server := httptest.NewServer(proxyHandler)
	defer server.Close()

	authHeaderValue := os.Getenv("AUTH_HEADER_FORMAT") + " Credential=" + os.Getenv("ALLOWED_ACCESS_KEY1") + "/20250516/us-east-1/s3/aws4_request"
	objectKey := "e2e-delete-test.txt"
	targetUrl := server.URL + "/test-bucket/" + objectKey

	// Step 1: PUT an object to ensure it exists
	putReq, err := http.NewRequest("PUT", targetUrl, strings.NewReader("data to be deleted"))
	if err != nil {
		t.Fatalf("Failed to create PUT request for DELETE test setup: %v", err)
	}
	putReq.Header.Set("Authorization", authHeaderValue)
	putReq.Header.Set("Content-Type", "text/plain")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("PUT request for DELETE test setup failed: %v", err)
	}
	defer putResp.Body.Close()

	// Continue if at least one backend succeeded
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusPartialContent {
		bodyBytes, _ := io.ReadAll(putResp.Body)
		t.Fatalf("All PUT operations failed with status %d. Body: %s", putResp.StatusCode, string(bodyBytes))
	}

	// Step 2: DELETE the object
	delReq, err := http.NewRequest("DELETE", targetUrl, nil)
	if err != nil {
		t.Fatalf("Failed to create DELETE request: %v", err)
	}
	delReq.Header.Set("Authorization", authHeaderValue)

	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE request failed: %v", err)
	}
	defer delResp.Body.Close()

	// Consider it a success if we can delete from any backend
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusPartialContent {
		bodyBytes, _ := io.ReadAll(delResp.Body)
		t.Errorf("Expected status OK (200) or Partial Content (206) for DELETE, got %d. Body: %s", delResp.StatusCode, string(bodyBytes))
	}

	// Optional: Try to GET the deleted object to confirm it's gone from at least one backend
	getReq, _ := http.NewRequest("GET", targetUrl, nil)
	getReq.Header.Set("Authorization", authHeaderValue)
	getResp, _ := http.DefaultClient.Do(getReq)
	if getResp != nil {
		defer getResp.Body.Close()
		if getResp.StatusCode == http.StatusOK {
			t.Logf("GET after DELETE returned 200 OK, object might still exist in some backends.")
		} else {
			t.Logf("GET after DELETE returned status %d (object not found in checked backends).", getResp.StatusCode)
		}
	}
}

// TestGetAndDeleteObject_ValidAuth tests both GET and DELETE operations in sequence
func TestGetAndDeleteObject_ValidAuth(t *testing.T) {
	// Create test file
	testData := []byte("test data for get and delete")
	testFile := "e2e-get-delete-test.txt"

	// Create PUT request
	req, err := http.NewRequest("PUT", fmt.Sprintf("http://localhost:8080/test-bucket/%s", testFile), bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create PUT request: %v", err)
	}

	// Add authentication headers
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=test-signature")
	req.Header.Set("X-Amz-Date", "20250101T000000Z")

	// Send PUT request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check PUT response - accept both 200 and 206
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT failed with status %d. Body: %s", resp.StatusCode, string(body))
	}

	// Create GET request
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/test-bucket/%s", testFile), nil)
	if err != nil {
		t.Fatalf("Failed to create GET request: %v", err)
	}

	// Add authentication headers
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=test-signature")
	req.Header.Set("X-Amz-Date", "20250101T000000Z")

	// Send GET request
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check GET response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET failed with status %d. Body: %s", resp.StatusCode, string(body))
	}

	// Verify content
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read GET response body: %v", err)
	}
	if !bytes.Equal(body, testData) {
		t.Fatalf("GET response body does not match test data. Got: %s, Want: %s", string(body), string(testData))
	}

	// Create DELETE request
	req, err = http.NewRequest("DELETE", fmt.Sprintf("http://localhost:8080/test-bucket/%s", testFile), nil)
	if err != nil {
		t.Fatalf("Failed to create DELETE request: %v", err)
	}

	// Add authentication headers
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=test-signature")
	req.Header.Set("X-Amz-Date", "20250101T000000Z")

	// Send DELETE request
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check DELETE response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE failed with status %d. Body: %s", resp.StatusCode, string(body))
	}

	// Verify deletion with GET
	req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:8080/test-bucket/%s", testFile), nil)
	if err != nil {
		t.Fatalf("Failed to create verification GET request: %v", err)
	}

	// Add authentication headers
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEACCESSKEY1/20250101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=test-signature")
	req.Header.Set("X-Amz-Date", "20250101T000000Z")

	// Send verification GET request
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Verification GET request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check verification GET response - should be 404
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Verification GET after DELETE returned status %d (expected 404). Body: %s", resp.StatusCode, string(body))
	}
}
