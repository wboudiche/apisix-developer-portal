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

func TestValidateEmptyEnvTreatsAsProduction(t *testing.T) {
	t.Setenv("PORTAL_ENV", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	t.Setenv("CREDENTIAL_ENC_KEY", "")
	err := Load().Validate()
	if err == nil {
		t.Fatal("empty env (fail-closed to production) should reject dev secrets")
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
	t.Setenv("JWT_SECRET", "a-real-strong-secret-that-is-long-enough")
	t.Setenv("APISIX_ADMIN_KEY", "a-real-admin-key")
	t.Setenv("CREDENTIAL_ENC_KEY", "a-real-enc-key")
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

func TestValidateFailsClosedByDefault(t *testing.T) {
	// Unset env (default "") must be treated as production: built-in dev secrets rejected.
	t.Setenv("PORTAL_ENV", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	t.Setenv("CREDENTIAL_ENC_KEY", "")
	c := Load()
	if err := c.Validate(); err == nil {
		t.Fatal("empty PORTAL_ENV must be production and reject the dev JWT secret")
	}
}

func TestValidateRejectsShortJWTSecretInProd(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "short")
	t.Setenv("APISIX_ADMIN_KEY", "real-key-value")
	t.Setenv("CREDENTIAL_ENC_KEY", "real-enc-key-value")
	c := Load()
	if err := c.Validate(); err == nil {
		t.Fatal("prod must reject a JWT secret shorter than 32 bytes")
	}
}

func TestValidateAllowsDevExplicitly(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APISIX_ADMIN_KEY", "")
	t.Setenv("CREDENTIAL_ENC_KEY", "")
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatalf("explicit dev env must allow dev secrets: %v", err)
	}
}

func TestValidatePassesProdWithRealSecrets(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://...")
	t.Setenv("JWT_SECRET", "a-very-long-production-jwt-secret-32b+")
	t.Setenv("APISIX_ADMIN_KEY", "rotated-admin-key")
	t.Setenv("CREDENTIAL_ENC_KEY", "rotated-credential-enc-key-32bytes!!")
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatalf("prod with real secrets must pass: %v", err)
	}
}
