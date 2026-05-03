package crypto

import (
	"encoding/hex"
	"fmt"

	"github.com/zeebo/blake3"
)

// HashBytes returns the BLAKE3-256 digest of data.
func HashBytes(data []byte) Hash {
	return blake3.Sum256(data)
}

// HashHex returns the lowercase hexadecimal encoding of h.
func HashHex(h Hash) string {
	return hex.EncodeToString(h[:])
}

// ParseHashHex parses a 32-byte lowercase or uppercase hexadecimal hash.
func ParseHashHex(s string) (Hash, error) {
	var h Hash

	if len(s) != hex.EncodedLen(len(h)) {
		return h, fmt.Errorf("invalid hash hex length: got %d, want %d", len(s), hex.EncodedLen(len(h)))
	}

	_, err := hex.Decode(h[:], []byte(s))
	if err != nil {
		return Hash{}, err
	}

	return h, nil
}
