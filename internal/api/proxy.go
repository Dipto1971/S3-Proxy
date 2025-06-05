package api

import (
	"bytes"
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
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Proxy struct {
	buckets      map[string]*s3Bucket
	auth         map[string]bool
	headerFormat string
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}

	log.Printf("Received request: %s %s", r.Method, r.RequestURI)
	log.Printf("Request headers: %v", r.Header)
	log.Printf("Request host: %s", r.Host)
	log.Printf("Request URL: %s", r.URL.String())

	if err := AuthenticateRequest(p, r); err != nil {
		log.Printf("Authentication failed: %v", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	log.Printf("Authentication successful for request: %s %s", r.Method, r.RequestURI)

	strBucket, strKey := "", ""
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 2 {
		strBucket, strKey = parts[0], parts[1]
	} else if len(parts) == 1 {
		strBucket = parts[0]
	}
	
	log.Printf("Path parsing - Original path: '%s', Trimmed path: '%s', Parts: %v", r.URL.Path, path, parts)
	log.Printf("Parsed - Bucket: '%s', Key: '%s'", strBucket, strKey)

	bucket := p.buckets[strBucket]
	if bucket != nil {
		log.Printf("Found bucket configuration for: %s", strBucket)
	} else {
		log.Printf("No bucket configuration found for: %s", strBucket)
		log.Printf("Available buckets: %v", func() []string {
			buckets := make([]string, 0, len(p.buckets))
			for k := range p.buckets {
				buckets = append(buckets, k)
			}
			return buckets
		}())
	}

	if bucket != nil && strKey != "" {
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			p.handlePut(bucket, strKey, w, r)
			return
		} else if r.Method == http.MethodGet {
			p.handleGet(bucket, strKey, w, r)
			return
		} else if r.Method == http.MethodHead {
			p.handleHead(bucket, strKey, w, r)
			return
		} else if r.Method == http.MethodDelete {
			p.handleDelete(bucket, strKey, w, r)
			return
		}
	}

	p.handleProxy(bucket, w, r)
}

func (p *Proxy) handlePut(bucket *s3Bucket, objectKey string, w http.ResponseWriter, r *http.Request) {
	log.Printf("Starting PUT operation for bucket: %s, key: %s", bucket.name, objectKey)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("PUT failed: error reading request body: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(bucket.backends))
	successCount := 0
	var mu sync.Mutex                       // Mutex for thread-safe logging
	successfulBackends := make([]string, 0) // Track successful backends

	for _, backend := range bucket.backends {
		wg.Add(1)
		go func(backend *s3Backend) {
			defer wg.Done()
			log.Printf("Encrypting data for backend: %s", backend.targetBucketName)
			encData := data
			if backend.crypto != nil {
				encData, err = backend.crypto.Encrypt(data)
				if err != nil {
					log.Printf("PUT failed: encryption error for backend %s: %v", backend.targetBucketName, err)
					errCh <- err
					return
				}
			}

			reader := bytes.NewReader(encData)
			log.Printf("Uploading to backend: %s, bucket: %s, key: %s", backend.targetBucketName, bucket.name, objectKey)
			_, err = backend.s3Client.Client.PutObject(context.Background(), &s3.PutObjectInput{
				Bucket:      &backend.targetBucketName,
				Key:         &objectKey,
				Body:        reader,
				ContentType: s(r.Header.Get("Content-Type")),
				Metadata:    getMetadataHeaders(r.Header),
			})
			if err != nil {
				log.Printf("PUT failed: upload error for backend %s: %v", backend.targetBucketName, err)
			} else {
				mu.Lock()
				successCount++
				successfulBackends = append(successfulBackends, fmt.Sprintf("%s (endpoint: %s)", backend.targetBucketName, backend.s3Client.Endpoint))
				log.Printf("PUT successful for backend bucket: %s at endpoint: %s", backend.targetBucketName, backend.s3Client.Endpoint)
				mu.Unlock()
			}
			errCh <- err
		}(backend)
	}
	wg.Wait()
	close(errCh)

	// Check if any backends succeeded
	if successCount > 0 {
		if successCount < len(bucket.backends) {
			// Some backends succeeded, some failed
			log.Printf("PUT operation partially successful: %d/%d backends succeeded", successCount, len(bucket.backends))
			log.Printf("Successfully uploaded to the following backends: %s", strings.Join(successfulBackends, ", "))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			// All backends succeeded
			log.Printf("PUT operation completed successfully for bucket: %s, key: %s", bucket.name, objectKey)
			log.Printf("Successfully uploaded to all backends: %s", strings.Join(successfulBackends, ", "))
			w.WriteHeader(http.StatusOK)
		}
		return
	}

	// All backends failed
	log.Printf("PUT operation failed: all backends failed")
	http.Error(w, "all backends failed", http.StatusBadGateway)
}

func (p *Proxy) handleGet(bucket *s3Bucket, objectKey string, w http.ResponseWriter, r *http.Request) {
	log.Printf("Starting GET operation for bucket: %s, key: %s", bucket.name, objectKey)
	if len(bucket.backends) == 0 {
		log.Printf("GET failed: no backend configured")
		http.Error(w, "no backend configured", http.StatusInternalServerError)
		return
	}

	var backendErrors []string
	var notFoundCount int
	for _, backend := range bucket.backends {
		log.Printf("Attempting to fetch from backend: %s, target bucket: %s", backend.s3Client.Endpoint, backend.targetBucketName)
		obj, err := backend.s3Client.Client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: &backend.targetBucketName,
			Key:    &objectKey,
		})

		if err != nil {
			errorMsg := fmt.Sprintf("backend %s: %v", backend.targetBucketName, err)
			log.Printf("GET attempt failed: %s", errorMsg)
			backendErrors = append(backendErrors, errorMsg)
			// Count as not found if NoSuchKey or NoSuchBucket
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "nosuchkey") || strings.Contains(errStr, "nosuchbucket") {
				notFoundCount++
			}
			continue
		}

		// If successful, process the object and return
		defer obj.Body.Close()
		log.Printf("Successfully fetched object from backend: %s", backend.targetBucketName)

		encData, err := io.ReadAll(obj.Body)
		if err != nil {
			errorMsg := fmt.Sprintf("backend %s: error reading object body: %v", backend.targetBucketName, err)
			log.Printf("GET failed: %s", errorMsg)
			backendErrors = append(backendErrors, errorMsg)
			continue
		}

		decData := encData
		if backend.crypto != nil {
			log.Printf("Decrypting data from backend: %s", backend.targetBucketName)
			decData, err = backend.crypto.Decrypt(encData)
			if err != nil {
				errorMsg := fmt.Sprintf("backend %s: decryption error: %v", backend.targetBucketName, err)
				log.Printf("GET failed: %s", errorMsg)
				backendErrors = append(backendErrors, errorMsg)
				continue
			}
		}
		log.Printf("GET operation completed successfully from backend: %s for bucket: %s, key: %s", backend.targetBucketName, bucket.name, objectKey)
		
		// Set proper headers for the response
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(decData)))
		w.WriteHeader(http.StatusOK)
		w.Write(decData)
		return // Success!
	}

	// If all backends failed
	errorSummary := fmt.Sprintf("Failed to get object from all backends. Errors: %s", strings.Join(backendErrors, "; "))
	log.Printf("GET failed: %s", errorSummary)

	// Return 404 if all backends returned not found errors
	statusCode := http.StatusBadGateway
	if notFoundCount == len(bucket.backends) {
		statusCode = http.StatusNotFound
		log.Printf("All backends returned not found errors, returning 404")
	}
	http.Error(w, errorSummary, statusCode)
}

