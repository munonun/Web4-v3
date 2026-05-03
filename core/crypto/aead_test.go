package crypto

import "testing"

func TestEncryptDecryptXChaCha20Roundtrip(t *testing.T) {
	key, err := RandomBytes(XChaCha20KeySize)
	if err != nil {
		t.Fatalf("random key: %v", err)
	}
	nonce, err := RandomBytes(XChaCha20NonceSize)
	if err != nil {
		t.Fatalf("random nonce: %v", err)
	}

	plaintext := []byte("secret")
	aad := []byte("context")
	ciphertext, err := EncryptXChaCha20(key, nonce, plaintext, aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := DecryptXChaCha20(key, nonce, ciphertext, aad)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptXChaCha20WrongAADFails(t *testing.T) {
	key, err := RandomBytes(XChaCha20KeySize)
	if err != nil {
		t.Fatalf("random key: %v", err)
	}
	nonce, err := RandomBytes(XChaCha20NonceSize)
	if err != nil {
		t.Fatalf("random nonce: %v", err)
	}

	ciphertext, err := EncryptXChaCha20(key, nonce, []byte("secret"), []byte("aad"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if _, err := DecryptXChaCha20(key, nonce, ciphertext, []byte("wrong")); err == nil {
		t.Fatal("expected wrong AAD to fail")
	}
}

func TestXChaCha20WrongSizesReturnError(t *testing.T) {
	key := make([]byte, XChaCha20KeySize)
	nonce := make([]byte, XChaCha20NonceSize)

	if _, err := EncryptXChaCha20(key[:XChaCha20KeySize-1], nonce, nil, nil); err == nil {
		t.Fatal("expected invalid key size error")
	}
	if _, err := EncryptXChaCha20(key, nonce[:XChaCha20NonceSize-1], nil, nil); err == nil {
		t.Fatal("expected invalid nonce size error")
	}
	if _, err := DecryptXChaCha20(key[:XChaCha20KeySize-1], nonce, nil, nil); err == nil {
		t.Fatal("expected invalid decrypt key size error")
	}
}

func TestRandomBytesRejectsNegativeCount(t *testing.T) {
	if _, err := RandomBytes(-1); err == nil {
		t.Fatal("expected negative count error")
	}
}
