// internal/config/config.go
package config

type Config struct {
	ListenAddr string           `yaml:"listen_addr"`
	Crypto     []ConfigCrypto   `yaml:"crypto"`
	S3Clients  []ConfigS3Client `yaml:"s3_clients"`
	S3Buckets  []ConfigS3Bucket `yaml:"s3_buckets"`
	Auth       ConfigAuth       `yaml:"auth"`
}

type ConfigAuth struct {
	HeaderFormat MultiSourceString `yaml:"header_format"`
	Users        []ConfigUser      `yaml:"users"`
}

// IsValidAccessKey checks if the given access key is valid
func (auth *ConfigAuth) IsValidAccessKey(key string) bool {
	for _, user := range auth.Users {
		if user.AccessKey.Get() == key {
			return true
		}
	}
	return false
}

type ConfigUser struct {
	AccessKey MultiSourceString `yaml:"access_key"`
}

type ConfigS3Bucket struct {
	BucketName string                  `yaml:"bucket_name"`
	Backends   []ConfigS3BucketBackend `yaml:"backends"`
}

type ConfigS3BucketBackend struct {
	S3ClientID   string `yaml:"s3_client_id"`
	S3BucketName string `yaml:"s3_bucket_name"`
	CryptoID     string `yaml:"crypto_id"`
}

// GetCryptoIDForBucket returns the crypto ID for the first backend of the specified bucket
func GetCryptoIDForBucket(cfg *Config, bucketName string) string {
	for _, bucket := range cfg.S3Buckets {
		if bucket.BucketName == bucketName {
			if len(bucket.Backends) > 0 {
				return bucket.Backends[0].CryptoID
			}
		}
	}
	return ""
}
