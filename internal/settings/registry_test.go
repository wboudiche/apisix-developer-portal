package settings

import "testing"

func TestRegistryShape(t *testing.T) {
	if len(Registry) != 24 {
		t.Fatalf("registry has %d defs, want 24", len(Registry))
	}
	bootCritical := map[string]bool{
		"DATABASE_URL": true, "PORTAL_ADDR": true, "PORTAL_ENV": true,
		"JWT_SECRET": true, "CREDENTIAL_ENC_KEY": true,
	}
	secrets := map[string]bool{
		"SMTP_PASSWORD": true, "APISIX_ADMIN_KEY": true,
		"APISIX_SANDBOX_ADMIN_KEY": true, "DATABASE_URL": true,
		"JWT_SECRET": true, "CREDENTIAL_ENC_KEY": true,
	}
	seen := map[string]bool{}
	for _, d := range Registry {
		if seen[d.Key] {
			t.Fatalf("duplicate key %s", d.Key)
		}
		seen[d.Key] = true
		if d.Editable == bootCritical[d.Key] {
			t.Errorf("%s: Editable=%v, want %v", d.Key, d.Editable, !bootCritical[d.Key])
		}
		if d.Secret != secrets[d.Key] {
			t.Errorf("%s: Secret=%v, want %v", d.Key, d.Secret, secrets[d.Key])
		}
	}
	if _, ok := Lookup("SMTP_HOST"); !ok {
		t.Fatal("Lookup must find SMTP_HOST")
	}
	if _, ok := Lookup("NOPE"); ok {
		t.Fatal("Lookup must miss unknown keys")
	}
}

func TestValidateByType(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"PORTAL_BASE_URL", "http://portal.example.com", true},
		{"PORTAL_BASE_URL", "not a url", false},
		{"PORTAL_BASE_URL", "", false},    // required
		{"OIDC_ISSUER", "", true},         // optional url
		{"OIDC_ISSUER", "ftp://x", false}, // http(s) only
		{"SMTP_PORT", "1025", true},
		{"SMTP_PORT", "0", false},
		{"SMTP_PORT", "notanumber", false},
		{"SMTP_PORT", "", true}, // optional
		{"REQUIRE_EMAIL_VERIFICATION", "1", true},
		{"REQUIRE_EMAIL_VERIFICATION", "", true},
		{"REQUIRE_EMAIL_VERIFICATION", "true", false}, // strict "1"/""
		{"ADMIN_EMAIL", "admin@portal.local", true},
		{"ADMIN_EMAIL", "nope", false},
		{"TRUSTED_PROXIES", "10.0.0.0/8, 192.168.0.0/16", true},
		{"UPSTREAM_ALLOW_PRIVATE", "0", false}, // strict "1"/""
	}
	for _, c := range cases {
		d, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("%s not in registry", c.key)
		}
		err := Validate(d, c.value)
		if (err == nil) != c.ok {
			t.Errorf("Validate(%s, %q) err=%v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
}
