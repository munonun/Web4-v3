package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
)

// GenerateKeypair creates a new Ed25519 public/private key pair.
func GenerateKeypair() (PublicKey, PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	return PublicKey(pub), PrivateKey(priv), nil
}

// Sign signs msg with an Ed25519 private key.
func Sign(priv PrivateKey, msg []byte) (Signature, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: got %d, want %d", len(priv), ed25519.PrivateKeySize)
	}

	sig := ed25519.Sign(ed25519.PrivateKey(priv), msg)
	return Signature(sig), nil
}

// Verify reports whether sig is a valid Ed25519 signature of msg by pub.
func Verify(pub PublicKey, msg []byte, sig Signature) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}
