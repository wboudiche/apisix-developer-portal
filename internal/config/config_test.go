package config

import (
	"os"
	"strings"
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

func TestValidateDevAllowsDefaults(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	if err := Load().Validate(); err != nil {
		t.Fatalf("dev with defaults should be allowed, got %v", err)
	}
}

func TestValidateEmptyEnvAllowsDefaults(t *testing.T) {
	t.Setenv("PORTAL_ENV", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	if err := Load().Validate(); err != nil {
		t.Fatalf("empty env (defaults to dev) should be allowed, got %v", err)
	}
}

func TestValidateProductionRejectsDefaultSecrets(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	err := Load().Validate()
	if err == nil {
		t.Fatal("production with dev-default secrets must be rejected")
	}
	// Both offending vars should be named so the operator knows what to set.
	if !strings.Contains(err.Error(), "JWT_SECRET") || !strings.Contains(err.Error(), "APISIX_ADMIN_KEY") {
		t.Fatalf("error should name both offending vars, got: %v", err)
	}
}

func TestValidateProductionAcceptsOverriddenSecrets(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-strong-secret")
	t.Setenv("APISIX_ADMIN_KEY", "a-real-admin-key")
	if err := Load().Validate(); err != nil {
		t.Fatalf("production with overridden secrets should be allowed, got %v", err)
	}
}

func TestValidateProductionNamesOnlyTheOffendingVar(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-strong-secret") // overridden
	t.Setenv("APISIX_ADMIN_KEY", "")               // still dev default
	err := Load().Validate()
	if err == nil {
		t.Fatal("production with one dev-default secret must be rejected")
	}
	if strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("JWT_SECRET was overridden; it should not be named: %v", err)
	}
	if !strings.Contains(err.Error(), "APISIX_ADMIN_KEY") {
		t.Fatalf("APISIX_ADMIN_KEY is still the dev default; it must be named: %v", err)
	}
}

func TestValidateTestEnvAllowsDefaults(t *testing.T) {
	t.Setenv("PORTAL_ENV", "test")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	if err := Load().Validate(); err != nil {
		t.Fatalf("test env should be treated as non-production, got %v", err)
	}
}
