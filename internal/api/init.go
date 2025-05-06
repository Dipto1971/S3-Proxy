// internal/api/init.go
package api

import (
	"fmt"
	"s3-proxy/internal/client"
	"s3-proxy/internal/config"
	"s3-proxy/internal/crypto"
	"strings"

	"github.com/google/tink/go/keyset"
)

type s3Bucket struct {
	name     string
	backends []*s3Backend
}

type s3Backend struct {
	targetBucketName string
	s3Client         *client.S3
	crypto           crypto.Crypt
}

func New(cfg *config.Config) (*Proxy, error) {
	cryptos := make(map[string]crypto.Crypt)
	for _, cfgCrypto := range cfg.Crypto {
		layers := make([]crypto.Crypt, 0, len(cfgCrypto.Layers))
		for _, cfgLayer := range cfgCrypto.Layers {
			if cfgLayer.Algorithm == "tink" {
				reader := keyset.NewJSONReader(strings.NewReader(cfgLayer.Keyset.Get()))
				kh, err := keyset.Read(reader, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to parse Tink keyset: %w", err)
				}
				layer, err := crypto.NewTinkCrypt(kh)
				if err != nil {
					return nil, err
				}
				layers = append(layers, layer)
			} else {
				return nil, fmt.Errorf("unsupported crypto algorithm: %s", cfgLayer.Algorithm)
			}
		}

		cryptos[cfgCrypto.ID] = crypto.NewMultiLayerCrypt(layers...)
	}

	s3Clients := make(map[string]*client.S3)
	for _, cfgClient := range cfg.S3Clients {
		client, err := client.NewS3(cfgClient.Endpoint, cfgClient.Region, cfgClient.AccessKey.Get(), cfgClient.SecretKey.Get())
		if err != nil {
			return nil, err
		}
		s3Clients[cfgClient.ID] = client
	}

	buckets := make(map[string]*s3Bucket)
	for _, cfgBucket := range cfg.S3Buckets {
		bucket := &s3Bucket{
			name:     cfgBucket.BucketName,
			backends: make([]*s3Backend, 0, len(cfgBucket.Backends)),
		}

		for _, cfgBucketBackend := range cfgBucket.Backends {
			s3Client, ok := s3Clients[cfgBucketBackend.S3ClientID]
			if !ok {
				return nil, fmt.Errorf("unknown S3 client ID: %s", cfgBucketBackend.S3ClientID)
			}

			var crypto crypto.Crypt = nil
			if cfgBucketBackend.CryptoID != "" {
				crypto, ok = cryptos[cfgBucketBackend.CryptoID]
				if !ok {
					return nil, fmt.Errorf("unknown crypto ID: %s", cfgBucketBackend.CryptoID)
				}
			}
			bucket.backends = append(bucket.backends, &s3Backend{
				targetBucketName: cfgBucketBackend.S3BucketName,
				s3Client:         s3Client,
				crypto:           crypto,
			})
		}

		buckets[cfgBucket.BucketName] = bucket
	}

	return &Proxy{
		buckets: buckets,
	}, nil
}
