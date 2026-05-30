package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	DevJWTSecret      = "dev-secret-change-me"
	DevAPISIXAdminKey = "edd1c9f034335f136f87ad84b625c8f1"
)

type Config struct {
	DatabaseURL    string
	Addr           string
	JWTSecret      string
	APISIXAdminURL string
	APISIXAdminKey string
	AdminEmail     string
	Env            string
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
		DatabaseURL:    get("DATABASE_URL", "postgres://portal:portal@localhost:5432/portal?sslmode=disable"),
		Addr:           get("PORTAL_ADDR", ":8080"),
		JWTSecret:      get("JWT_SECRET", DevJWTSecret),
		APISIXAdminURL: get("APISIX_ADMIN_URL", "http://localhost:19180"),
		APISIXAdminKey: get("APISIX_ADMIN_KEY", DevAPISIXAdminKey),
		AdminEmail:     get("ADMIN_EMAIL", "admin@portal.local"),
		Env:            get("PORTAL_ENV", "dev"),
	}
}

// isDevLike reports whether the environment tolerates the built-in dev secrets.
func (c Config) isDevLike() bool {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "", "dev", "development", "test":
		return true
	default:
		return false
	}
}

// UsesDevSecrets reports whether any secret is still the built-in dev default.
func (c Config) UsesDevSecrets() bool {
	return c.JWTSecret == DevJWTSecret || c.APISIXAdminKey == DevAPISIXAdminKey
}

// Validate returns an error if the configuration is unsafe to run in a
// production-like environment (PORTAL_ENV not dev/test): specifically, if any
// secret is still its built-in dev default. In a dev-like environment it always
// returns nil (callers may still warn via UsesDevSecrets).
func (c Config) Validate() error {
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
	if len(bad) > 0 {
		return fmt.Errorf("refusing to start in %q environment with built-in dev secrets; set %s to secure value(s)", c.Env, strings.Join(bad, ", "))
	}
	return nil
}
