// internal/crypto/layers.go
package crypto

import (
	"fmt"
	"s3-proxy/internal/config"
)

func NewMultiLayerCrypt(layers ...Crypt) *MultiLayerCrypt {
	return &MultiLayerCrypt{layers: layers}
}

type MultiLayerCrypt struct {
	layers []Crypt
}

func (c *MultiLayerCrypt) Encrypt(data []byte) ([]byte, error) {
	var err error
	for _, layer := range c.layers {
		data, err = layer.Encrypt(data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (c *MultiLayerCrypt) Decrypt(data []byte) ([]byte, error) {
	var err error
	for i := len(c.layers) - 1; i >= 0; i-- {
		data, err = c.layers[i].Decrypt(data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// NewCryptFromConfig creates a crypto instance from configuration by crypto ID
func NewCryptFromConfig(cfg *config.Config, cryptoID string) (Crypt, error) {
	// Find the crypto configuration by ID
	var cryptoConfig *config.ConfigCrypto
	for _, crypto := range cfg.Crypto {
		if crypto.ID == cryptoID {
			cryptoConfig = &crypto
			break
		}
	}

	if cryptoConfig == nil {
		return nil, fmt.Errorf("crypto configuration not found for ID: %s", cryptoID)
	}

	// Build layers from configuration
	layers := make([]Crypt, 0, len(cryptoConfig.Layers))
	for _, cfgLayer := range cryptoConfig.Layers {
		var layer Crypt
		var err error
		switch cfgLayer.Algorithm {
		case "tink":
			layer, err = NewTinkCrypt(cfgLayer.Keyset.Get())
		case "aes":
			mode := cfgLayer.Params["mode"]
			if mode != "gcm" {
				return nil, fmt.Errorf("unsupported AES mode: %s", mode)
			}
			layer, err = NewAESCrypt(cfgLayer.Keyset.Get())
		case "chacha20poly1305":
			layer, err = NewChaChaCrypt(cfgLayer.Keyset.Get())
		default:
			return nil, fmt.Errorf("unsupported crypto algorithm: %s", cfgLayer.Algorithm)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create %s crypto layer: %v", cfgLayer.Algorithm, err)
		}
		layers = append(layers, layer)
	}

	return NewMultiLayerCrypt(layers...), nil
}
