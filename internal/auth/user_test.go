package auth

import "testing"

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
