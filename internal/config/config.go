package config

import "os"

type Config struct {
	DatabaseURL    string
	Addr           string
	JWTSecret      string
	APISIXAdminURL string
	APISIXAdminKey string
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
		JWTSecret:      get("JWT_SECRET", "dev-secret-change-me"),
		APISIXAdminURL: get("APISIX_ADMIN_URL", "http://localhost:19180"),
		APISIXAdminKey: get("APISIX_ADMIN_KEY", "edd1c9f034335f136f87ad84b625c8f1"),
	}
}
