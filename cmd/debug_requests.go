// cmd/debug_requests.go
package cmd

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DebugServer creates a simple HTTP server that logs all incoming requests in detail
func DebugServer() error {
	port := flag.String("port", "8081", "port to listen on for debugging")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logRequest(r)
		
		// Respond with a simple message to keep connections alive
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Debug server - request logged"))
	})

	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Debug server listening on %s", addr)
	log.Printf("Configure s3fs to point to this debug server to see what requests it sends")
	return http.ListenAndServe(addr, nil)
}

func logRequest(r *http.Request) {
	log.Printf("=== DEBUG REQUEST ===")
	log.Printf("Method: %s", r.Method)
	log.Printf("URL: %s", r.URL.String())
	log.Printf("Path: %s", r.URL.Path)
	log.Printf("RawQuery: %s", r.URL.RawQuery)
	log.Printf("Host: %s", r.Host)
	log.Printf("RemoteAddr: %s", r.RemoteAddr)
	log.Printf("RequestURI: %s", r.RequestURI)
	
	log.Printf("Headers:")
	for name, values := range r.Header {
		for _, value := range values {
			log.Printf("  %s: %s", name, value)
		}
	}
	
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			log.Printf("Body (%d bytes): %s", len(body), string(body))
		}
	}
	
	// Parse bucket and key from path like the main proxy does
	strBucket, strKey := "", ""
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 2 {
		strBucket, strKey = parts[0], parts[1]
	} else if len(parts) == 1 {
		strBucket = parts[0]
	}
	
	log.Printf("Parsed - Bucket: '%s', Key: '%s'", strBucket, strKey)
	log.Printf("Timestamp: %s", time.Now().Format(time.RFC3339))
	log.Printf("========================")
} 