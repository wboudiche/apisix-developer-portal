// Package settings makes the portal's configuration runtime-editable: a
// declarative registry of every parameter, a DB-backed override store, an
// atomic effective-config snapshot, and the admin HTTP API. Spec:
// docs/superpowers/specs/2026-07-13-runtime-settings-design.md
package settings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"apisix-portal/internal/httpx"
)

type Type string

const (
	TypeString Type = "string"
	TypeBool   Type = "bool"
	TypePort   Type = "port"
	TypeURL    Type = "url"
	TypeEmail  Type = "email"
	TypeCSV    Type = "csv"
	// TypeProxyCIDRs is a comma-separated list of CIDR blocks, validated with
	// the same parser the server boots with (httpx.ParseProxyCIDRs) so a value
	// that saves is a value that boots.
	TypeProxyCIDRs Type = "proxyCIDRs"
)

type Def struct {
	Key      string
	Group    string
	Type     Type
	Secret   bool
	Editable bool
	Required bool
}

// Registry lists every portal parameter in UI display order. Boot-critical
// entries are Editable:false — visible, never writable.
var Registry = []Def{
	{Key: "PORTAL_ADDR", Group: "server", Type: TypeString},
	{Key: "PORTAL_ENV", Group: "server", Type: TypeString},
	{Key: "DATABASE_URL", Group: "server", Type: TypeString, Secret: true},
	{Key: "JWT_SECRET", Group: "server", Type: TypeString, Secret: true},
	{Key: "CREDENTIAL_ENC_KEY", Group: "server", Type: TypeString, Secret: true},
	{Key: "PORTAL_BASE_URL", Group: "portal", Type: TypeURL, Editable: true, Required: true},
	{Key: "ADMIN_EMAIL", Group: "portal", Type: TypeEmail, Editable: true, Required: true},
	{Key: "TRUSTED_PROXIES", Group: "portal", Type: TypeProxyCIDRs, Editable: true},
	{Key: "UPSTREAM_ALLOW_PRIVATE", Group: "portal", Type: TypeBool, Editable: true},
	{Key: "APISIX_ADMIN_URL", Group: "apisix", Type: TypeURL, Editable: true, Required: true},
	{Key: "APISIX_GATEWAY_URL", Group: "apisix", Type: TypeURL, Editable: true, Required: true},
	{Key: "APISIX_ADMIN_KEY", Group: "apisix", Type: TypeString, Secret: true, Editable: true, Required: true},
	{Key: "APISIX_SANDBOX_ADMIN_URL", Group: "sandbox", Type: TypeURL, Editable: true},
	{Key: "APISIX_SANDBOX_GATEWAY_URL", Group: "sandbox", Type: TypeURL, Editable: true},
	{Key: "APISIX_SANDBOX_ADMIN_KEY", Group: "sandbox", Type: TypeString, Secret: true, Editable: true},
	{Key: "SMTP_HOST", Group: "smtp", Type: TypeString, Editable: true},
	{Key: "SMTP_PORT", Group: "smtp", Type: TypePort, Editable: true},
	{Key: "SMTP_USERNAME", Group: "smtp", Type: TypeString, Editable: true},
	{Key: "SMTP_PASSWORD", Group: "smtp", Type: TypeString, Secret: true, Editable: true},
	{Key: "SMTP_FROM", Group: "smtp", Type: TypeEmail, Editable: true},
	{Key: "REQUIRE_EMAIL_VERIFICATION", Group: "policy", Type: TypeBool, Editable: true},
	{Key: "OIDC_ISSUER", Group: "oidc", Type: TypeURL, Editable: true},
	{Key: "OIDC_CLIENT_ID_CLAIM", Group: "oidc", Type: TypeString, Editable: true},
	{Key: "PROMETHEUS_URL", Group: "observability", Type: TypeURL, Editable: true},
}

var byKey = func() map[string]Def {
	m := make(map[string]Def, len(Registry))
	for _, d := range Registry {
		m[d.Key] = d
	}
	return m
}()

func Lookup(key string) (Def, bool) { d, ok := byKey[key]; return d, ok }

// Validate checks a candidate wire value against the def's type. Empty is
// allowed unless Required; bool is strictly "1" or "" (env semantics).
func Validate(d Def, value string) error {
	if value == "" {
		if d.Required {
			return fmt.Errorf("required")
		}
		return nil
	}
	switch d.Type {
	case TypeBool:
		if value != "1" {
			return fmt.Errorf(`must be "1" (on) or empty (off)`)
		}
	case TypePort:
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("must be a port between 1 and 65535")
		}
	case TypeURL:
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("must be an http(s) URL")
		}
	case TypeEmail:
		if !strings.Contains(value, "@") {
			return fmt.Errorf("must be an email address")
		}
	case TypeProxyCIDRs:
		if _, err := httpx.ParseProxyCIDRs(value); err != nil {
			return err
		}
	case TypeString, TypeCSV:
		// free-form
	}
	return nil
}
