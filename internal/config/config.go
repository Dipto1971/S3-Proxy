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
	Users []ConfigUser `yaml:"users"`
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
