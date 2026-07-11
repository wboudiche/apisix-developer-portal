package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong is returned when a password exceeds bcrypt's 72-byte limit
// (bcrypt silently truncates past 72 bytes, so we reject rather than lose entropy).
var ErrPasswordTooLong = errors.New("password must be at most 72 bytes")

type User struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Language string `json:"language"`
	// Verified reports whether the account's email address has been confirmed.
	// Only meaningful when REQUIRE_EMAIL_VERIFICATION is enabled; the column
	// defaults to TRUE so it never blocks pre-feature accounts.
	Verified bool `json:"-"`
}

// HashPassword returns a bcrypt hash (cost 12) of the plaintext password.
func HashPassword(plain string) (string, error) {
	if len(plain) > 72 {
		return "", ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
// Passwords beyond bcrypt's 72-byte limit are rejected outright (silent
// truncation must not make a longer password verify).
func CheckPassword(hash, plain string) bool {
	if len(plain) > 72 {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// GenerateVerifyToken returns a random email-verification token and its
// SHA-256 hex digest (only the digest is stored). Panics on CSPRNG failure,
// like subscriptions.GenerateKey.
func GenerateVerifyToken() (plain, hash string) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	plain = hex.EncodeToString(b)
	return plain, HashVerifyToken(plain)
}

// HashVerifyToken returns the SHA-256 hex digest under which a verification
// token is stored and looked up.
func HashVerifyToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
