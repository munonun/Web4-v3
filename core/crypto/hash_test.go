package crypto

import "testing"

func TestHashBytesStable(t *testing.T) {
	h1 := HashBytes([]byte("web4"))
	h2 := HashBytes([]byte("web4"))

	if h1 != h2 {
		t.Fatal("same input produced different hashes")
	}
}

func TestHashBytesDifferentInputs(t *testing.T) {
	h1 := HashBytes([]byte("web4"))
	h2 := HashBytes([]byte("web5"))

	if h1 == h2 {
		t.Fatal("different inputs produced same hash")
	}
}

func TestHashHexRoundtrip(t *testing.T) {
	h := HashBytes([]byte("roundtrip"))
	parsed, err := ParseHashHex(HashHex(h))
	if err != nil {
		t.Fatalf("parse hash hex: %v", err)
	}
	if parsed != h {
		t.Fatal("parsed hash did not match original")
	}
}

func TestParseHashHexRejectsInvalidLength(t *testing.T) {
	if _, err := ParseHashHex("abc"); err == nil {
		t.Fatal("expected invalid length error")
	}
}
