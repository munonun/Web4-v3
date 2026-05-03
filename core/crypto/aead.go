package crypto

import (
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	XChaCha20KeySize   = chacha20poly1305.KeySize
	XChaCha20NonceSize = chacha20poly1305.NonceSizeX
)

// EncryptXChaCha20 encrypts plaintext with XChaCha20-Poly1305.
func EncryptXChaCha20(key []byte, nonce []byte, plaintext []byte, aad []byte) ([]byte, error) {
	if err := validateXChaCha20Inputs(key, nonce); err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	return aead.Seal(nil, nonce, plaintext, aad), nil
}

// DecryptXChaCha20 decrypts ciphertext with XChaCha20-Poly1305.
func DecryptXChaCha20(key []byte, nonce []byte, ciphertext []byte, aad []byte) ([]byte, error) {
	if err := validateXChaCha20Inputs(key, nonce); err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	return aead.Open(nil, nonce, ciphertext, aad)
}

// RandomBytes returns n cryptographically secure random bytes.
func RandomBytes(n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("invalid random byte count: %d", n)
	}

	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}

	return b, nil
}

func validateXChaCha20Inputs(key []byte, nonce []byte) error {
	if len(key) != XChaCha20KeySize {
		return fmt.Errorf("invalid XChaCha20 key length: got %d, want %d", len(key), XChaCha20KeySize)
	}
	if len(nonce) != XChaCha20NonceSize {
		return fmt.Errorf("invalid XChaCha20 nonce length: got %d, want %d", len(nonce), XChaCha20NonceSize)
	}

	return nil
}