func (p *Proxy) handleHead(bucket *s3Bucket, objectKey string, w http.ResponseWriter, r *http.Request) {
	log.Printf("Starting HEAD operation for bucket: %s, key: %s", bucket.name, objectKey)
	if len(bucket.backends) == 0 {
		log.Printf("HEAD failed: no backend configured")
		http.Error(w, "no backend configured", http.StatusInternalServerError)
		return
	}

	var backendErrors []string
	var notFoundCount int
	for _, backend := range bucket.backends {
		log.Printf("Attempting to head from backend: %s, target bucket: %s", backend.s3Client.Endpoint, backend.targetBucketName)
		obj, err := backend.s3Client.Client.HeadObject(context.Background(), &s3.HeadObjectInput{
			Bucket: &backend.targetBucketName,
			Key:    &objectKey,
		})

		if err != nil {
			errorMsg := fmt.Sprintf("backend %s: %v", backend.targetBucketName, err)
			log.Printf("HEAD attempt failed: %s", errorMsg)
			backendErrors = append(backendErrors, errorMsg)
			// Count as not found if NoSuchKey or NoSuchBucket
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "nosuchkey") || strings.Contains(errStr, "nosuchbucket") {
				notFoundCount++
			}
			continue
		}

		// If successful, set headers and return (no body for HEAD)
		log.Printf("Successfully fetched object metadata from backend: %s", backend.targetBucketName)
		
		// For HEAD requests, we need to return the decrypted content length
		// Since we can't decrypt without downloading, we'll estimate or get the original size
		contentLength := obj.ContentLength
		if contentLength != nil {
			// If the object is encrypted, we need to get actual decrypted size
			// For now, we'll use a simple approach: get the object to determine real size
			getObj, getErr := backend.s3Client.Client.GetObject(context.Background(), &s3.GetObjectInput{
				Bucket: &backend.targetBucketName,
				Key:    &objectKey,
			})
			if getErr == nil {
				encData, readErr := io.ReadAll(getObj.Body)
				getObj.Body.Close()
				if readErr == nil {
					decData := encData
					if backend.crypto != nil {
						decData, err = backend.crypto.Decrypt(encData)
						if err == nil {
							contentLength = aws.Int64(int64(len(decData)))
						}
					}
				}
			}
		}

		// Set response headers
		if obj.ContentType != nil {
			w.Header().Set("Content-Type", *obj.ContentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		
		if contentLength != nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", *contentLength))
		}
		
		if obj.LastModified != nil {
			w.Header().Set("Last-Modified", obj.LastModified.Format(http.TimeFormat))
		}
		
		if obj.ETag != nil {
			w.Header().Set("ETag", *obj.ETag)
		}

		log.Printf("HEAD operation completed successfully from backend: %s for bucket: %s, key: %s", backend.targetBucketName, bucket.name, objectKey)
		w.WriteHeader(http.StatusOK)
		return // Success!
	}

	// If all backends failed
	errorSummary := fmt.Sprintf("Failed to head object from all backends. Errors: %s", strings.Join(backendErrors, "; "))
	log.Printf("HEAD failed: %s", errorSummary)

	// Return 404 if all backends returned not found errors
	statusCode := http.StatusBadGateway
	if notFoundCount == len(bucket.backends) {
		statusCode = http.StatusNotFound
		log.Printf("All backends returned not found errors, returning 404")
	}
	http.Error(w, errorSummary, statusCode)
}

