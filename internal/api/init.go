package api

import (
	"fmt"
	"log"
	"s3-proxy/internal/client"
	"s3-proxy/internal/config"
	"s3-proxy/internal/crypto"
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
			var layer crypto.Crypt
			var err error
			switch cfgLayer.Algorithm {
			case "tink":
				layer, err = crypto.NewTinkCrypt(cfgLayer.Keyset.Get())
			case "aes":
				mode := cfgLayer.Params["mode"]
				if mode != "gcm" {
					return nil, fmt.Errorf("unsupported AES mode: %s", mode)
				}
				layer, err = crypto.NewAESCrypt(cfgLayer.Keyset.Get())
			case "chacha20poly1305":
				layer, err = crypto.NewChaChaCrypt(cfgLayer.Keyset.Get())
			default:
				return nil, fmt.Errorf("unsupported crypto algorithm: %s", cfgLayer.Algorithm)
			}

			if err != nil {
				return nil, err
			}
			layers = append(layers, layer)
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

	auth := make(map[string]bool)
	log.Printf("Loading access keys from configuration")
	for _, user := range cfg.Auth.Users {
		accessKey := user.AccessKey.Get()
		if accessKey != "" {
			log.Printf("Loaded access key: %s", accessKey)
			auth[accessKey] = true
		} else {
			log.Printf("Warning: Empty access key found in configuration")
		}
	}
	log.Printf("Total access keys loaded: %d", len(auth))

	headerFormat := cfg.Auth.HeaderFormat.Get()
	if headerFormat == "" {
		log.Printf("Warning: No authorization header format specified, authentication will fail")
	}
	log.Printf("Loaded authorization header format: %s", headerFormat)

	return &Proxy{
		buckets:      buckets,
		auth:         auth,
		headerFormat: headerFormat,
	}, nil
}