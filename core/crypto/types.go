package crypto

// Hash is a 32-byte BLAKE3 digest.
type Hash [32]byte

// PublicKey is an Ed25519 public key.
type PublicKey []byte

// PrivateKey is an Ed25519 private key.
type PrivateKey []byte

// Signature is an Ed25519 signature.
type Signature []byte
