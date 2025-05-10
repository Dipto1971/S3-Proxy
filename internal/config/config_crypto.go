// internal/config/config_crypto.go
package config

type ConfigCrypto struct {
	ID     string              `yaml:"id"`
	Layers []ConfigCryptoLayer `yaml:"layers"`
}

type ConfigCryptoLayer struct {
	Algorithm string             `yaml:"algorithm"`
	Keyset    *MultiSourceString `yaml:"keyset"`
	Params    map[string]string `yaml:"params"` 
}
