package security

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := []byte(`{"cookies":"secret"}`)
	enc, err := EncryptSession(plain, "master-key")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == string(plain) {
		t.Fatal("ciphertext equals plaintext")
	}
	dec, err := DecryptSession(enc, "master-key")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("round trip mismatch: %q", dec)
	}
	if _, err := DecryptSession(enc, "wrong-key"); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}

func TestSignWebhookDeterministic(t *testing.T) {
	payload := []byte(`{"event":"payment.paid"}`)
	a := SignWebhook("secret", payload)
	b := SignWebhook("secret", payload)
	if a != b {
		t.Fatal("signature is not deterministic")
	}
	if a == SignWebhook("other", payload) {
		t.Fatal("signature does not depend on the secret")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
}

func TestHashTokenStable(t *testing.T) {
	if HashToken("abc") != HashToken("abc") {
		t.Fatal("hash is not stable")
	}
	if HashToken("abc") == HashToken("abcd") {
		t.Fatal("different tokens hashed identically")
	}
}
