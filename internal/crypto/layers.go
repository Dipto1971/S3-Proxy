// internal/crypto/layers.gointernal/crypto/layers.go
package crypto

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
