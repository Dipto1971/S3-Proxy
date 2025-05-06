// internal/api/proxy.go
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

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Proxy struct {
	buckets map[string]*s3Bucket
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}

	fmt.Printf("<%s> %s %s\n", time.Now().Format(time.RFC3339), r.Method, r.RequestURI)

	strBucket, strKey := "", ""

	// Expected path: /{bucket}/{objectKey}
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 2 {
		strBucket, strKey = parts[0], parts[1]
	} else if len(parts) == 1 {
		strBucket = parts[0]
	}

	bucket := p.buckets[strBucket]

	if bucket != nil && strKey != "" {
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			p.handlePut(bucket, strKey, w, r)
			return
		} else if r.Method == http.MethodGet {
			p.handleGet(bucket, strKey, w)
			return
		} else if r.Method == http.MethodDelete {
			p.handleDelete(bucket, strKey, w)
			return
		}
	}

	p.handleProxy(bucket, w, r)
}

func (p *Proxy) handlePut(bucket *s3Bucket, objectKey string, w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(bucket.backends))
	for _, backend := range bucket.backends {
		wg.Add(1)
		go func(backend *s3Backend) {
			defer wg.Done()

			encData := data
			if backend.crypto != nil {
				encData, err = backend.crypto.Encrypt(data)
				if err != nil {
					errCh <- err
					return
				}
			}

			reader := bytes.NewReader(encData)
			_, err = backend.s3Client.Client.PutObject(context.Background(), &s3.PutObjectInput{
				Bucket:      &backend.targetBucketName,
				Key:         &objectKey,
				Body:        reader,
				ContentType: s(r.Header.Get("Content-Type")),
				Metadata:    getMetadataHeaders(r.Header),

				// TODO: more headers
			})
			errCh <- err
		}(backend)
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil {
			log.Printf("replication error: %v", e)
			http.Error(w, e.Error(), http.StatusBadGateway)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (p *Proxy) handleGet(bucket *s3Bucket, objectKey string, w http.ResponseWriter) {
	if len(bucket.backends) == 0 {
		http.Error(w, "no backend configured", http.StatusInternalServerError)
		return
	}

	backend := bucket.backends[0]

	obj, err := backend.s3Client.Client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &backend.targetBucketName,
		Key:    &objectKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer obj.Body.Close()

	encData, err := io.ReadAll(obj.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	decData := encData
	if backend.crypto != nil {
		decData, err = backend.crypto.Decrypt(encData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Write(decData)
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
