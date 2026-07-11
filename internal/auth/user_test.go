package auth

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "s3cret!" {
		t.Fatal("hash must not equal the plaintext")
	}
	if !CheckPassword(hash, "s3cret!") {
		t.Fatal("correct password should verify")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password must not verify")
	}
}

func TestHashPasswordRejectsTooLong(t *testing.T) {
	// bcrypt silently truncates at 72 bytes — reject explicitly.
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := HashPassword(string(long)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong, got %v", err)
	}
}

func TestHashPasswordUsesCost12(t *testing.T) {
	h, err := HashPassword("a-normal-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil || cost != 12 {
		t.Fatalf("want cost 12, got %d (err %v)", cost, err)
	}
}

func TestGenerateVerifyToken(t *testing.T) {
	plain, hash := GenerateVerifyToken()
	if len(plain) != 32 {
		t.Fatalf("plain len = %d, want 32 hex chars", len(plain))
	}
	if hash != HashVerifyToken(plain) {
		t.Fatal("hash must equal HashVerifyToken(plain)")
	}
	if hash == plain {
		t.Fatal("hash must not equal the plain token")
	}
	p2, _ := GenerateVerifyToken()
	if p2 == plain {
		t.Fatal("two tokens must differ")
	}
}

func TestCheckPasswordRejectsTooLong(t *testing.T) {
	// HashPassword already rejects >72 bytes, but CheckPassword must too
	// (symmetric guard against silent truncation at verification time).
	validHash, err := HashPassword("a-valid-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if CheckPassword(validHash, string(long)) {
		t.Fatal("CheckPassword must reject passwords longer than 72 bytes")
	}
}
