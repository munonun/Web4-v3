package crypto

import "testing"

func TestGeneratedKeySignsAndVerifies(t *testing.T) {
	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	msg := []byte("message")
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if !Verify(pub, msg, sig) {
		t.Fatal("valid signature did not verify")
	}
}

func TestModifiedMessageFails(t *testing.T) {
	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	sig, err := Sign(priv, []byte("message"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if Verify(pub, []byte("modified"), sig) {
		t.Fatal("modified message verified")
	}
}

func TestModifiedSignatureFails(t *testing.T) {
	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	msg := []byte("message")
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[0] ^= 0xff

	if Verify(pub, msg, sig) {
		t.Fatal("modified signature verified")
	}
}

func TestMalformedKeysFailCleanly(t *testing.T) {
	if _, err := Sign(PrivateKey([]byte("bad")), []byte("message")); err == nil {
		t.Fatal("expected malformed private key error")
	}

	if Verify(PublicKey([]byte("bad")), []byte("message"), Signature(make([]byte, 64))) {
		t.Fatal("malformed public key verified")
	}

	pub, _, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	if Verify(pub, []byte("message"), Signature([]byte("bad"))) {
		t.Fatal("malformed signature verified")
	}
}
