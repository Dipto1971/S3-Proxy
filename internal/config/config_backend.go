// internal/config/config_backend.go
package config

type ConfigS3Client struct {
	ID        string            `yaml:"id"`
	Endpoint  string            `yaml:"endpoint"`
	Region    string            `yaml:"region"`
	AccessKey MultiSourceString `yaml:"access_key"`
	SecretKey MultiSourceString `yaml:"secret_key"`
}
