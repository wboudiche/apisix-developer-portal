package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// Cipher encrypts/decrypts short secrets (API keys) with AES-256-GCM. The key
// is supplied as base64 of 32 raw bytes (a raw ASCII passphrase would carry
// far less than 256 bits of entropy). Ciphertext is "v1:" + base64(nonce||ct);
// the version prefix lets a future key rotation re-encrypt v1 rows in place
// instead of forcing a DB wipe.
type Cipher struct{ aead cipher.AEAD }

// v1Prefix tags the ciphertext format/key version.
const v1Prefix = "v1:"

// New builds a Cipher from a base64-encoded 32-byte key
// (generate with: openssl rand -base64 32).
func New(b64Key string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, errors.New("credential encryption key must be base64 (of 32 raw bytes)")
	}
	if len(key) != 32 {
		return nil, errors.New("credential encryption key must decode to exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns "v1:" + base64(nonce || ciphertext). AAD is intentionally
// nil: ciphertexts are not bound to their DB row, so an attacker with DB
// *write* access could swap two rows' keys — out of scope for the at-rest
// threat model, and leaving AAD out keeps bulk re-encryption on key rotation
// row-context-free. The GCM tag still rejects forgery/corruption.
func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return v1Prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. Ciphertext without a known version prefix is
// rejected (it is either corrupt or a pre-encryption plaintext row).
func (c *Cipher) Decrypt(enc string) (string, error) {
	b64, ok := strings.CutPrefix(enc, v1Prefix)
	if !ok {
		return "", errors.New("ciphertext missing version prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	pt, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
