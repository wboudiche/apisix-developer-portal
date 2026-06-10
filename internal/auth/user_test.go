package auth

import (
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
	if _, err := HashPassword(string(long)); err == nil {
		t.Fatal("passwords longer than 72 bytes must be rejected")
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
