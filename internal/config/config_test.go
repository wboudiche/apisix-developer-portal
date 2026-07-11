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

func TestPrometheusURLDefault(t *testing.T) {
	os.Unsetenv("PROMETHEUS_URL")
	if got := Load().PrometheusURL; got != "http://localhost:9099" {
		t.Fatalf("default PrometheusURL = %q, want http://localhost:9099", got)
	}
}

func TestPrometheusURLOverride(t *testing.T) {
	t.Setenv("PROMETHEUS_URL", "http://prometheus:9090")
	if got := Load().PrometheusURL; got != "http://prometheus:9090" {
		t.Fatalf("PrometheusURL = %q", got)
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

// prodEncKey is base64 of 32 raw bytes — the shape Validate requires in prod.
const prodEncKey = "cHJvZC1jcmVkZW50aWFsLWVuY3J5cHRpb24ta2V5MzI="

func TestValidateProductionAcceptsOverriddenSecrets(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-strong-secret-that-is-long-enough")
	t.Setenv("APISIX_ADMIN_KEY", "a-real-admin-key")
	t.Setenv("CREDENTIAL_ENC_KEY", prodEncKey)
	if err := Load().Validate(); err != nil {
		t.Fatalf("production with overridden secrets should be allowed, got %v", err)
	}
}

func TestValidateRejectsMalformedEncKeyInProd(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-strong-secret-that-is-long-enough")
	t.Setenv("APISIX_ADMIN_KEY", "a-real-admin-key")
	for _, bad := range []string{"not-base64!!!", "dG9vLXNob3J0"} { // invalid; base64("too-short")
		t.Setenv("CREDENTIAL_ENC_KEY", bad)
		err := Load().Validate()
		if err == nil || !strings.Contains(err.Error(), "CREDENTIAL_ENC_KEY") {
			t.Fatalf("prod must reject malformed enc key %q with a named error, got: %v", bad, err)
		}
	}
}

func TestValidateProductionNamesOnlyTheOffendingVar(t *testing.T) {
	t.Setenv("PORTAL_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-strong-secret") // overridden
	t.Setenv("APISIX_ADMIN_KEY", "")               // still dev default
	t.Setenv("CREDENTIAL_ENC_KEY", prodEncKey)     // overridden
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
	if strings.Contains(err.Error(), "CREDENTIAL_ENC_KEY") {
		t.Fatalf("CREDENTIAL_ENC_KEY was overridden; it should not be named: %v", err)
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
	t.Setenv("CREDENTIAL_ENC_KEY", prodEncKey)
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
	t.Setenv("CREDENTIAL_ENC_KEY", prodEncKey)
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatalf("prod with real secrets must pass: %v", err)
	}
}

func TestGatewayURLDefaultAndOverride(t *testing.T) {
	os.Unsetenv("APISIX_GATEWAY_URL")
	if got := Load().APISIXGatewayURL; got != "http://localhost:9080" {
		t.Errorf("default = %q, want http://localhost:9080", got)
	}
	t.Setenv("APISIX_GATEWAY_URL", "http://gw:9080")
	if got := Load().APISIXGatewayURL; got != "http://gw:9080" {
		t.Errorf("override = %q", got)
	}
}

func TestOIDCConfigDefaultsAndPredicate(t *testing.T) {
	os.Unsetenv("OIDC_ISSUER")
	c := Load()
	if c.OIDCConfigured() {
		t.Error("OIDCConfigured() = true, want false when OIDC_ISSUER is unset")
	}
	if c.OIDCClientIDClaim != "azp" {
		t.Errorf("OIDCClientIDClaim = %q, want azp", c.OIDCClientIDClaim)
	}
	t.Setenv("OIDC_ISSUER", "https://idp.example")
	c2 := Load()
	if !c2.OIDCConfigured() {
		t.Error("OIDCConfigured() = false, want true when OIDC_ISSUER is set")
	}
}

func TestSMTPConfigDefaultsAndPredicate(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	c := Load()
	if c.SMTPPort != "587" {
		t.Errorf("SMTPPort default = %q, want 587", c.SMTPPort)
	}
	if c.PortalBaseURL != "http://localhost:5173" {
		t.Errorf("PortalBaseURL default = %q", c.PortalBaseURL)
	}
	if c.SMTPConfigured() {
		t.Error("SMTPConfigured() = true with no host/from")
	}
	c.SMTPHost, c.SMTPFrom = "mail.example.com", "portal@example.com"
	if !c.SMTPConfigured() {
		t.Error("SMTPConfigured() = false with host+from set")
	}
	c.SMTPFrom = ""
	if c.SMTPConfigured() {
		t.Error("SMTPConfigured() = true with from empty")
	}
}

func TestSandboxConfigDefaultsAndPredicate(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	c := Load()
	if c.APISIXSandboxAdminURL != "http://localhost:19280" {
		t.Errorf("sandbox admin url = %q", c.APISIXSandboxAdminURL)
	}
	if c.APISIXSandboxGatewayURL != "http://localhost:9081" {
		t.Errorf("sandbox gateway url = %q", c.APISIXSandboxGatewayURL)
	}
	// Sandbox admin key defaults to the production admin key.
	if c.APISIXSandboxAdminKey != c.APISIXAdminKey {
		t.Errorf("sandbox admin key = %q, want = prod admin key", c.APISIXSandboxAdminKey)
	}
	if !c.SandboxConfigured() {
		t.Error("SandboxConfigured() = false, want true with both URLs set")
	}
	c.APISIXSandboxGatewayURL = ""
	if c.SandboxConfigured() {
		t.Error("SandboxConfigured() = true with gateway URL empty")
	}
}

func TestRequireEmailVerificationFlag(t *testing.T) {
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "1")
	if !Load().RequireEmailVerification {
		t.Fatal("flag=1 should enable RequireEmailVerification")
	}
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "")
	if Load().RequireEmailVerification {
		t.Fatal("unset flag should disable RequireEmailVerification")
	}
}

func TestValidateRejectsVerificationWithoutSMTP(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev") // even dev must fail-fast on this combination
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "1")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_FROM", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("Validate() must error when verification is on without SMTP")
	}
	t.Setenv("SMTP_HOST", "mail.example.com")
	t.Setenv("SMTP_FROM", "portal@example.com")
	if err := Load().Validate(); err != nil {
		t.Fatalf("Validate() with SMTP configured: %v", err)
	}
}
