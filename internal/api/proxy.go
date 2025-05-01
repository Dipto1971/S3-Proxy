// internal/api/proxy.go
package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Proxy struct {
	buckets    map[string]*s3Bucket // Added to store bucket configurations
	s3Clients  []*s3.Client
	credentials map[string]string // accessKey -> secretKey
	mu         sync.Mutex
}

// parseAuthorization parses the AWS SigV4 Authorization header to extract the access key and secret key.
func parseAuthorization(authHeader string) (string, string, error) {
	// Example: "AWS4-HMAC-SHA256 Credential=ACCESS_KEY/DATE/REGION/SERVICE/aws4_request, SignedHeaders=..., Signature=..."
	parts := strings.Split(authHeader, " ")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "AWS4-HMAC-SHA256") {
		return "", "", fmt.Errorf("invalid Authorization header format")
	}

	// Extract the Credential part
	var accessKey, signature string
	for _, part := range strings.Split(parts[1], ",") {
		if strings.HasPrefix(part, "Credential=") {
			credParts := strings.Split(strings.TrimPrefix(part, "Credential="), "/")
			if len(credParts) < 1 {
				return "", "", fmt.Errorf("invalid Credential format")
			}
			accessKey = credParts[0]
		} else if strings.HasPrefix(part, "Signature=") {
			signature = strings.TrimPrefix(part, "Signature=")
		}
	}

	if accessKey == "" {
		return "", "", fmt.Errorf("Credential not found in Authorization header")
	}
	if signature == "" {
		return "", "", fmt.Errorf("Signature not found in Authorization header")
	}

	return accessKey, signature, nil
}

type s3ClientWrapper struct { // Wrapper for s3.Client
	Client *s3.Client
	Config aws.Config // Includes Credentials and Region
}

