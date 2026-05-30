package config

import (
	"os"
	"testing"
)

func TestAdminEmailDefault(t *testing.T) {
	os.Unsetenv("ADMIN_EMAIL")
	if got := Load().AdminEmail; got != "admin@portal.local" {
		t.Fatalf("default AdminEmail = %q, want admin@portal.local", got)
	}
}

func TestAdminEmailOverride(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "boss@example.com")
	if got := Load().AdminEmail; got != "boss@example.com" {
		t.Fatalf("AdminEmail = %q, want boss@example.com", got)
	}
}
