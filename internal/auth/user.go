package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong is returned when a password exceeds bcrypt's 72-byte limit
// (bcrypt silently truncates past 72 bytes, so we reject rather than lose entropy).
var ErrPasswordTooLong = errors.New("password must be at most 72 bytes")

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
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
