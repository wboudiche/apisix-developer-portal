package auth

import "testing"

func TestIssueAndParseToken(t *testing.T) {
	tk := NewTokenizer("test-secret")
	token, err := tk.Issue(42, "dev@example.com", "developer")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := tk.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != 42 || claims.Email != "dev@example.com" || claims.Role != "developer" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseRejectsBadSecret(t *testing.T) {
	good := NewTokenizer("secret-a")
	bad := NewTokenizer("secret-b")
	token, _ := good.Issue(1, "a@b.c", "developer")
	if _, err := bad.Parse(token); err == nil {
		t.Fatal("token signed with a different secret must not verify")
	}
}