func (p *Proxy) handleDelete(bucket *s3Bucket, objectKey string, w http.ResponseWriter, r *http.Request) {
	log.Printf("Starting DELETE operation for bucket: %s, key: %s", bucket.name, objectKey)
	var wg sync.WaitGroup
	errCh := make(chan error, len(bucket.backends))
	for _, backend := range bucket.backends {
		wg.Add(1)
		go func(backend *s3Backend) {
			defer wg.Done()
			log.Printf("Deleting from backend: %s", backend.targetBucketName)
			_, err := backend.s3Client.Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: &backend.targetBucketName,
				Key:    &objectKey,
			})
			if err != nil {
				log.Printf("DELETE failed for backend %s: %v", backend.targetBucketName, err)
			} else {
				log.Printf("DELETE successful for backend: %s", backend.targetBucketName)
			}
			errCh <- err
		}(backend)
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil && strings.Contains(e.Error(), "NoSuchKey") {
			log.Printf("DELETE operation failed due to replication error: %v", e)
			http.Error(w, e.Error(), http.StatusBadGateway)
			return
		}
	}
	log.Printf("DELETE operation completed successfully for bucket: %s, key: %s", bucket.name, objectKey)
	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) handleProxy(bucket *s3Bucket, w http.ResponseWriter, r *http.Request) {
	log.Printf("Starting PROXY operation for request: %s %s", r.Method, r.RequestURI)
	if bucket == nil {
		for _, b := range p.buckets {
			bucket = b
			break
		}
	}

	if len(bucket.backends) == 0 {
		log.Printf("PROXY failed: no backend configured")
		http.Error(w, "no backend configured", http.StatusInternalServerError)
		return
	}

	backend := bucket.backends[0]
	log.Printf("Proxying to backend: %s", backend.targetBucketName)
	newReq := p.repackage(r, backend)
	newReq.URL.Path = strings.ReplaceAll(newReq.URL.Path, bucket.name, backend.targetBucketName)

	creds, err := backend.s3Client.Config.Credentials.Retrieve(context.TODO())
	if err != nil {
		log.Printf("PROXY failed: error retrieving credentials: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	signer := v4.NewSigner()
	err = signer.SignHTTP(context.Background(), creds, newReq, newReq.Header.Get("X-Amz-Content-Sha256"), "s3", backend.s3Client.Config.Region, time.Now())
	if err != nil {
		log.Printf("PROXY failed: error signing request: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		log.Printf("PROXY failed: error executing request: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("PROXY response received: status %d", resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	if err != nil {
		log.Printf("PROXY failed: error writing response: %v", err)
		return
	}
	log.Printf("PROXY operation completed successfully")
}

func (p *Proxy) repackage(r *http.Request, backend *s3Backend) *http.Request {
	log.Printf("Repackaging request for backend: %s", backend.targetBucketName)
	req := r.Clone(r.Context())
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

	log.Printf("Request repackaged successfully")
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