type Crypto interface { // Placeholder for encryption/decryption
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

func NewProxy(clients []*s3.Client, creds map[string]string) *Proxy {
	return &Proxy{
		s3Clients:   clients,
		credentials: creds,
		buckets:     make(map[string]*s3Bucket),
	}
}


func (p *Proxy) verifySignature(r *http.Request) (string, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    authHeader := r.Header.Get("Authorization")
    if authHeader == "" {
        return "", fmt.Errorf("missing Authorization header")
    }

    // Extract only the access key; we don't need the raw signature here.
    accessKey, _, err := parseAuthorization(authHeader)
    if err != nil {
        return "", fmt.Errorf("invalid Authorization header: %w", err)
    }

    // Look up the user’s secret on file
    secretKey, ok := p.credentials[accessKey]
    if !ok {
        return "", fmt.Errorf("unknown access key")
    }

    // Clone the request for signing
    req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
    if err != nil {
        return "", fmt.Errorf("failed to clone request: %w", err)
    }
    req.Header = r.Header.Clone()
    req.RequestURI = "" // required for SignHTTP

    // Build a static credentials provider and retrieve a Credentials value
    provider := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
    awsCreds, err := provider.Retrieve(context.Background())
    if err != nil {
        return "", fmt.Errorf("could not retrieve static credentials: %w", err)
    }

    // Have the v4 signer re-sign the request
    signer := v4.NewSigner()
    if err := signer.SignHTTP(
        context.Background(),
        awsCreds,
        req,
        "",
        "s3",
        req.URL.Hostname(),
        time.Now(),
    ); err != nil {
        return "", fmt.Errorf("internal signing failed: %w", err)
    }

    // Compare their original header to the one we just generated
    if got := req.Header.Get("Authorization"); got != authHeader {
        return "", fmt.Errorf("signature mismatch")
    }

    return accessKey, nil
}





func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify SigV4 signature
	accessKey, err := p.verifySignature(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Proceed with request handling (e.g., PUT, GET, DELETE)
	switch r.Method {
	case http.MethodPut:
		p.handlePut(w, r, accessKey)
	case http.MethodGet:
		p.handleGet(w, r, accessKey)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Proxy) handlePut(w http.ResponseWriter, r *http.Request, accessKey string) {
	pr, pw := io.Pipe()
	defer pr.Close()

	// Stream request body to S3 backends
	var wg sync.WaitGroup
	errChan := make(chan error, len(p.s3Clients))

	for _, client := range p.s3Clients {
		wg.Add(1)
		go func(cl *s3.Client) {
			defer wg.Done()
			_, err := cl.PutObject(r.Context(), &s3.PutObjectInput{
				Bucket: aws.String("my-bucket"), // Replace with dynamic bucket
				Key:    aws.String(r.URL.Path),
				Body:   pr,
			})
			if err != nil {
				errChan <- err
			}
		}(client)
	}

	// Copy request body to pipe
	go func() {
		defer pw.Close()
		_, err := io.Copy(pw, r.Body)
		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			http.Error(w, "Upload failed", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) handleGet(w http.ResponseWriter, r *http.Request, accessKey string) {
    // 1) Determine which bucket we’re targeting. You might map accessKey -> bucket,
    //    or parse r.URL.Path to extract the bucket name.
    //    Here’s an example that parses “/{bucket}/{key…}” from the URL:

    parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
    if len(parts) < 2 {
        http.Error(w, "invalid path", http.StatusBadRequest)
        return
    }
    bucketName, objectKey := parts[0], parts[1]

    // 2) Look up your bucket config
    bucketCfg, ok := p.buckets[bucketName]
    if !ok {
        http.Error(w, "bucket not found", http.StatusNotFound)
        return
    }
    if len(bucketCfg.backends) == 0 {
        http.Error(w, "no backends configured", http.StatusInternalServerError)
        return
    }

    // 3) Fetch from the first backend and stream it out
    backend := bucketCfg.backends[0]
    obj, err := backend.s3Client.Client.GetObject(context.Background(), &s3.GetObjectInput{
        Bucket: &backend.targetBucketName,
        Key:    &objectKey,
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadGateway)
        return
    }
    defer obj.Body.Close()

    // 4) Read and optionally decrypt
    data, err := io.ReadAll(obj.Body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    if backend.crypto != nil {
        data, err = backend.crypto.Decrypt(data)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
    }

    // 5) Write out
    w.WriteHeader(http.StatusOK)
    if _, err := w.Write(data); err != nil {
        log.Printf("error writing response: %v", err)
    }
}


func (p *Proxy) handleDelete(bucket *s3Bucket, objectKey string, w http.ResponseWriter) {
	var wg sync.WaitGroup
	errCh := make(chan error, len(bucket.backends))
	for _, backend := range bucket.backends {
		wg.Add(1)
		go func(backend *s3Backend) {
			defer wg.Done()
			_, err := backend.s3Client.Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: &backend.targetBucketName,
				Key:    &objectKey,
			})
			errCh <- err
		}(backend)
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil && strings.Contains(e.Error(), "NoSuchKey") {
			log.Printf("replication error: %v", e)
			http.Error(w, e.Error(), http.StatusBadGateway)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) handleProxy(bucket *s3Bucket, w http.ResponseWriter, r *http.Request) {
	if bucket == nil {
		// TODO: handle ?x-id=ListBuckets

		for _, b := range p.buckets {
			bucket = b
			break
		}
	}

	if len(bucket.backends) == 0 {
		http.Error(w, "no backend configured", http.StatusInternalServerError)
		return
	}

	backend := bucket.backends[0]
	newReq := p.repackage(r, backend)
	newReq.URL.Path = strings.ReplaceAll(newReq.URL.Path, bucket.name, backend.targetBucketName)

	creds, err := backend.s3Client.Config.Credentials.Retrieve(context.TODO())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	signer := v4.NewSigner()

	err = signer.SignHTTP(context.Background(), creds, newReq, newReq.Header.Get("X-Amz-Content-Sha256"), "s3", backend.s3Client.Config.Region, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	t, _ := io.ReadAll(resp.Body)
	fmt.Println(string(t))

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		fmt.Println("Error copying response body:", err)
		return
	}
}

func (p *Proxy) repackage(r *http.Request, backend *s3Backend) *http.Request {
	req := r.Clone(r.Context())

	// HTTP clients are not supposed to set this field, however when we receive a request it is set.
	// So, we unset it.
	req.RequestURI = ""

	if strings.HasPrefix(backend.s3Client.Endpoint, "https://") {
		req.URL.Scheme = "https"
	} else {
		req.URL.Scheme = "http"
	}

	host := strings.TrimPrefix(backend.s3Client.Endpoint, req.URL.Scheme+"://")
	req.Host = host
	req.URL.Host = host

	headersToRemove := []string{
		"X-Real-Ip",
		"X-Forwarded-Scheme",
		"X-Forwarded-Proto",
		"X-Scheme",
		"X-Forwarded-Host",
		"X-Forwarded-Port",
		"X-Forwarded-For",
	}

	for _, header := range headersToRemove {
		req.Header.Del(header)
	}

	return req
}

func s(d string) *string {
	if d == "" {
		return nil
	}
	return &d
}

func getMetadataHeaders(header http.Header) map[string]string {
	result := map[string]string{}

	for key := range header {
		key = strings.ToLower(key)

		if strings.HasPrefix(key, "x-amz-meta-") {
			name := strings.TrimPrefix(key, "x-amz-meta-")
			result[name] = strings.Join(header.Values(key), ",")
		}
	}

	return result
}
