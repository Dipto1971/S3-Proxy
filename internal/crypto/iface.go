// internal/crypto/iface.go
package crypto

type Crypt interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}
