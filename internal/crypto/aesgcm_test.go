package crypto_test

import (
	"strings"
	"testing"

	"apisix-portal/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// The key is base64 of 32 raw bytes (config.DevCredentialEncKey has the
	// same shape).
	c, err := crypto.New("ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ct, err := c.Encrypt("ax_live_secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "ax_live_secret" || ct == "" {
		t.Fatal("ciphertext must differ from plaintext and be non-empty")
	}
	if !strings.HasPrefix(ct, "v1:") {
		t.Fatalf("ciphertext must carry the v1: version prefix for future key rotation, got %q", ct)
	}
	pt, err := c.Decrypt(ct)
	if err != nil || pt != "ax_live_secret" {
		t.Fatalf("decrypt: got %q err %v", pt, err)
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	// Not base64 at all.
	if _, err := crypto.New("not-base64!!!"); err == nil {
		t.Fatal("non-base64 key must be rejected")
	}
	// Valid base64 but not 32 decoded bytes.
	if _, err := crypto.New("dG9vLXNob3J0"); err == nil { // base64("too-short")
		t.Fatal("key that does not decode to 32 bytes must be rejected")
	}
}

func TestDecryptRejectsUnversionedCiphertext(t *testing.T) {
	c, _ := crypto.New("ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=")
	if _, err := c.Decrypt("AAAAAAAAAAAAAAAAAAAAAAAAAAAA"); err == nil {
		t.Fatal("ciphertext without the v1: prefix must be rejected")
	}
}
