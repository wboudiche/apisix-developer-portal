package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

const (
	DevJWTSecret        = "dev-secret-change-me"
	DevAPISIXAdminKey   = "edd1c9f034335f136f87ad84b625c8f1"
	DevCredentialEncKey = "ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=" // base64(32 bytes)
)

type Config struct {
	DatabaseURL              string
	Addr                     string
	JWTSecret                string
	APISIXAdminURL           string
	APISIXGatewayURL         string // base URL of the APISIX data-plane (gateway), used by the try-it proxy
	APISIXAdminKey           string
	APISIXSandboxAdminURL    string
	APISIXSandboxGatewayURL  string
	APISIXSandboxAdminKey    string
	AdminEmail               string
	Env                      string
	CredentialEncKey         string
	TrustedProxies           string // comma-separated CIDRs whose X-Forwarded-For is trusted
	PrometheusURL            string // base URL of the Prometheus read API; empty disables usage metrics
	OIDCIssuer               string // OIDC issuer URL; empty means OAuth2 is disabled
	OIDCClientIDClaim        string // JWT claim that carries the client_id (default "azp")
	SMTPHost                 string
	SMTPPort                 string
	SMTPUsername             string
	SMTPPassword             string
	SMTPFrom                 string
	PortalBaseURL            string
	RequireEmailVerification bool
	UpstreamAllowPrivate     bool
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment, applying dev defaults.
func Load() Config {
	return Config{
		DatabaseURL:              get("DATABASE_URL", "postgres://portal:portal@localhost:5432/portal?sslmode=disable"),
		Addr:                     get("PORTAL_ADDR", ":8080"),
		JWTSecret:                get("JWT_SECRET", DevJWTSecret),
		APISIXAdminURL:           get("APISIX_ADMIN_URL", "http://localhost:19180"),
		APISIXGatewayURL:         get("APISIX_GATEWAY_URL", "http://localhost:9080"),
		APISIXAdminKey:           get("APISIX_ADMIN_KEY", DevAPISIXAdminKey),
		APISIXSandboxAdminURL:    get("APISIX_SANDBOX_ADMIN_URL", "http://localhost:19280"),
		APISIXSandboxGatewayURL:  get("APISIX_SANDBOX_GATEWAY_URL", "http://localhost:9081"),
		APISIXSandboxAdminKey:    get("APISIX_SANDBOX_ADMIN_KEY", get("APISIX_ADMIN_KEY", DevAPISIXAdminKey)),
		AdminEmail:               get("ADMIN_EMAIL", "admin@portal.local"),
		Env:                      get("PORTAL_ENV", ""),
		CredentialEncKey:         get("CREDENTIAL_ENC_KEY", DevCredentialEncKey),
		TrustedProxies:           get("TRUSTED_PROXIES", ""),
		PrometheusURL:            get("PROMETHEUS_URL", "http://localhost:9099"),
		OIDCIssuer:               get("OIDC_ISSUER", ""),
		OIDCClientIDClaim:        get("OIDC_CLIENT_ID_CLAIM", "azp"),
		SMTPHost:                 get("SMTP_HOST", ""),
		SMTPPort:                 get("SMTP_PORT", "587"),
		SMTPUsername:             get("SMTP_USERNAME", ""),
		SMTPPassword:             get("SMTP_PASSWORD", ""),
		SMTPFrom:                 get("SMTP_FROM", ""),
		PortalBaseURL:            get("PORTAL_BASE_URL", "http://localhost:5173"),
		RequireEmailVerification: get("REQUIRE_EMAIL_VERIFICATION", "") == "1",
		UpstreamAllowPrivate:     get("UPSTREAM_ALLOW_PRIVATE", "") == "1",
	}
}

// isDevLike reports whether the environment tolerates the built-in dev secrets.
func (c Config) isDevLike() bool {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "dev", "development", "test":
		return true
	default:
		return false
	}
}

// UsesDevSecrets reports whether any secret is still the built-in dev default.
func (c Config) UsesDevSecrets() bool {
	return c.JWTSecret == DevJWTSecret || c.APISIXAdminKey == DevAPISIXAdminKey || c.CredentialEncKey == DevCredentialEncKey
}

// OIDCConfigured reports whether OAuth2 (bring-your-own OIDC) is wired up.
func (c Config) OIDCConfigured() bool { return c.OIDCIssuer != "" }

// SMTPConfigured reports whether email notifications are wired up. When false,
// the notifier is unset and every notification call is a no-op.
func (c Config) SMTPConfigured() bool { return c.SMTPHost != "" && c.SMTPFrom != "" }

// SandboxConfigured reports whether the dedicated sandbox gateway is wired up.
// When false, the portal runs production-only and all sandbox features are inert.
func (c Config) SandboxConfigured() bool {
	return c.APISIXSandboxAdminURL != "" && c.APISIXSandboxGatewayURL != ""
}

// Validate returns an error if the configuration is unsafe to run in a
// production-like environment (PORTAL_ENV not dev/test): specifically, if any
// secret is still its built-in dev default, or if the JWT secret is too short.
// Unset PORTAL_ENV is treated as production (fail-closed). In a dev-like
// environment it always returns nil (callers may still warn via UsesDevSecrets).
func (c Config) Validate() error {
	if c.RequireEmailVerification && !c.SMTPConfigured() {
		return fmt.Errorf("REQUIRE_EMAIL_VERIFICATION=1 needs a mail server: set SMTP_HOST and SMTP_FROM, or unset REQUIRE_EMAIL_VERIFICATION")
	}
	if c.isDevLike() {
		return nil
	}
	var bad []string
	if c.JWTSecret == DevJWTSecret {
		bad = append(bad, "JWT_SECRET")
	}
	if c.APISIXAdminKey == DevAPISIXAdminKey {
		bad = append(bad, "APISIX_ADMIN_KEY")
	}
	if c.CredentialEncKey == DevCredentialEncKey {
		bad = append(bad, "CREDENTIAL_ENC_KEY")
	}
	if len(bad) > 0 {
		return fmt.Errorf("refusing to start in %q environment with built-in dev secrets; set %s to secure value(s)", c.Env, strings.Join(bad, ", "))
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes in %q environment", c.Env)
	}
	// Same shape check crypto.New applies at boot — done here so a malformed
	// key fails fast with a config error instead of mid-startup.
	if key, err := base64.StdEncoding.DecodeString(c.CredentialEncKey); err != nil || len(key) != 32 {
		return fmt.Errorf("CREDENTIAL_ENC_KEY must be base64 of 32 raw bytes (openssl rand -base64 32) in %q environment", c.Env)
	}
	return nil
}
