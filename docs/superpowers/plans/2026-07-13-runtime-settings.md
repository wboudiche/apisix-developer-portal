# Runtime-Editable Portal Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admins view every portal parameter and edit every non-boot-critical one from a new admin Paramètres tab; changes validate, probe, persist to Postgres, and apply live — no restart.

**Architecture:** A declarative registry describes all 24 settings. `settings.Service` overlays DB rows (secrets encrypted with the existing credential cipher) onto env defaults into an immutable snapshot behind `atomic.Pointer`; savers validate → probe → persist → swap → run apply-hooks. Live consumers either read the snapshot per use (SMTP sender, verification gate, URLs) or are swappable holders rebuilt by hooks (APISIX gateway clients).

**Tech Stack:** Go (chi, pgx, atomic), Postgres migrations via `internal/db`, React + TypeScript + vitest.

**Spec:** `docs/superpowers/specs/2026-07-13-runtime-settings-design.md`

## Global Constraints

- Boot-critical, read-only in UI/API: `DATABASE_URL`, `PORTAL_ADDR`, `PORTAL_ENV`, `JWT_SECRET`, `CREDENTIAL_ENC_KEY` (registry `Editable:false`).
- Secret keys (write-only, encrypted at rest with `crypto.Cipher`): `SMTP_PASSWORD`, `APISIX_ADMIN_KEY`, `APISIX_SANDBOX_ADMIN_KEY`. GET never returns their values — not even boot-critical secrets' values (only `set`).
- Precedence: DB row wins over env; no row = env default; reset = DELETE row.
- Wire values are strings; `bool`-typed keys accept exactly `"1"` (on) or `""` (off).
- Cross-field invariant (non-forceable): effective `REQUIRE_EMAIL_VERIFICATION="1"` requires effective `SMTP_HOST` and `SMTP_FROM` non-empty, evaluated with candidate values overlaid.
- Probes (5 s timeout, run only when the PUT/test touches that group's keys; PUT failures forceable with `force:true`): APISIX `GET {ADMIN_URL}/apisix/admin/routes?page_size=1` with candidate key expects 200; SMTP TCP dial + EHLO.
- All settings endpoints behind `requireAdmin`. Every successful PUT/DELETE writes one `portal_settings_audit` row per changed key (secrets logged as `(secret)`); this table realizes the spec's audit requirement (the existing `events` table is app-scoped and unsuitable — deliberate spec correction).
- Verification gate contract preserved: feature off ⇒ verify/resend answer 404.
- Migrations: `0021_portal_settings.sql`. Backend tests: `go test ./internal/...` (DB tests skip without Postgres; the compose stack provides one at localhost:5432). Frontend: `cd web && npx vitest run src/...` — NEVER bare `npx vitest run` (e2e/*.spec.ts are Playwright files that fail under vitest, pre-existing).
- `gofmt -l` clean on all touched Go packages before each commit. Commit style matches repo history.

## File structure (locked)

```
internal/settings/registry.go       # the declarative table + validation
internal/settings/registry_test.go
internal/settings/service.go        # snapshot, load/overlay, Set/Reset, hooks, audit
internal/settings/service_test.go
internal/settings/probe.go          # APISIX + SMTP probes, ProbeFor dispatch
internal/settings/probe_test.go
internal/settings/handler.go        # admin HTTP API
internal/settings/handler_test.go
internal/apisix/swappable.go        # SwappableGateway
internal/apisix/swappable_test.go
internal/notify/dynamic.go          # DynamicSender
internal/db/migrations/0021_portal_settings.sql
web/src/pages/admin/SettingsPage.tsx (+ .test.tsx)
web/src/styles/admin-settings.css
```

---

### Task 1: Config completes — `UPSTREAM_ALLOW_PRIVATE` joins Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/server/server.go:88` (drop the raw `os.Getenv`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.UpstreamAllowPrivate bool` read from `UPSTREAM_ALLOW_PRIVATE` (`"1"` = true). `server.New` uses `cfg.UpstreamAllowPrivate` instead of `os.Getenv("UPSTREAM_ALLOW_PRIVATE") == "1"`. Task 2's registry maps every registry key onto a `Config` field, so this must exist first.

- [ ] **Step 1: Write the failing test** — append to `internal/config/config_test.go`:

```go
func TestUpstreamAllowPrivateFlag(t *testing.T) {
	t.Setenv("UPSTREAM_ALLOW_PRIVATE", "1")
	if !Load().UpstreamAllowPrivate {
		t.Fatal("flag=1 should enable UpstreamAllowPrivate")
	}
	t.Setenv("UPSTREAM_ALLOW_PRIVATE", "")
	if Load().UpstreamAllowPrivate {
		t.Fatal("unset flag should disable UpstreamAllowPrivate")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run TestUpstreamAllowPrivate -v`
Expected: FAIL — field undefined (compile error).

- [ ] **Step 3: Implement** — in `config.go` add the struct field `UpstreamAllowPrivate bool` (next to `RequireEmailVerification`) and in `Load()`:

```go
		UpstreamAllowPrivate: get("UPSTREAM_ALLOW_PRIVATE", "") == "1",
```

In `internal/server/server.go` replace:

```go
	allowPrivate := os.Getenv("UPSTREAM_ALLOW_PRIVATE") == "1"
```

with:

```go
	allowPrivate := cfg.UpstreamAllowPrivate
```

and remove the now-unused `"os"` import if nothing else uses it (check — `server.go` currently imports `os` only for this).

- [ ] **Step 4: Verify pass + full config/server build**

Run: `gofmt -l internal/config internal/server && go build ./... && go test ./internal/config/ -v | tail -5`
Expected: no gofmt output, build ok, tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/server/server.go
git commit -m "refactor(config): UPSTREAM_ALLOW_PRIVATE becomes a Config field"
```

---

### Task 2: Settings registry

**Files:**
- Create: `internal/settings/registry.go`
- Test: `internal/settings/registry_test.go`

**Interfaces:**
- Consumes: `config.Config` field names (Task 1 complete).
- Produces (used by every later backend task):

```go
package settings

type Type string
const (
	TypeString Type = "string"
	TypeBool   Type = "bool"   // wire "1" or ""
	TypePort   Type = "port"   // "1".."65535", or "" if optional
	TypeURL    Type = "url"    // http(s)://..., or "" if optional
	TypeEmail  Type = "email"  // must contain "@", or "" if optional
	TypeCSV    Type = "csv"    // comma-separated, free-form
)

type Def struct {
	Key      string // the env var name, e.g. "SMTP_HOST"
	Group    string // "server"|"portal"|"apisix"|"sandbox"|"smtp"|"policy"|"oidc"|"observability"
	Type     Type
	Secret   bool
	Editable bool
	Required bool // "" not allowed when editable and required
}

var Registry []Def          // ordered as the UI displays
func Lookup(key string) (Def, bool)
func Validate(d Def, value string) error  // type/format check; nil for valid
```

- Registry contents (exact rows, in this order):

| Key | Group | Type | Secret | Editable | Required |
|---|---|---|---|---|---|
| PORTAL_ADDR | server | string | no | no | – |
| PORTAL_ENV | server | string | no | no | – |
| DATABASE_URL | server | string | yes | no | – |
| JWT_SECRET | server | string | yes | no | – |
| CREDENTIAL_ENC_KEY | server | string | yes | no | – |
| PORTAL_BASE_URL | portal | url | no | yes | yes |
| ADMIN_EMAIL | portal | email | no | yes | yes |
| TRUSTED_PROXIES | portal | csv | no | yes | no |
| UPSTREAM_ALLOW_PRIVATE | portal | bool | no | yes | no |
| APISIX_ADMIN_URL | apisix | url | no | yes | yes |
| APISIX_GATEWAY_URL | apisix | url | no | yes | yes |
| APISIX_ADMIN_KEY | apisix | string | yes | yes | yes |
| APISIX_SANDBOX_ADMIN_URL | sandbox | url | no | yes | no |
| APISIX_SANDBOX_GATEWAY_URL | sandbox | url | no | yes | no |
| APISIX_SANDBOX_ADMIN_KEY | sandbox | string | yes | yes | no |
| SMTP_HOST | smtp | string | no | yes | no |
| SMTP_PORT | smtp | port | no | yes | no |
| SMTP_USERNAME | smtp | string | no | yes | no |
| SMTP_PASSWORD | smtp | string | yes | yes | no |
| SMTP_FROM | smtp | email | no | yes | no |
| REQUIRE_EMAIL_VERIFICATION | policy | bool | no | yes | no |
| OIDC_ISSUER | oidc | url | no | yes | no |
| OIDC_CLIENT_ID_CLAIM | oidc | string | no | yes | no |
| PROMETHEUS_URL | observability | url | no | yes | no |

(Note DATABASE_URL is marked secret: it can embed a password; it is also non-editable, so it renders as "set" only.)

- [ ] **Step 1: Write the failing tests** — `internal/settings/registry_test.go`:

```go
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
		{"PORTAL_BASE_URL", "", false},                    // required
		{"OIDC_ISSUER", "", true},                         // optional url
		{"OIDC_ISSUER", "ftp://x", false},                 // http(s) only
		{"SMTP_PORT", "1025", true},
		{"SMTP_PORT", "0", false},
		{"SMTP_PORT", "notanumber", false},
		{"SMTP_PORT", "", true},                           // optional
		{"REQUIRE_EMAIL_VERIFICATION", "1", true},
		{"REQUIRE_EMAIL_VERIFICATION", "", true},
		{"REQUIRE_EMAIL_VERIFICATION", "true", false},     // strict "1"/""
		{"ADMIN_EMAIL", "admin@portal.local", true},
		{"ADMIN_EMAIL", "nope", false},
		{"TRUSTED_PROXIES", "10.0.0.0/8, 192.168.0.0/16", true},
		{"UPSTREAM_ALLOW_PRIVATE", "0", false},            // strict "1"/""
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/settings/ -v`
Expected: FAIL — package does not exist / symbols undefined.

- [ ] **Step 3: Implement** — `internal/settings/registry.go`:

```go
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
)

type Type string

const (
	TypeString Type = "string"
	TypeBool   Type = "bool"
	TypePort   Type = "port"
	TypeURL    Type = "url"
	TypeEmail  Type = "email"
	TypeCSV    Type = "csv"
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
	{Key: "TRUSTED_PROXIES", Group: "portal", Type: TypeCSV, Editable: true},
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
	case TypeString, TypeCSV:
		// free-form
	}
	return nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -l internal/settings && go test ./internal/settings/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/settings/registry.go internal/settings/registry_test.go
git commit -m "feat(settings): declarative registry of all portal parameters"
```

---

### Task 3: Migration 0021 — override + audit tables

**Files:**
- Create: `internal/db/migrations/0021_portal_settings.sql`
- Test: `internal/db/migrate_portal_settings_test.go`

**Interfaces:**
- Produces: tables `portal_settings(key TEXT PK, value TEXT NOT NULL, updated_at, updated_by)` and `portal_settings_audit(id BIGSERIAL PK, key, old_value, new_value, admin_id, at)`. Consumed by Task 4's service.

- [ ] **Step 1: Write the failing test** — `internal/db/migrate_portal_settings_test.go` (connect-or-skip pattern of `migrate_teams_test.go`):

```go
package db

import (
	"context"
	"os"
	"testing"
)

func TestPortalSettingsMigration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Override row round-trip; PK upsert works.
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings(key, value) VALUES('TEST_KEY','v1')
		 ON CONFLICT (key) DO UPDATE SET value='v2', updated_at=now()`); err != nil {
		t.Fatalf("portal_settings upsert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings WHERE key='TEST_KEY'`) })
	var v string
	if err := pool.QueryRow(ctx, `SELECT value FROM portal_settings WHERE key='TEST_KEY'`).Scan(&v); err != nil || v != "v1" {
		t.Fatalf("value = %q err=%v, want v1 (first insert wins the test's single statement)", v, err)
	}
	// Audit table accepts a row with a NULL admin (deleted user).
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id) VALUES('TEST_KEY', NULL, '(secret)', NULL)`); err != nil {
		t.Fatalf("portal_settings_audit insert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings_audit WHERE key='TEST_KEY'`) })
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/db/ -run TestPortalSettingsMigration -v`
Expected: FAIL — relation "portal_settings" does not exist. (SKIP means Postgres isn't up: run `make up` first.)

- [ ] **Step 3: Create** `internal/db/migrations/0021_portal_settings.sql`:

```sql
-- Runtime-editable settings (spec 2026-07-13). One row per OVERRIDDEN key;
-- absence = env default; reset-to-env = DELETE. Secret values hold ciphertext
-- from the credential cipher, never plaintext.
CREATE TABLE IF NOT EXISTS portal_settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);

-- Audit trail: settings are portal-scoped, the app-scoped events table does
-- not fit; secrets are recorded as the literal string '(secret)'.
CREATE TABLE IF NOT EXISTS portal_settings_audit (
  id        BIGSERIAL PRIMARY KEY,
  key       TEXT NOT NULL,
  old_value TEXT,
  new_value TEXT,
  admin_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
  at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/db/ -run TestPortalSettingsMigration -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/0021_portal_settings.sql internal/db/migrate_portal_settings_test.go
git commit -m "feat(db): portal_settings override + audit tables"
```

---

### Task 4: settings.Service — snapshot, precedence, Set/Reset, hooks, audit

**Files:**
- Create: `internal/settings/service.go`
- Test: `internal/settings/service_test.go`

**Interfaces:**
- Consumes: Registry/Validate (Task 2), tables (Task 3), `crypto.Cipher` (`Encrypt(string)(string,error)`, `Decrypt(string)(string,error)`), `config.Config`.
- Produces (consumed by Tasks 5-10):

```go
// Effective is an immutable snapshot of the running configuration.
type Effective struct {
	Values map[string]string // key -> effective wire value (secrets decrypted)
	Source map[string]string // key -> "env" | "db"
}
func (e *Effective) Get(key string) string
func (e *Effective) Bool(key string) bool          // value == "1"
func (e *Effective) SMTPConfigured() bool           // SMTP_HOST != "" && SMTP_FROM != ""
func (e *Effective) SandboxConfigured() bool        // both sandbox URLs != ""

type Prober interface {
	Probe(ctx context.Context, candidate *Effective, touched map[string]bool) []ProbeResult
}
type ProbeResult struct {
	Name   string `json:"name"`   // "apisix" | "sandbox" | "smtp"
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func NewService(pool *pgxpool.Pool, cipher *crypto.Cipher, cfg config.Config, prober Prober) (*Service, error)
    // loads DB overrides; rows that fail to decrypt or match no registry key
    // are logged and skipped (env default applies)

func (s *Service) Snapshot() *Effective            // atomic load, lock-free
func (s *Service) OnChange(hook func(*Effective))   // called (serially) after every swap
func (s *Service) EnvDefault(key string) string     // the boot-time env value

// Set validates, enforces invariants, probes (unless force), persists all-or-
// nothing, swaps the snapshot, audits, runs hooks.
func (s *Service) Set(ctx context.Context, values map[string]string, adminID int64, force bool) error
func (s *Service) Reset(ctx context.Context, key string, adminID int64) error
func (s *Service) Test(ctx context.Context, values map[string]string) []ProbeResult

// Errors the handler maps to HTTP:
type FieldErrors map[string]string          // implements error; 422 {fields}
type ProbeError struct{ Results []ProbeResult } // implements error; 422 {probe}, forceable
var ErrUnknownKey  = errors.New("settings: unknown key")   // 400
var ErrReadOnlyKey = errors.New("settings: read-only key") // 400
```

- Invariant (in `Set`, evaluated on the candidate snapshot): `REQUIRE_EMAIL_VERIFICATION == "1"` and not SMTPConfigured ⇒ `FieldErrors{"REQUIRE_EMAIL_VERIFICATION": "requires SMTP_HOST and SMTP_FROM"}` — never bypassed by force.
- `EnvDefault` comes from a map captured at `NewService` from `cfg` (field-by-field mapping registry key → cfg field; write the explicit 24-entry mapping function `envValues(cfg config.Config) map[string]string` — booleans render as `"1"`/`""`).

- [ ] **Step 1: Write the failing tests** — `internal/settings/service_test.go`. Use the DB (connect-or-skip via a local `testPool` helper copied from `internal/auth/repo_test.go`'s pattern, including `db.Migrate`); build the cipher with `crypto.New(config.DevCredentialEncKey)`; use a stub prober:

```go
package settings

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
	"apisix-portal/internal/db"
)

type stubProber struct {
	results []ProbeResult
	calls   int
	mu      sync.Mutex
}

func (p *stubProber) Probe(_ context.Context, _ *Effective, _ map[string]bool) []ProbeResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.results
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	pool, err := db.Connect(context.Background(), url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func testService(t *testing.T, prober Prober) *Service {
	t.Helper()
	pool := testPool(t)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM portal_settings`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM portal_settings_audit`)
	})
	cipher, err := crypto.New(config.DevCredentialEncKey)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	t.Setenv("SMTP_HOST", "envhost")
	t.Setenv("SMTP_FROM", "env@portal.local")
	svc, err := NewService(pool, cipher, config.Load(), prober)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestPrecedenceAndReset(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()

	snap := svc.Snapshot()
	if snap.Get("SMTP_HOST") != "envhost" || snap.Source["SMTP_HOST"] != "env" {
		t.Fatalf("env default: got %q/%q", snap.Get("SMTP_HOST"), snap.Source["SMTP_HOST"])
	}
	if err := svc.Set(ctx, map[string]string{"SMTP_HOST": "dbhost"}, 1, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	snap = svc.Snapshot()
	if snap.Get("SMTP_HOST") != "dbhost" || snap.Source["SMTP_HOST"] != "db" {
		t.Fatalf("override: got %q/%q", snap.Get("SMTP_HOST"), snap.Source["SMTP_HOST"])
	}
	if svc.EnvDefault("SMTP_HOST") != "envhost" {
		t.Fatalf("EnvDefault = %q", svc.EnvDefault("SMTP_HOST"))
	}
	if err := svc.Reset(ctx, "SMTP_HOST", 1); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if snap = svc.Snapshot(); snap.Get("SMTP_HOST") != "envhost" || snap.Source["SMTP_HOST"] != "env" {
		t.Fatalf("after reset: got %q/%q", snap.Get("SMTP_HOST"), snap.Source["SMTP_HOST"])
	}
}

func TestSecretsEncryptedAtRest(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	if err := svc.Set(ctx, map[string]string{"SMTP_PASSWORD": "hunter2"}, 1, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var raw string
	pool := testPool(t)
	if err := pool.QueryRow(ctx, `SELECT value FROM portal_settings WHERE key='SMTP_PASSWORD'`).Scan(&raw); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if raw == "hunter2" {
		t.Fatal("secret stored in plaintext")
	}
	if svc.Snapshot().Get("SMTP_PASSWORD") != "hunter2" {
		t.Fatal("snapshot must expose the decrypted secret to internal consumers")
	}
}

func TestRejectUnknownReadOnlyAndInvalid(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	if err := svc.Set(ctx, map[string]string{"NOPE": "x"}, 1, false); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown: %v", err)
	}
	if err := svc.Set(ctx, map[string]string{"JWT_SECRET": "x"}, 1, false); !errors.Is(err, ErrReadOnlyKey) {
		t.Fatalf("read-only: %v", err)
	}
	var fe FieldErrors
	if err := svc.Set(ctx, map[string]string{"SMTP_PORT": "not-a-port"}, 1, false); !errors.As(err, &fe) {
		t.Fatalf("invalid type: %v", err)
	}
}

func TestVerificationInvariantNotForceable(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	// Turn SMTP off and verification on in ONE call: candidate evaluation.
	var fe FieldErrors
	err := svc.Set(ctx, map[string]string{
		"SMTP_HOST":                  "",
		"REQUIRE_EMAIL_VERIFICATION": "1",
	}, 1, true) // force=true must NOT bypass the invariant
	if !errors.As(err, &fe) {
		t.Fatalf("want FieldErrors, got %v", err)
	}
	if _, ok := fe["REQUIRE_EMAIL_VERIFICATION"]; !ok {
		t.Fatalf("invariant must name the flag, got %v", fe)
	}
}

func TestProbeFailureForceable(t *testing.T) {
	p := &stubProber{results: []ProbeResult{{Name: "smtp", OK: false, Detail: "connection refused"}}}
	svc := testService(t, p)
	ctx := context.Background()
	var pe *ProbeError
	if err := svc.Set(ctx, map[string]string{"SMTP_HOST": "bogus"}, 1, false); !errors.As(err, &pe) {
		t.Fatalf("want ProbeError, got %v", err)
	}
	if err := svc.Set(ctx, map[string]string{"SMTP_HOST": "bogus"}, 1, true); err != nil {
		t.Fatalf("force must bypass probe failure: %v", err)
	}
	if svc.Snapshot().Get("SMTP_HOST") != "bogus" {
		t.Fatal("forced value must apply")
	}
}

func TestHooksAndAudit(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	var mu sync.Mutex
	var hookSeen string
	svc.OnChange(func(e *Effective) { mu.Lock(); hookSeen = e.Get("SMTP_HOST"); mu.Unlock() })
	if err := svc.Set(ctx, map[string]string{"SMTP_HOST": "h2", "SMTP_PASSWORD": "s3cret"}, 42, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	mu.Lock()
	if hookSeen != "h2" {
		t.Fatalf("hook saw %q", hookSeen)
	}
	mu.Unlock()
	pool := testPool(t)
	rows, err := pool.Query(ctx, `SELECT key, COALESCE(new_value,'') FROM portal_settings_audit ORDER BY id`)
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var k, nv string
		_ = rows.Scan(&k, &nv)
		got[k] = nv
	}
	if got["SMTP_HOST"] != "h2" {
		t.Fatalf("audit SMTP_HOST new_value = %q", got["SMTP_HOST"])
	}
	if got["SMTP_PASSWORD"] != "(secret)" {
		t.Fatalf("secret audit must be redacted, got %q", got["SMTP_PASSWORD"])
	}
}

func TestUndecryptableRowFallsBackToEnv(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	// Plant a secret row that is NOT valid ciphertext.
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings(key, value) VALUES('SMTP_PASSWORD','not-ciphertext')
		 ON CONFLICT (key) DO UPDATE SET value='not-ciphertext'`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings WHERE key='SMTP_PASSWORD'`) })
	t.Setenv("SMTP_PASSWORD", "envpw")
	cipher, _ := crypto.New(config.DevCredentialEncKey)
	svc, err := NewService(pool, cipher, config.Load(), &stubProber{})
	if err != nil {
		t.Fatalf("NewService must tolerate bad rows: %v", err)
	}
	snap := svc.Snapshot()
	if snap.Get("SMTP_PASSWORD") != "envpw" || snap.Source["SMTP_PASSWORD"] != "env" {
		t.Fatalf("bad row must fall back to env: %q/%q", snap.Get("SMTP_PASSWORD"), snap.Source["SMTP_PASSWORD"])
	}
}

func TestConcurrentReadersDuringSwap(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = svc.Snapshot().Get("SMTP_HOST")
			_ = svc.Snapshot().SMTPConfigured()
		}
	}()
	for i := 0; i < 20; i++ {
		v := "h"
		if i%2 == 0 {
			v = "envhost"
		}
		if err := svc.Set(ctx, map[string]string{"SMTP_HOST": v}, 1, false); err != nil {
			t.Fatalf("Set #%d: %v", i, err)
		}
	}
	<-done
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/settings/ -run 'TestPrecedence|TestSecrets|TestReject|TestVerificationInvariant|TestProbeFailure|TestHooks|TestConcurrent' -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 3: Implement** `internal/settings/service.go`. Key parts (write the whole file; the skeleton below is the complete logic):

```go
package settings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
)

type Effective struct {
	Values map[string]string
	Source map[string]string
}

func (e *Effective) Get(key string) string { return e.Values[key] }
func (e *Effective) Bool(key string) bool  { return e.Values[key] == "1" }
func (e *Effective) SMTPConfigured() bool {
	return e.Get("SMTP_HOST") != "" && e.Get("SMTP_FROM") != ""
}
func (e *Effective) SandboxConfigured() bool {
	return e.Get("APISIX_SANDBOX_ADMIN_URL") != "" && e.Get("APISIX_SANDBOX_GATEWAY_URL") != ""
}

type ProbeResult struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type Prober interface {
	Probe(ctx context.Context, candidate *Effective, touched map[string]bool) []ProbeResult
}

type FieldErrors map[string]string

func (f FieldErrors) Error() string { return fmt.Sprintf("settings: %d invalid field(s)", len(f)) }

type ProbeError struct{ Results []ProbeResult }

func (p *ProbeError) Error() string { return "settings: probe failed" }

var (
	ErrUnknownKey  = errors.New("settings: unknown key")
	ErrReadOnlyKey = errors.New("settings: read-only key")
)

type Service struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
	prober Prober
	env    map[string]string // boot-time env values, immutable

	mu    sync.Mutex // serializes writers and hook runs
	snap  atomic.Pointer[Effective]
	hooks []func(*Effective)
}

// envValues maps every registry key to its boot-time Config value.
func envValues(cfg config.Config) map[string]string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return ""
	}
	return map[string]string{
		"PORTAL_ADDR":                cfg.Addr,
		"PORTAL_ENV":                 cfg.Env,
		"DATABASE_URL":               cfg.DatabaseURL,
		"JWT_SECRET":                 cfg.JWTSecret,
		"CREDENTIAL_ENC_KEY":         cfg.CredentialEncKey,
		"PORTAL_BASE_URL":            cfg.PortalBaseURL,
		"ADMIN_EMAIL":                cfg.AdminEmail,
		"TRUSTED_PROXIES":            cfg.TrustedProxies,
		"UPSTREAM_ALLOW_PRIVATE":     b(cfg.UpstreamAllowPrivate),
		"APISIX_ADMIN_URL":           cfg.APISIXAdminURL,
		"APISIX_GATEWAY_URL":         cfg.APISIXGatewayURL,
		"APISIX_ADMIN_KEY":           cfg.APISIXAdminKey,
		"APISIX_SANDBOX_ADMIN_URL":   cfg.APISIXSandboxAdminURL,
		"APISIX_SANDBOX_GATEWAY_URL": cfg.APISIXSandboxGatewayURL,
		"APISIX_SANDBOX_ADMIN_KEY":   cfg.APISIXSandboxAdminKey,
		"SMTP_HOST":                  cfg.SMTPHost,
		"SMTP_PORT":                  cfg.SMTPPort,
		"SMTP_USERNAME":              cfg.SMTPUsername,
		"SMTP_PASSWORD":              cfg.SMTPPassword,
		"SMTP_FROM":                  cfg.SMTPFrom,
		"REQUIRE_EMAIL_VERIFICATION": b(cfg.RequireEmailVerification),
		"OIDC_ISSUER":                cfg.OIDCIssuer,
		"OIDC_CLIENT_ID_CLAIM":       cfg.OIDCClientIDClaim,
		"PROMETHEUS_URL":             cfg.PrometheusURL,
	}
}

func NewService(pool *pgxpool.Pool, cipher *crypto.Cipher, cfg config.Config, prober Prober) (*Service, error) {
	s := &Service{pool: pool, cipher: cipher, prober: prober, env: envValues(cfg)}
	overrides, err := s.loadOverrides(context.Background())
	if err != nil {
		return nil, err
	}
	s.snap.Store(s.build(overrides))
	return s, nil
}

// loadOverrides reads portal_settings; unknown keys and undecryptable secrets
// are logged and skipped so a bad row can never prevent boot.
func (s *Service) loadOverrides(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM portal_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		d, ok := Lookup(k)
		if !ok {
			log.Printf("settings: ignoring unknown override %q", k)
			continue
		}
		if d.Secret {
			plain, err := s.cipher.Decrypt(v)
			if err != nil {
				log.Printf("settings: cannot decrypt %q, falling back to env default: %v", k, err)
				continue
			}
			v = plain
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Service) build(overrides map[string]string) *Effective {
	e := &Effective{Values: map[string]string{}, Source: map[string]string{}}
	for _, d := range Registry {
		if v, ok := overrides[d.Key]; ok {
			e.Values[d.Key], e.Source[d.Key] = v, "db"
		} else {
			e.Values[d.Key], e.Source[d.Key] = s.env[d.Key], "env"
		}
	}
	return e
}

func (s *Service) Snapshot() *Effective          { return s.snap.Load() }
func (s *Service) EnvDefault(key string) string  { return s.env[key] }
func (s *Service) OnChange(h func(*Effective)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, h)
}

// candidate returns the current snapshot with values overlaid (not persisted).
func (s *Service) candidate(values map[string]string) *Effective {
	cur := s.Snapshot()
	e := &Effective{Values: map[string]string{}, Source: map[string]string{}}
	for k, v := range cur.Values {
		e.Values[k], e.Source[k] = v, cur.Source[k]
	}
	for k, v := range values {
		e.Values[k], e.Source[k] = v, "db"
	}
	return e
}

func checkKeys(values map[string]string) error {
	fe := FieldErrors{}
	for k, v := range values {
		d, ok := Lookup(k)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownKey, k)
		}
		if !d.Editable {
			return fmt.Errorf("%w: %s", ErrReadOnlyKey, k)
		}
		if err := Validate(d, v); err != nil {
			fe[k] = err.Error()
		}
	}
	if len(fe) > 0 {
		return fe
	}
	return nil
}

func invariants(c *Effective) error {
	if c.Bool("REQUIRE_EMAIL_VERIFICATION") && !c.SMTPConfigured() {
		return FieldErrors{"REQUIRE_EMAIL_VERIFICATION": "requires SMTP_HOST and SMTP_FROM to be set"}
	}
	return nil
}

func (s *Service) Set(ctx context.Context, values map[string]string, adminID int64, force bool) error {
	if len(values) == 0 {
		return nil
	}
	if err := checkKeys(values); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cand := s.candidate(values)
	if err := invariants(cand); err != nil {
		return err // never forceable
	}
	touched := map[string]bool{}
	for k := range values {
		touched[k] = true
	}
	if !force && s.prober != nil {
		results := s.prober.Probe(ctx, cand, touched)
		for _, r := range results {
			if !r.OK {
				return &ProbeError{Results: results}
			}
		}
	}
	// Persist all-or-nothing, with audit rows in the same tx.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	old := s.Snapshot()
	for k, v := range values {
		d, _ := Lookup(k)
		stored := v
		if d.Secret {
			if stored, err = s.cipher.Encrypt(v); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO portal_settings(key, value, updated_by) VALUES($1,$2,$3)
			 ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=now(), updated_by=$3`,
			k, stored, adminID); err != nil {
			return err
		}
		oldV, newV := old.Get(k), v
		if d.Secret {
			oldV, newV = "(secret)", "(secret)"
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id) VALUES($1,$2,$3,$4)`,
			k, oldV, newV, adminID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.snap.Store(cand)
	for _, h := range s.hooks {
		h(cand)
	}
	return nil
}

func (s *Service) Reset(ctx context.Context, key string, adminID int64) error {
	d, ok := Lookup(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if !d.Editable {
		return fmt.Errorf("%w: %s", ErrReadOnlyKey, key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.Snapshot()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM portal_settings WHERE key=$1`, key); err != nil {
		return err
	}
	oldV, newV := old.Get(key), s.env[key]
	if d.Secret {
		oldV, newV = "(secret)", "(secret)"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id) VALUES($1,$2,$3,$4)`,
		key, oldV, newV, adminID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// Rebuild from the surviving DB rows (cheap; writers are rare).
	overrides, err := s.loadOverrides(ctx)
	if err != nil {
		return err
	}
	next := s.build(overrides)
	if err := invariants(next); err != nil {
		// Resetting must not create an invalid state (e.g. resetting SMTP_HOST
		// to an empty env default while verification is on).
		return err
	}
	s.snap.Store(next)
	for _, h := range s.hooks {
		h(next)
	}
	return nil
}

func (s *Service) Test(ctx context.Context, values map[string]string) []ProbeResult {
	if err := checkKeys(values); err != nil {
		return []ProbeResult{{Name: "validation", OK: false, Detail: err.Error()}}
	}
	touched := map[string]bool{}
	for k := range values {
		touched[k] = true
	}
	if s.prober == nil {
		return nil
	}
	return s.prober.Probe(ctx, s.candidate(values), touched)
}
```

NOTE for the implementer: `Reset`'s invariant check runs AFTER the DELETE commit in this skeleton — that is a bug if applied literally. Move the rebuild + `invariants(next)` check BEFORE `tx.Commit(ctx)` (compute `overrides` from the candidate: copy current DB overrides minus the key, or simply run `invariants` on `s.build(overridesWithoutKey)` computed before the tx). The test below pins the correct behavior.

Add this test to `service_test.go` (part of Step 1's file — include it from the start):

```go
func TestResetCannotBreakInvariant(t *testing.T) {
	svc := testService(t, &stubProber{})
	ctx := context.Background()
	// env has SMTP (from testService Setenv); enable verification, then
	// override SMTP_HOST in DB, then try to reset SMTP_FROM's env... simpler:
	// clear env SMTP_FROM so the DB override is the only thing keeping SMTP on.
	t.Setenv("SMTP_FROM", "")
	svc2 := func() *Service { // rebuild service with the new env
		pool := testPool(t)
		cipher, _ := crypto.New(config.DevCredentialEncKey)
		s, err := NewService(pool, cipher, config.Load(), &stubProber{})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		return s
	}()
	if err := svc2.Set(ctx, map[string]string{"SMTP_FROM": "db@x.io", "REQUIRE_EMAIL_VERIFICATION": "1"}, 1, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var fe FieldErrors
	if err := svc2.Reset(ctx, "SMTP_FROM", 1); !errors.As(err, &fe) {
		t.Fatalf("reset breaking the invariant must fail, got %v", err)
	}
	if !svc2.Snapshot().SMTPConfigured() {
		t.Fatal("failed reset must not have applied")
	}
	_ = svc // keep first service alive; separate rows cleaned by t.Cleanup
}
```

- [ ] **Step 4: Run to verify pass (with race detector)**

Run: `gofmt -l internal/settings && go test ./internal/settings/ -count=1 -race -v 2>&1 | tail -15`
Expected: PASS all (including `TestResetCannotBreakInvariant` — fix the skeleton's reset ordering as noted).

- [ ] **Step 5: Commit**

```bash
git add internal/settings/service.go internal/settings/service_test.go
git commit -m "feat(settings): snapshot service — precedence, secrets, invariants, hooks, audit"
```

---

### Task 5: Probes

**Files:**
- Create: `internal/settings/probe.go`
- Test: `internal/settings/probe_test.go`

**Interfaces:**
- Consumes: `Effective`, `ProbeResult`, `Prober` (Task 4).
- Produces: `NewProber() *LiveProber` implementing `Prober`. Group→probe mapping: keys of group `apisix` → probe "apisix"; group `sandbox` → probe "sandbox" (skipped, OK=true with detail "sandbox not configured", when candidate sandbox URLs are empty); group `smtp` → probe "smtp" (skipped-OK when candidate SMTP_HOST is empty). Other groups probe nothing.

- [ ] **Step 1: Write the failing tests** — `internal/settings/probe_test.go`:

```go
package settings

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func candidateWith(t *testing.T, values map[string]string) *Effective {
	t.Helper()
	e := &Effective{Values: map[string]string{}, Source: map[string]string{}}
	for _, d := range Registry {
		e.Values[d.Key] = ""
	}
	for k, v := range values {
		e.Values[k] = v
	}
	return e
}

func TestAPISIXProbe(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/apisix/admin/routes") {
			w.WriteHeader(404)
			return
		}
		gotKey = r.Header.Get("X-API-KEY")
		if gotKey != "goodkey" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewProber()
	res := p.Probe(context.Background(), candidateWith(t, map[string]string{
		"APISIX_ADMIN_URL": srv.URL, "APISIX_ADMIN_KEY": "goodkey",
	}), map[string]bool{"APISIX_ADMIN_URL": true})
	if len(res) != 1 || res[0].Name != "apisix" || !res[0].OK {
		t.Fatalf("good key: %+v", res)
	}
	res = p.Probe(context.Background(), candidateWith(t, map[string]string{
		"APISIX_ADMIN_URL": srv.URL, "APISIX_ADMIN_KEY": "badkey",
	}), map[string]bool{"APISIX_ADMIN_KEY": true})
	if len(res) != 1 || res[0].OK {
		t.Fatalf("bad key must fail: %+v", res)
	}
}

func TestSMTPProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("220 test ESMTP\r\n"))
				buf := make([]byte, 128)
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = c.Read(buf) // EHLO line
				_, _ = c.Write([]byte("250 ok\r\n"))
			}(c)
		}
	}()
	u, _ := url.Parse("http://" + ln.Addr().String())
	host, port := u.Hostname(), u.Port()

	p := NewProber()
	res := p.Probe(context.Background(), candidateWith(t, map[string]string{
		"SMTP_HOST": host, "SMTP_PORT": port,
	}), map[string]bool{"SMTP_HOST": true})
	if len(res) != 1 || res[0].Name != "smtp" || !res[0].OK {
		t.Fatalf("smtp ok: %+v", res)
	}
	res = p.Probe(context.Background(), candidateWith(t, map[string]string{
		"SMTP_HOST": "127.0.0.1", "SMTP_PORT": "1", // nothing listens on :1
	}), map[string]bool{"SMTP_HOST": true})
	if len(res) != 1 || res[0].OK {
		t.Fatalf("smtp refused must fail: %+v", res)
	}
}

func TestProbeSkipsUntouchedAndUnconfigured(t *testing.T) {
	p := NewProber()
	// Touching only PORTAL_BASE_URL probes nothing.
	res := p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"PORTAL_BASE_URL": true})
	if len(res) != 0 {
		t.Fatalf("no probes expected, got %+v", res)
	}
	// Touching sandbox keys while sandbox candidate is empty: skipped-OK.
	res = p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"APISIX_SANDBOX_ADMIN_URL": true})
	if len(res) != 1 || !res[0].OK || res[0].Name != "sandbox" {
		t.Fatalf("empty sandbox must be skipped-OK: %+v", res)
	}
	// Touching SMTP keys while SMTP_HOST empty: skipped-OK.
	res = p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"SMTP_FROM": true})
	if len(res) != 1 || !res[0].OK || res[0].Name != "smtp" {
		t.Fatalf("empty smtp must be skipped-OK: %+v", res)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/settings/ -run 'TestAPISIXProbe|TestSMTPProbe|TestProbeSkips' -v`
Expected: FAIL — `NewProber` undefined.

- [ ] **Step 3: Implement** `internal/settings/probe.go`:

```go
package settings

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const probeTimeout = 5 * time.Second

// LiveProber health-checks candidate settings before they are applied:
// APISIX admin APIs get a 1-item routes list, SMTP gets a dial + EHLO.
type LiveProber struct{ client *http.Client }

func NewProber() *LiveProber {
	return &LiveProber{client: &http.Client{Timeout: probeTimeout}}
}

func groupTouched(touched map[string]bool, group string) bool {
	for k := range touched {
		if d, ok := Lookup(k); ok && d.Group == group {
			return true
		}
	}
	return false
}

func (p *LiveProber) Probe(ctx context.Context, c *Effective, touched map[string]bool) []ProbeResult {
	var out []ProbeResult
	if groupTouched(touched, "apisix") {
		out = append(out, p.apisix(ctx, "apisix", c.Get("APISIX_ADMIN_URL"), c.Get("APISIX_ADMIN_KEY")))
	}
	if groupTouched(touched, "sandbox") {
		if !c.SandboxConfigured() {
			out = append(out, ProbeResult{Name: "sandbox", OK: true, Detail: "sandbox not configured — skipped"})
		} else {
			out = append(out, p.apisix(ctx, "sandbox", c.Get("APISIX_SANDBOX_ADMIN_URL"), c.Get("APISIX_SANDBOX_ADMIN_KEY")))
		}
	}
	if groupTouched(touched, "smtp") {
		if c.Get("SMTP_HOST") == "" {
			out = append(out, ProbeResult{Name: "smtp", OK: true, Detail: "SMTP not configured — skipped"})
		} else {
			out = append(out, p.smtp(ctx, c.Get("SMTP_HOST"), c.Get("SMTP_PORT")))
		}
	}
	return out
}

func (p *LiveProber) apisix(ctx context.Context, name, adminURL, key string) ProbeResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(adminURL, "/")+"/apisix/admin/routes?page_size=1", nil)
	if err != nil {
		return ProbeResult{Name: name, Detail: err.Error()}
	}
	req.Header.Set("X-API-KEY", key)
	resp, err := p.client.Do(req)
	if err != nil {
		return ProbeResult{Name: name, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProbeResult{Name: name, Detail: fmt.Sprintf("admin API answered HTTP %d", resp.StatusCode)}
	}
	return ProbeResult{Name: name, OK: true, Detail: "admin API reachable"}
}

func (p *LiveProber) smtp(ctx context.Context, host, port string) ProbeResult {
	if port == "" {
		port = "587"
	}
	d := net.Dialer{Timeout: probeTimeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return ProbeResult{Name: "smtp", Detail: err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(probeTimeout))
	r := bufio.NewReader(conn)
	greet, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(greet, "220") {
		return ProbeResult{Name: "smtp", Detail: fmt.Sprintf("no SMTP greeting (got %q, err %v)", strings.TrimSpace(greet), err)}
	}
	if _, err := fmt.Fprintf(conn, "EHLO portal\r\n"); err != nil {
		return ProbeResult{Name: "smtp", Detail: err.Error()}
	}
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "250") {
		return ProbeResult{Name: "smtp", Detail: fmt.Sprintf("EHLO rejected (got %q, err %v)", strings.TrimSpace(line), err)}
	}
	return ProbeResult{Name: "smtp", OK: true, Detail: "SMTP reachable"}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -l internal/settings && go test ./internal/settings/ -count=1 -v 2>&1 | tail -8`
Expected: PASS all settings tests.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/probe.go internal/settings/probe_test.go
git commit -m "feat(settings): APISIX + SMTP live probes"
```

---

### Task 6: apisix.SwappableGateway

**Files:**
- Create: `internal/apisix/swappable.go`
- Test: `internal/apisix/swappable_test.go`

**Interfaces:**
- Consumes: `apisix.Gateway` interface (5 methods: EnsureConsumer, DeleteConsumer, EnsureRoute, DeleteRoute, EnsureOAuthRoute — see `internal/apisix/gateway.go:13`), `apisix.NewClient(baseURL, apiKey string) *Client`.
- Produces (consumed by Task 9 wiring):

```go
// NewSwappable builds a gateway holder; inner may be nil (disabled).
func NewSwappable(inner Gateway) *SwappableGateway
func (s *SwappableGateway) Swap(inner Gateway)   // nil disables
func (s *SwappableGateway) Enabled() bool
// SwappableGateway implements Gateway by delegating to the current inner;
// when disabled, every method returns ErrGatewayDisabled.
var ErrGatewayDisabled = errors.New("apisix: gateway not configured")
```

- [ ] **Step 1: Write the failing test** — `internal/apisix/swappable_test.go` (check the package for an existing `Fake` gateway — `gateway.go` says one exists for tests; reuse it. If its recorded-calls API differs, adapt assertions to it):

```go
package apisix

import (
	"context"
	"errors"
	"testing"
)

func TestSwappableDelegatesAndSwaps(t *testing.T) {
	f1, f2 := NewFake(), NewFake()
	sw := NewSwappable(f1)
	if !sw.Enabled() {
		t.Fatal("non-nil inner must be enabled")
	}
	if err := sw.EnsureConsumer(context.Background(), "u", "k", RateLimit{}); err != nil {
		t.Fatalf("delegate: %v", err)
	}
	sw.Swap(f2)
	if err := sw.DeleteConsumer(context.Background(), "u"); err != nil {
		t.Fatalf("post-swap delegate: %v", err)
	}
	// f1 got the first call, f2 the second — adapt to Fake's recording API,
	// e.g. if Fake records consumers: f1 has "u", f2 does not have it anymore.
	_ = f1
	_ = f2
}

func TestSwappableDisabled(t *testing.T) {
	sw := NewSwappable(nil)
	if sw.Enabled() {
		t.Fatal("nil inner must be disabled")
	}
	if err := sw.EnsureRoute(context.Background(), "r", "/x", "u:80", nil); !errors.Is(err, ErrGatewayDisabled) {
		t.Fatalf("disabled call: %v", err)
	}
	sw.Swap(NewFake())
	if !sw.Enabled() {
		t.Fatal("swap-in must enable")
	}
}
```

(First read `internal/apisix/gateway.go` and the Fake's constructor/recording surface; if the fake is named differently (`Fake{}` literal, `NewFake()` absent), use the actual construction the package's other tests use and strengthen the delegation assertions with its real recording fields.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/apisix/ -run TestSwappable -v`
Expected: FAIL — `NewSwappable` undefined.

- [ ] **Step 3: Implement** `internal/apisix/swappable.go`:

```go
package apisix

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrGatewayDisabled is returned by a SwappableGateway with no inner client
// (e.g. sandbox URLs unset at runtime).
var ErrGatewayDisabled = errors.New("apisix: gateway not configured")

// SwappableGateway lets runtime settings replace the underlying Admin API
// client without rewiring consumers: it implements Gateway and delegates to
// the current inner, which Swap replaces atomically.
type SwappableGateway struct {
	inner atomic.Pointer[gatewayBox]
}

type gatewayBox struct{ gw Gateway } // box so a nil Gateway is representable

func NewSwappable(inner Gateway) *SwappableGateway {
	s := &SwappableGateway{}
	s.Swap(inner)
	return s
}

func (s *SwappableGateway) Swap(inner Gateway) { s.inner.Store(&gatewayBox{gw: inner}) }
func (s *SwappableGateway) Enabled() bool      { return s.inner.Load().gw != nil }

func (s *SwappableGateway) get() (Gateway, error) {
	if gw := s.inner.Load().gw; gw != nil {
		return gw, nil
	}
	return nil, ErrGatewayDisabled
}

func (s *SwappableGateway) EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureConsumer(ctx, username, apiKey, limit)
}

func (s *SwappableGateway) DeleteConsumer(ctx context.Context, username string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.DeleteConsumer(ctx, username)
}

func (s *SwappableGateway) EnsureRoute(ctx context.Context, routeID, contextPath, upstreamURL string, allowedConsumers []string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureRoute(ctx, routeID, contextPath, upstreamURL, allowedConsumers)
}

func (s *SwappableGateway) DeleteRoute(ctx context.Context, routeID string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.DeleteRoute(ctx, routeID)
}

func (s *SwappableGateway) EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureOAuthRoute(ctx, routeID, contextPath, upstreamURL, issuer, claimName, allowedClientIDs)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -l internal/apisix && go test ./internal/apisix/ -count=1 2>&1 | tail -3`
Expected: PASS (whole package — pre-existing tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/apisix/swappable.go internal/apisix/swappable_test.go
git commit -m "feat(apisix): SwappableGateway — runtime-replaceable admin client"
```

---

### Task 7: Dynamic SMTP sender + snapshot-driven notify/auth reads

**Files:**
- Create: `internal/notify/dynamic.go`
- Modify: `internal/auth/handler.go` (verification gate reads a provider)
- Test: `internal/notify/dynamic_test.go`, `internal/auth/handler_test.go` (adapt)

**Interfaces:**
- Consumes: `settings.Effective` getters (Task 4), existing `notify.Sender`, `notify.SMTPSender`, `auth.VerificationConfig`.
- Produces:

```go
// notify: a Sender that resolves SMTP parameters at send time.
type SettingsSource interface{ Snapshot() *settings.Effective }
func NewDynamicSender(src SettingsSource) *DynamicSender
func (d *DynamicSender) Send(ctx context.Context, to []string, subject, body string) error
// returns ErrSMTPNotConfigured when snapshot SMTP is off.
var ErrSMTPNotConfigured = errors.New("notify: SMTP not configured")

// auth: the one-shot EnableEmailVerification gains a dynamic sibling.
type VerificationProvider interface {
	// Enabled reports whether the gate is on RIGHT NOW; BaseURL and Sender are
	// resolved at the same moment for consistency.
	VerificationEnabled() bool
	VerificationBaseURL() string
}
func (h *Handler) EnableDynamicVerification(p VerificationProvider, sender notify.Sender, limiter *httpx.RateLimiter)
```

Behavior change in `auth`: `EnableDynamicVerification` mounts the verify/resend routes ONCE and stores the provider; `register`, `login`, `verifyEmail`, `resendVerification` check `p.VerificationEnabled()` per request. verify/resend answer 404 (`httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")` — add the i18n key `common.notFound` fr "introuvable" / en "not found") when disabled. The static `EnableEmailVerification(VerificationConfig)` remains (tests use it) and is reimplemented as a fixed-value provider wrapping the same code path — one implementation, two enablement styles.

- [ ] **Step 1: Write failing notify test** — `internal/notify/dynamic_test.go`:

```go
package notify

import (
	"context"
	"errors"
	"testing"

	"apisix-portal/internal/settings"
)

type stubSource struct{ e *settings.Effective }

func (s stubSource) Snapshot() *settings.Effective { return s.e }

func eff(values map[string]string) *settings.Effective {
	e := &settings.Effective{Values: map[string]string{}, Source: map[string]string{}}
	for k, v := range values {
		e.Values[k] = v
	}
	return e
}

func TestDynamicSenderUnconfigured(t *testing.T) {
	d := NewDynamicSender(stubSource{eff(nil)})
	err := d.Send(context.Background(), []string{"a@b.c"}, "s", "b")
	if !errors.Is(err, ErrSMTPNotConfigured) {
		t.Fatalf("want ErrSMTPNotConfigured, got %v", err)
	}
}

func TestDynamicSenderReadsSnapshotPerSend(t *testing.T) {
	src := stubSource{eff(map[string]string{
		"SMTP_HOST": "127.0.0.1", "SMTP_PORT": "1", "SMTP_FROM": "x@y.z",
	})}
	d := NewDynamicSender(src)
	// Nothing listens on :1 — the point is that it TRIED the snapshot values
	// (connection refused), not that it succeeded.
	err := d.Send(context.Background(), []string{"a@b.c"}, "s", "b")
	if err == nil || errors.Is(err, ErrSMTPNotConfigured) {
		t.Fatalf("want a dial error from snapshot values, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/notify/ -run TestDynamicSender -v`
Expected: FAIL — `NewDynamicSender` undefined.

- [ ] **Step 3: Implement** `internal/notify/dynamic.go`:

```go
package notify

import (
	"context"
	"errors"

	"apisix-portal/internal/settings"
)

// ErrSMTPNotConfigured is returned by DynamicSender when the current settings
// snapshot has no SMTP host/from.
var ErrSMTPNotConfigured = errors.New("notify: SMTP not configured")

// SettingsSource is the read surface DynamicSender needs (satisfied by
// *settings.Service).
type SettingsSource interface{ Snapshot() *settings.Effective }

// DynamicSender resolves SMTP parameters from the settings snapshot at each
// Send, so runtime settings changes apply to the next email with no rewiring.
type DynamicSender struct{ src SettingsSource }

func NewDynamicSender(src SettingsSource) *DynamicSender { return &DynamicSender{src: src} }

func (d *DynamicSender) Send(ctx context.Context, to []string, subject, body string) error {
	e := d.src.Snapshot()
	if !e.SMTPConfigured() {
		return ErrSMTPNotConfigured
	}
	s := NewSMTPSender(e.Get("SMTP_HOST"), e.Get("SMTP_PORT"), e.Get("SMTP_USERNAME"), e.Get("SMTP_PASSWORD"), e.Get("SMTP_FROM"))
	return s.Send(ctx, to, subject, body)
}
```

- [ ] **Step 4: Write failing auth test** — append to `internal/auth/handler_test.go` (reuse `memRepo`, `fakeVerifSender`, `postAuth` already there):

```go
type toggleProvider struct {
	on      bool
	baseURL string
}

func (p *toggleProvider) VerificationEnabled() bool   { return p.on }
func (p *toggleProvider) VerificationBaseURL() string { return p.baseURL }

func TestDynamicVerificationToggles(t *testing.T) {
	sender := &fakeVerifSender{}
	prov := &toggleProvider{on: false, baseURL: "http://localhost:8088"}
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	h.EnableDynamicVerification(prov, sender, nil)

	// OFF: routes answer 404; register auto-logins like the feature-off path.
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("verify while off = %d, want 404", rec.Code)
	}
	rec = postAuth(h, "/api/auth/register", credentials{Email: "a@x.io", Password: "longenough"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register off = %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == nil {
		t.Fatal("off: register must return a token")
	}

	// ON: register withholds token, sends mail; verify works.
	prov.on = true
	rec = postAuth(h, "/api/auth/register", credentials{Email: "b@x.io", Password: "longenough"})
	body = map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, has := body["token"]; has {
		t.Fatal("on: register must withhold the token")
	}
	sender.waitFor(t, 1)

	// Back OFF mid-flight: an unverified user can now log in (gate consults
	// the provider per request).
	prov.on = false
	rec = postAuth(h, "/api/auth/login", credentials{Email: "b@x.io", Password: "longenough"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login with gate re-disabled = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 5: Run to verify failure**

Run: `go test ./internal/auth/ -run TestDynamicVerification -v`
Expected: FAIL — `EnableDynamicVerification` undefined.

- [ ] **Step 6: Implement the auth changes** in `internal/auth/handler.go`:

Replace the `verify *VerificationConfig` field usage with a provider-based gate. Concretely:

```go
type VerificationProvider interface {
	VerificationEnabled() bool
	VerificationBaseURL() string
}

// staticProvider adapts the legacy VerificationConfig one-shot enablement.
type staticProvider struct{ baseURL string }

func (p staticProvider) VerificationEnabled() bool   { return true }
func (p staticProvider) VerificationBaseURL() string { return p.baseURL }
```

Handler struct: replace `verify *VerificationConfig` with:

```go
	verifyProv    VerificationProvider // nil = feature entirely absent
	verifySender  notify.Sender
	verifyLimiter *httpx.RateLimiter
	verifyTTL     time.Duration
	verifyGen     func() (plain, hash string)
```

`EnableEmailVerification(vc VerificationConfig)` becomes:

```go
func (h *Handler) EnableEmailVerification(vc VerificationConfig) {
	if vc.TokenTTL == 0 {
		vc.TokenTTL = 24 * time.Hour
	}
	if vc.GenToken == nil {
		vc.GenToken = GenerateVerifyToken
	}
	h.verifySender, h.verifyLimiter, h.verifyTTL, h.verifyGen = vc.Sender, vc.Limiter, vc.TokenTTL, vc.GenToken
	h.verifyProv = staticProvider{baseURL: vc.BaseURL}
	h.mountVerifyRoutes()
}

func (h *Handler) EnableDynamicVerification(p VerificationProvider, sender notify.Sender, limiter *httpx.RateLimiter) {
	h.verifySender, h.verifyLimiter, h.verifyTTL, h.verifyGen = sender, limiter, 24*time.Hour, GenerateVerifyToken
	h.verifyProv = p
	h.mountVerifyRoutes()
}

func (h *Handler) mountVerifyRoutes() {
	h.router.Post("/api/auth/verify", h.verifyEmail)
	h.router.Post("/api/auth/resend-verification", h.resendVerification)
}

func (h *Handler) verificationOn() bool { return h.verifyProv != nil && h.verifyProv.VerificationEnabled() }
```

Then change every `h.verify != nil` check to `h.verificationOn()`, every `h.verify.TokenTTL` to `h.verifyTTL`, `h.verify.GenToken` to `h.verifyGen`, `h.verify.Limiter` to `h.verifyLimiter`, `h.verify.BaseURL` to `h.verifyProv.VerificationBaseURL()` (read once per request, in the handler, before spawning the send goroutine), `h.verify.Sender` to `h.verifySender`. At the top of `verifyEmail` and `resendVerification` add:

```go
	if !h.verificationOn() {
		httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")
		return
	}
```

Add `"common.notFound"` to `internal/i18n/catalog_fr.go` (`"introuvable"`) and `catalog_en.go` (`"not found"`).

NOTE: `TestVerifyRoutesAbsentWhenDisabled` (existing) builds a handler with NO enablement — routes genuinely absent, still 404. It keeps passing unchanged.

- [ ] **Step 7: Run to verify pass**

Run: `gofmt -l internal/auth internal/notify internal/i18n && go build ./... && go test ./internal/auth/ ./internal/notify/ ./internal/i18n/ -count=1 2>&1 | tail -5`
Expected: PASS all (pre-existing verification tests keep passing via the static provider path).

- [ ] **Step 8: Commit**

```bash
git add internal/notify/dynamic.go internal/notify/dynamic_test.go internal/auth/handler.go internal/auth/handler_test.go internal/i18n/catalog_fr.go internal/i18n/catalog_en.go
git commit -m "feat(auth,notify): snapshot-driven verification gate + dynamic SMTP sender"
```

---

### Task 8: Settings HTTP API

**Files:**
- Create: `internal/settings/handler.go`
- Test: `internal/settings/handler_test.go`

**Interfaces:**
- Consumes: `*settings.Service` (Task 4 — Snapshot/EnvDefault/Set/Reset/Test), `Registry`, error types; `auth.UserID(ctx)` for the acting admin id (`internal/auth` middleware puts it in the context — check `internal/auth/middleware.go` for the exact helper name); `httpx.JSON`, `httpx.ErrorT`.
- Produces (consumed by Task 9 wiring and Task 11 frontend):

```go
func NewHandler(svc *Service) *Handler   // chi router, ServeHTTP
// GET    /api/admin/settings                  → []GroupView
// PUT    /api/admin/settings                  {values, force} → 204|400|422
// DELETE /api/admin/settings/{key}            → 204|400|404|422
// POST   /api/admin/settings/test             {values} → []ProbeResult

type ItemView struct {
	Key        string  `json:"key"`
	Type       string  `json:"type"`
	Editable   bool    `json:"editable"`
	Secret     bool    `json:"secret"`
	Value      *string `json:"value"`      // nil for secrets
	Set        bool    `json:"set"`        // secrets: non-empty?
	Source     string  `json:"source"`     // "env"|"db"
	EnvDefault *string `json:"envDefault"` // nil for secrets
}
type GroupView struct {
	Group string     `json:"group"`
	Items []ItemView `json:"items"`
}
```

- 422 bodies: `{"fields": {...}}` for FieldErrors, `{"probe": [...]}` for ProbeError.

- [ ] **Step 1: Write the failing tests** — `internal/settings/handler_test.go` (DB-backed via `testService`; wrap requests exactly like `internal/admin/handler_test.go`'s `do` helper; inject the admin id with the real context helper from `internal/auth` — read `internal/auth/middleware.go` first and use whatever `RequireAdmin` injects, e.g. `auth.WithUserID(ctx, 7)`; if no exported setter exists, add one next to `UserID` in that file as part of this task):

```go
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

func doReq(h *Handler, method, target string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	req = req.WithContext(auth.WithUserID(req.Context(), 7))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGetShapeAndSecretMasking(t *testing.T) {
	svc := testService(t, &stubProber{})
	if err := svc.Set(context.Background(), map[string]string{"SMTP_PASSWORD": "s3cret", "SMTP_HOST": "h1"}, 7, false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := NewHandler(svc)
	rec := doReq(h, http.MethodGet, "/api/admin/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatal("secret value leaked in GET")
	}
	var groups []GroupView
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := map[string]ItemView{}
	for _, g := range groups {
		for _, it := range g.Items {
			byKey[it.Key] = it
		}
	}
	if len(byKey) != 24 {
		t.Fatalf("items = %d, want 24", len(byKey))
	}
	pw := byKey["SMTP_PASSWORD"]
	if pw.Value != nil || !pw.Set || pw.Source != "db" || pw.EnvDefault != nil {
		t.Fatalf("secret view wrong: %+v", pw)
	}
	host := byKey["SMTP_HOST"]
	if host.Value == nil || *host.Value != "h1" || host.Source != "db" || host.EnvDefault == nil || *host.EnvDefault != "envhost" {
		t.Fatalf("host view wrong: %+v", host)
	}
	jwt := byKey["JWT_SECRET"]
	if jwt.Editable || jwt.Value != nil {
		t.Fatalf("boot-critical secret must be read-only + valueless: %+v", jwt)
	}
}

func TestPutMatrix(t *testing.T) {
	svc := testService(t, &stubProber{})
	h := NewHandler(svc)
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "new"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("valid PUT = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"NOPE": "x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown = %d", rec.Code)
	}
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"JWT_SECRET": "x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("read-only = %d", rec.Code)
	}
	rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_PORT": "nope"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"fields"`) {
		t.Fatalf("invalid field = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPutProbeFailureAndForce(t *testing.T) {
	p := &stubProber{results: []ProbeResult{{Name: "smtp", OK: false, Detail: "refused"}}}
	svc := testService(t, p)
	h := NewHandler(svc)
	rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "bogus"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"probe"`) {
		t.Fatalf("probe fail = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "bogus"}, "force": true})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("forced = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteReset(t *testing.T) {
	svc := testService(t, &stubProber{})
	h := NewHandler(svc)
	_ = svc.Set(context.Background(), map[string]string{"SMTP_HOST": "x"}, 7, false)
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/SMTP_HOST", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("reset = %d", rec.Code)
	}
	if svc.Snapshot().Source["SMTP_HOST"] != "env" {
		t.Fatal("reset must fall back to env")
	}
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/NOPE", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reset = %d", rec.Code)
	}
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/JWT_SECRET", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("read-only reset = %d", rec.Code)
	}
}

func TestTestEndpoint(t *testing.T) {
	p := &stubProber{results: []ProbeResult{{Name: "smtp", OK: true, Detail: "ok"}}}
	svc := testService(t, p)
	h := NewHandler(svc)
	rec := doReq(h, http.MethodPost, "/api/admin/settings/test", map[string]any{"values": map[string]string{"SMTP_HOST": "candidate"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"smtp"`) {
		t.Fatalf("test = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.Snapshot().Get("SMTP_HOST") == "candidate" {
		t.Fatal("test must not persist")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/settings/ -run 'TestGetShape|TestPutMatrix|TestPutProbe|TestDeleteReset|TestTestEndpoint' -v`
Expected: FAIL — `NewHandler` undefined (or `auth.WithUserID` missing — add it in `internal/auth/middleware.go` next to `UserID`, mirroring how the middleware injects the id).

- [ ] **Step 3: Implement** `internal/settings/handler.go`:

```go
package settings

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

type ItemView struct {
	Key        string  `json:"key"`
	Type       string  `json:"type"`
	Editable   bool    `json:"editable"`
	Secret     bool    `json:"secret"`
	Value      *string `json:"value"`
	Set        bool    `json:"set"`
	Source     string  `json:"source"`
	EnvDefault *string `json:"envDefault"`
}

type GroupView struct {
	Group string     `json:"group"`
	Items []ItemView `json:"items"`
}

type Handler struct {
	svc    *Service
	router chi.Router
}

func NewHandler(svc *Service) *Handler {
	h := &Handler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/settings", h.list)
	h.router.Put("/api/admin/settings", h.put)
	h.router.Post("/api/admin/settings/test", h.test)
	h.router.Delete("/api/admin/settings/{key}", h.reset)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	e := h.svc.Snapshot()
	var groups []GroupView
	idx := map[string]int{}
	for _, d := range Registry {
		gi, ok := idx[d.Group]
		if !ok {
			groups = append(groups, GroupView{Group: d.Group})
			gi = len(groups) - 1
			idx[d.Group] = gi
		}
		it := ItemView{
			Key: d.Key, Type: string(d.Type), Editable: d.Editable, Secret: d.Secret,
			Set: e.Get(d.Key) != "", Source: e.Source[d.Key],
		}
		if !d.Secret {
			v := e.Get(d.Key)
			it.Value = &v
			def := h.svc.EnvDefault(d.Key)
			it.EnvDefault = &def
		}
		groups[gi].Items = append(groups[gi].Items, it)
	}
	httpx.JSON(w, http.StatusOK, groups)
}

type putBody struct {
	Values map[string]string `json:"values"`
	Force  bool              `json:"force"`
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Values) == 0 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	err := h.svc.Set(r.Context(), body.Values, auth.UserID(r.Context()), body.Force)
	h.respond(w, r, err)
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if _, ok := Lookup(key); !ok {
		httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")
		return
	}
	err := h.svc.Reset(r.Context(), key, auth.UserID(r.Context()))
	h.respond(w, r, err)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, err error) {
	var fe FieldErrors
	var pe *ProbeError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrUnknownKey), errors.Is(err, ErrReadOnlyKey):
		httpx.ErrorT(w, r, http.StatusBadRequest, "settings.badKey")
	case errors.As(err, &fe):
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"fields": fe})
	case errors.As(err, &pe):
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"probe": pe.Results})
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "settings.saveFailed")
	}
}

func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	httpx.JSON(w, http.StatusOK, h.svc.Test(r.Context(), body.Values))
}
```

Add i18n keys to both backend catalogs: `settings.badKey` (fr `"paramètre inconnu ou en lecture seule"`, en `"unknown or read-only setting"`), `settings.saveFailed` (fr `"échec de l'enregistrement du paramètre"`, en `"failed to save setting"`).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -l internal/settings internal/auth internal/i18n && go test ./internal/settings/ ./internal/i18n/ -count=1 2>&1 | tail -4`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/handler.go internal/settings/handler_test.go internal/auth/middleware.go internal/i18n/catalog_fr.go internal/i18n/catalog_en.go
git commit -m "feat(settings): admin API — list, save (probe+force), reset, test"
```

---

### Task 9: Server rewiring — everything reads the snapshot

**Files:**
- Modify: `internal/server/server.go` (the heart of the task)
- Modify: `internal/subscriptions/service.go` (dynamic sandbox-enabled + OIDC), `internal/subscriptions/handler.go` (dynamic sandbox gateway URL), `internal/tryit/handler.go` (dynamic gateway URLs), `internal/notify/notifier.go` (dynamic base URL)
- Modify: `cmd/portal/main.go` (no change expected — verify only)
- Test: none new here beyond compilation + the full suite; behavior is covered by Tasks 4-8 units and Task 12's live pass. (This task is wiring; a reviewer verifies each consumer switched.)

**Interfaces:**
- Consumes: everything above.
- Produces: `server.New` builds `settings.NewService(pool, cipher, cfg, settings.NewProber())` and:
  1. `prodGW := apisix.NewSwappable(apisix.NewClient(snap.Get("APISIX_ADMIN_URL"), snap.Get("APISIX_ADMIN_KEY")))`; sandbox likewise but `nil` inner when `!snap.SandboxConfigured()`. `subSvc` receives the two swappables.
  2. `settingsSvc.OnChange(hook)` rebuilds both inner clients on every swap (cheap; unconditional rebuild avoids diffing) and re-arms `subSvc.ConfigureOIDC(e.Get("OIDC_ISSUER"), e.Get("OIDC_CLIENT_ID_CLAIM"))`.
  3. Subscriptions: `sandboxEnabled()` consults a new injected `func() bool` — add `func (s *Service) SetSandboxEnabledFn(fn func() bool)` to `internal/subscriptions/service.go` and change `sandboxEnabled()` to `if s.sandboxOn != nil { return s.sandboxOn() }; return s.sandboxGW != nil` (existing tests unaffected — they don't call the setter).
  4. Dynamic strings become getters: `subscriptions.NewHandler`'s `sandboxGatewayURL string` parameter and `tryit.NewHandler`'s two URL parameters change to `func() string` (update ALL constructor call sites incl. tests in those packages — grep `NewHandler(` in both); `notify.NewNotifier`'s `baseURL string` likewise becomes `func() string`. Inside, every former use of the string calls the getter.
  5. SMTP: notifier + `EnableDynamicVerification` get the shared `notify.NewDynamicSender(settingsSvc)`. The notifier is now set UNCONDITIONALLY (the dynamic sender no-ops with `ErrSMTPNotConfigured` when off — notify already logs-and-drops errors). Verification gate: `authH.EnableDynamicVerification(settingsProvider{settingsSvc}, dynSender, httpx.NewRateLimiter(3, 1.0/60))` where `settingsProvider` adapts `Snapshot()` → `VerificationEnabled() bool` (`Bool("REQUIRE_EMAIL_VERIFICATION")`) and `VerificationBaseURL()` (`Get("PORTAL_BASE_URL")`). The old `if cfg.SMTPConfigured()` block disappears.
  6. `ADMIN_EMAIL` hook: in `OnChange`, if `e.Get("ADMIN_EMAIL")` differs from the previous snapshot's, call `authRepo.EnsureAdminRole(ctx, e.Get("ADMIN_EMAIL"))` (background context, log error).
  7. Prometheus: `subH.SetUsageReader` is re-armed in the hook with a client built from `e.Get("PROMETHEUS_URL")` (check `SetUsageReader`'s concurrency: it's called at boot today; make the field it sets an atomic/mutex-guarded swap in `internal/subscriptions/handler.go` if it's a plain field — inspect and adjust).
  8. Trusted proxies: `ipLimiter.SetTrustedProxies` re-armed in the hook (parse errors: log and keep previous).
  9. Admin products handler `allowPrivate`: change `admin.NewHandler`'s `allowPrivate bool` to `func() bool` (update call sites + tests — mechanical `func() bool { return true }` in tests); wire `func() bool { return settingsSvc.Snapshot().Bool("UPSTREAM_ALLOW_PRIVATE") }`. Same for `oidcConfigured` → `func() bool { return settingsSvc.Snapshot().Get("OIDC_ISSUER") != "" }` and `sandboxConfigured` (the Task-session `/api/admin/meta` flags become live).
  10. Mount `settings.NewHandler(settingsSvc)` at `mux.Handle("/api/admin/settings", requireAdmin(settingsH))` and `mux.Handle("/api/admin/settings/", requireAdmin(settingsH))`.

- [ ] **Step 1: Make the constructor-signature changes** (subscriptions handler, tryit, notifier, admin handler) each as a small mechanical edit with its package's tests updated to pass constant getters (`func() string { return "x" }`). Run per-package tests as you go:

Run: `go test ./internal/subscriptions/ ./internal/tryit/ ./internal/notify/ ./internal/admin/ -count=1`
Expected: PASS after each package's call sites are updated.

- [ ] **Step 2: Rewire `server.New`** per the Produces list above. The resulting shape (abridged to the settings-relevant lines — integrate with the existing code, do not delete unrelated wiring):

```go
	settingsSvc, err := settings.NewService(pool, cipher, cfg, settings.NewProber())
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	snap := settingsSvc.Snapshot()

	newProd := func(e *settings.Effective) apisix.Gateway {
		return apisix.NewClient(e.Get("APISIX_ADMIN_URL"), e.Get("APISIX_ADMIN_KEY"))
	}
	newSandbox := func(e *settings.Effective) apisix.Gateway {
		if !e.SandboxConfigured() {
			return nil
		}
		return apisix.NewClient(e.Get("APISIX_SANDBOX_ADMIN_URL"), e.Get("APISIX_SANDBOX_ADMIN_KEY"))
	}
	prodGW := apisix.NewSwappable(newProd(snap))
	sandboxGW := apisix.NewSwappable(newSandbox(snap))

	subSvc := subscriptions.NewService(subRepo, prodGW, sandboxGW, subscriptions.GenerateKey, eventRepo)
	subSvc.SetSandboxEnabledFn(func() bool { return settingsSvc.Snapshot().SandboxConfigured() })
	subSvc.ConfigureOIDC(snap.Get("OIDC_ISSUER"), snap.Get("OIDC_CLIENT_ID_CLAIM"))

	dynSender := notify.NewDynamicSender(settingsSvc)
	subSvc.SetNotifier(notify.NewNotifier(dynSender, notify.NewRepo(pool),
		func() string { return settingsSvc.Snapshot().Get("PORTAL_BASE_URL") }))
	authH.EnableDynamicVerification(verificationFromSettings{settingsSvc}, dynSender, httpx.NewRateLimiter(3, 1.0/60))

	prevAdminEmail := snap.Get("ADMIN_EMAIL")
	settingsSvc.OnChange(func(e *settings.Effective) {
		prodGW.Swap(newProd(e))
		sandboxGW.Swap(newSandbox(e))
		subSvc.ConfigureOIDC(e.Get("OIDC_ISSUER"), e.Get("OIDC_CLIENT_ID_CLAIM"))
		if pu := e.Get("PROMETHEUS_URL"); pu != "" {
			subH.SetUsageReader(metrics.NewService(metrics.NewClient(pu)))
		}
		if proxies, err := httpx.ParseProxyCIDRs(e.Get("TRUSTED_PROXIES")); err != nil {
			log.Printf("settings: TRUSTED_PROXIES invalid, keeping previous: %v", err)
		} else {
			ipLimiter.SetTrustedProxies(proxies)
		}
		if ae := e.Get("ADMIN_EMAIL"); ae != prevAdminEmail {
			prevAdminEmail = ae
			if err := authRepo.EnsureAdminRole(context.Background(), ae); err != nil {
				log.Printf("settings: promote %q: %v", ae, err)
			}
		}
	})
```

with the adapter:

```go
type verificationFromSettings struct{ svc *settings.Service }

func (v verificationFromSettings) VerificationEnabled() bool {
	return v.svc.Snapshot().Bool("REQUIRE_EMAIL_VERIFICATION")
}
func (v verificationFromSettings) VerificationBaseURL() string {
	return v.svc.Snapshot().Get("PORTAL_BASE_URL")
}
```

`cipher` already exists in `server.New` (built for `subscriptions.NewRepo`) — build it BEFORE the settings service and reuse the variable. `ConfigureOIDC` is called from the hook: check it for a data race (it writes two plain string fields read by provisioning paths) — make those two fields guarded (store both in one `atomic.Pointer[oidcConf]` struct) as part of the `subscriptions/service.go` edit; run that package's tests with `-race`.

- [ ] **Step 3: Full sweep**

Run: `gofmt -l internal/ && go build ./... && go test ./... 2>&1 | grep -v '^ok\|no test files'`
Expected: no gofmt output, empty test-failure output.

Also confirm `cfg.Validate()` in `main.go` still guards the boot combination (it reads env, which remains the seed — nothing to change; just verify the file was untouched).

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go internal/subscriptions/ internal/tryit/ internal/notify/ internal/admin/
git commit -m "feat(server): all service bindings read the live settings snapshot"
```

---

### Task 10: Web API client + types

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/api/client.ts`
- Test: `web/src/api/settings.test.ts` (new)

**Interfaces:**
- Produces (consumed by Tasks 11-12):

```ts
// types.ts
export interface SettingItem {
  key: string; type: string; editable: boolean; secret: boolean
  value: string | null; set: boolean; source: 'env' | 'db'; envDefault: string | null
}
export interface SettingsGroup { group: string; items: SettingItem[] }
export interface ProbeResult { name: string; ok: boolean; detail: string }
// client.ts
export async function adminGetSettings(token: string): Promise<SettingsGroup[]>
export async function adminPutSettings(token: string, values: Record<string, string>, force?: boolean): Promise<void>
   // throws SettingsSaveError on 422 carrying {fields?} / {probe?}
export async function adminResetSetting(token: string, key: string): Promise<void>
export async function adminTestSettings(token: string, values: Record<string, string>): Promise<ProbeResult[]>
export class SettingsSaveError extends ApiError {
  fields?: Record<string, string>
  probe?: ProbeResult[]
}
```

- [ ] **Step 1: Write the failing test** — `web/src/api/settings.test.ts` (mock `fetch` like any existing client-level test; check `web/src/api` for an existing fetch-mocking test to copy the idiom — if none exists, use `vi.stubGlobal('fetch', ...)`):

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { adminPutSettings, SettingsSaveError } from './client'

beforeEach(() => { localStorage.setItem('lang', 'fr') })
afterEach(() => { vi.unstubAllGlobals() })

describe('adminPutSettings', () => {
  it('resolves on 204', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    await expect(adminPutSettings('jwt', { SMTP_HOST: 'x' })).resolves.toBeUndefined()
  })

  it('throws SettingsSaveError carrying field errors on 422', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ fields: { SMTP_PORT: 'must be a port between 1 and 65535' } }), { status: 422 })))
    const err = await adminPutSettings('jwt', { SMTP_PORT: 'x' }).catch(e => e)
    expect(err).toBeInstanceOf(SettingsSaveError)
    expect(err.fields.SMTP_PORT).toMatch(/port/)
  })

  it('throws SettingsSaveError carrying probe results on 422', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ probe: [{ name: 'smtp', ok: false, detail: 'refused' }] }), { status: 422 })))
    const err = await adminPutSettings('jwt', { SMTP_HOST: 'bogus' }).catch(e => e)
    expect(err).toBeInstanceOf(SettingsSaveError)
    expect(err.probe[0].name).toBe('smtp')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/api/settings.test.ts`
Expected: FAIL — exports missing.

- [ ] **Step 3: Implement** — add the types above to `types.ts`; in `client.ts`:

```ts
export class SettingsSaveError extends ApiError {
  fields?: Record<string, string>
  probe?: ProbeResult[]
  constructor(msg: string, status: number, fields?: Record<string, string>, probe?: ProbeResult[]) {
    super(msg, status)
    this.fields = fields
    this.probe = probe
  }
}

export async function adminGetSettings(token: string): Promise<SettingsGroup[]> {
  return parse<SettingsGroup[]>(await fetch('/api/admin/settings', { headers: langHeaders(token) }), '/api/admin/settings')
}

export async function adminPutSettings(token: string, values: Record<string, string>, force = false): Promise<void> {
  const res = await fetch('/api/admin/settings', {
    method: 'PUT', headers: langHeaders(token), body: JSON.stringify({ values, force }),
  })
  if (res.status === 422) {
    const body = await res.json().catch(() => ({}))
    throw new SettingsSaveError('settings save failed', 422, body.fields, body.probe)
  }
  await parse<unknown>(res, '/api/admin/settings')
}

export async function adminResetSetting(token: string, key: string): Promise<void> {
  await parse<unknown>(await fetch(`/api/admin/settings/${encodeURIComponent(key)}`, {
    method: 'DELETE', headers: langHeaders(token),
  }), '/api/admin/settings')
}

export async function adminTestSettings(token: string, values: Record<string, string>): Promise<ProbeResult[]> {
  return parse<ProbeResult[]>(await fetch('/api/admin/settings/test', {
    method: 'POST', headers: langHeaders(token), body: JSON.stringify({ values }),
  }), '/api/admin/settings/test')
}
```

(Import the new types at the top of `client.ts`.)

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx tsc --noEmit && npx vitest run src/api/settings.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/settings.test.ts
git commit -m "feat(web): settings API client (get/put/reset/test, typed 422s)"
```

---

### Task 11: SettingsPage UI

**Files:**
- Create: `web/src/pages/admin/SettingsPage.tsx`, `web/src/styles/admin-settings.css`
- Modify: `web/src/pages/admin/AdminShell.tsx` (new tab), `web/src/App.tsx` (route), `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`
- Test: `web/src/pages/admin/SettingsPage.test.tsx`

**Interfaces:**
- Consumes: Task 10 client functions + types; `AdminShell` (`active` prop union gains `'settings'` — check its props type and the subnav in `AdminShell.tsx:55-60`).
- Produces: route `/admin/settings` (behind `AdminGuard` like siblings in `App.tsx:28-31`); subnav link labeled from `admin.settingsNavLabel`.

**Component behavior (the spec's UI section, made concrete):**
- On mount: `adminGetSettings`; render one card per group, headed by `t('settings.group.<group>')`.
- Row per item: label = the raw key (mono) + `t('settings.desc.<KEY>')` one-liner; control per type: `bool` → checkbox (checked ⇔ draft value `"1"`), everything else → text input (secret: `type="password"`, placeholder `••••• (défini)` when `set`, empty otherwise; a secret's input only sends when non-empty — typing replaces).
- Badge: `source === 'db'` → `modifié` (accent), else `env` (muted). Reset button (↺) only when `source === 'db'`; confirms via the existing `ConfirmModal` pattern or `window.confirm` (match what ProductsPage uses — it has `ConfirmModal`; reuse it), then `adminResetSetting` + reload.
- Read-only group (`server`): inputs disabled, lock hint `t('settings.readOnlyHint')`.
- Dirty tracking: `draft: Record<string,string>` of changed keys only; sticky bar appears when non-empty with `Tester` → `adminTestSettings(draft)` rendering inline `ProbeResult` chips, `Enregistrer` → `adminPutSettings(draft)`; on `SettingsSaveError.fields` → per-row error text; on `.probe` → probe chips + a `Enregistrer quand même` button that re-calls with `force=true`; on success → toast (`useToast` like ProductsPage) + reload + clear draft.
- i18n keys (add to BOTH catalogs; French shown, English analogous): `admin.settingsNavLabel: 'Paramètres'`, `settings.title: 'Paramètres'`, `settings.desc: 'Configuration du portail — les modifications s'appliquent immédiatement.'`, `settings.group.server/portal/apisix/sandbox/smtp/policy/oidc/observability`, `settings.desc.<KEY>` for all 24 keys (one short sentence each — write them, e.g. `settings.desc.SMTP_HOST: 'Serveur SMTP pour les e-mails'`), `settings.badgeEnv: 'env'`, `settings.badgeDb: 'modifié'`, `settings.reset: 'Rétablir la valeur env'`, `settings.resetConfirm: 'Rétablir {key} à sa valeur d'environnement ({value}) ?'`, `settings.secretSet: '••••• (défini)'`, `settings.readOnlyHint: 'Défini au démarrage — non modifiable ici'`, `settings.test: 'Tester'`, `settings.save: 'Enregistrer'`, `settings.saveForce: 'Enregistrer quand même'`, `settings.saved: 'Paramètres enregistrés'`, `settings.loadError: 'Impossible de charger les paramètres.'`.
- CSS `web/src/styles/admin-settings.css` scoped under `.adminpage .settings`: group cards reuse the `.rows` card look (`background:var(--surface);border:1px solid var(--border);border-radius:var(--r);box-shadow:var(--shadow)`); rows are a 3-column grid (label / control / badge+reset); sticky save bar `position:sticky;bottom:0` on `var(--surface)` with top border; probe chips reuse the billing pill vocabulary (`.ok` success colors, `.ko` danger colors). Import the css from `SettingsPage.tsx`.

- [ ] **Step 1: Write the failing tests** — `web/src/pages/admin/SettingsPage.test.tsx` (mirror `ProductsPage.test.tsx` setup: `MemoryRouter` + providers + admin localStorage + `vi.spyOn(api, ...)`):

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import { SettingsSaveError } from '../../api/client'
import type { SettingsGroup } from '../../api/types'

const groups: SettingsGroup[] = [
  { group: 'server', items: [
    { key: 'JWT_SECRET', type: 'string', editable: false, secret: true, value: null, set: true, source: 'env', envDefault: null },
  ]},
  { group: 'smtp', items: [
    { key: 'SMTP_HOST', type: 'string', editable: true, secret: false, value: 'mailpit', set: true, source: 'db', envDefault: 'envhost' },
    { key: 'SMTP_PASSWORD', type: 'string', editable: true, secret: true, value: null, set: false, source: 'env', envDefault: null },
  ]},
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetSettings').mockResolvedValue(groups)
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

const renderPage = () => render(
  <MemoryRouter><LanguageProvider><AuthProvider><SettingsPage /></AuthProvider></LanguageProvider></MemoryRouter>
)

describe('SettingsPage', () => {
  it('renders groups, values, source badges, and masks secrets', async () => {
    renderPage()
    expect(await screen.findByDisplayValue('mailpit')).toBeInTheDocument()
    expect(screen.getByText('modifié')).toBeInTheDocument()
    expect(screen.queryByDisplayValue(/secret/i)).not.toBeInTheDocument()
    const jwt = screen.getByLabelText('JWT_SECRET') as HTMLInputElement
    expect(jwt.disabled).toBe(true)
  })

  it('saves the dirty draft and reloads', async () => {
    const put = vi.spyOn(api, 'adminPutSettings').mockResolvedValue(undefined)
    renderPage()
    const host = await screen.findByLabelText('SMTP_HOST')
    await userEvent.clear(host)
    await userEvent.type(host, 'newhost')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer' }))
    await waitFor(() => expect(put).toHaveBeenCalledWith('jwt', { SMTP_HOST: 'newhost' }, false))
  })

  it('offers force-save when the probe fails', async () => {
    const put = vi.spyOn(api, 'adminPutSettings')
      .mockRejectedValueOnce(new SettingsSaveError('x', 422, undefined, [{ name: 'smtp', ok: false, detail: 'refused' }]))
      .mockResolvedValueOnce(undefined)
    renderPage()
    const host = await screen.findByLabelText('SMTP_HOST')
    await userEvent.clear(host)
    await userEvent.type(host, 'bogus')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(await screen.findByText(/refused/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer quand même' }))
    await waitFor(() => expect(put).toHaveBeenLastCalledWith('jwt', { SMTP_HOST: 'bogus' }, true))
  })

  it('resets an overridden key after confirmation', async () => {
    const reset = vi.spyOn(api, 'adminResetSetting').mockResolvedValue(undefined)
    renderPage()
    await screen.findByDisplayValue('mailpit')
    await userEvent.click(screen.getByRole('button', { name: /Rétablir/ }))
    // ConfirmModal path: click its confirm button (adapt to the modal's real cta label)
    await userEvent.click(await screen.findByRole('button', { name: /confirmer|rétablir/i }))
    await waitFor(() => expect(reset).toHaveBeenCalledWith('jwt', 'SMTP_HOST'))
  })
})
```

(Selectors bind inputs to keys via `aria-label={item.key}` — build the component that way. Adapt the ConfirmModal confirm-button query to the component's real API after reading `web/src/components/ConfirmModal.tsx`.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/admin/SettingsPage.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the page** per the behavior block above (single component ~200 lines; sub-pieces: `SettingRow`, `ProbeChips`, `SaveBar` as local functions). Add the AdminShell tab:

```tsx
<Link className={active === 'settings' ? 'active' : ''} to="/admin/settings">{t('admin.settingsNavLabel')}</Link>
```

(and widen the `active` prop type), the `App.tsx` route:

```tsx
<Route path="/admin/settings" element={<AdminGuard><SettingsPage /></AdminGuard>} />
```

all i18n keys in both catalogs, and the scoped CSS file.

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx tsc --noEmit && npx vitest run src/pages/admin/ src/api/`
Expected: PASS (new tests + all existing admin page tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/admin/SettingsPage.tsx web/src/pages/admin/SettingsPage.test.tsx web/src/pages/admin/AdminShell.tsx web/src/App.tsx web/src/styles/admin-settings.css web/src/i18n/fr.ts web/src/i18n/en.ts
git commit -m "feat(web): admin Paramètres tab — grouped settings, probes, force, reset"
```

---

### Task 12: Full-stack live pass + docs

**Files:**
- Modify: `README.md` (one paragraph pointing at the Paramètres tab)
- No other files (verification task; fixes go to the owning task's files)

- [ ] **Step 1: Full suites**

Run: `go test ./... 2>&1 | grep -v '^ok\|no test files'; cd web && npx tsc --noEmit && npx vitest run src/`
Expected: all green.

- [ ] **Step 2: Rebuild and drive the stack**

```bash
docker compose -f docker-compose.yml -f docker-compose.full.yml up -d --build portal web
```

Then, in a browser (or curl with the admin JWT from POST /api/auth/login):

1. Open `/admin/settings` — all 8 groups render; `server` group read-only; every source badge says `env`.
2. Change `SMTP_HOST` to `bogus.invalid`, save → probe failure with detail; force-save → applied, badge flips to `modifié`.
3. Trigger a subscription approval (or registration if verification enabled) → portal logs show the send failing against `bogus.invalid` — proving live apply with **no restart** (check `docker compose ps` start times).
4. Reset `SMTP_HOST` → badge back to `env`; repeat the email action → arrives in Mailpit.
5. Set `REQUIRE_EMAIL_VERIFICATION` on via the toggle (SMTP configured) → register a fresh account → 201 without token; toggle off → that same unverified account logs in (dynamic gate). Clean up test users.
6. Blank both sandbox URLs → product detail try-it loses the Sandbox environment; restore → it returns.
7. `SELECT key, old_value, new_value, admin_id FROM portal_settings_audit ORDER BY id` shows every change with secrets redacted.

- [ ] **Step 3: README** — add one paragraph under the admin/bootstrap section: settings are live-editable at **Admin → Paramètres** (env vars seed the defaults; UI overrides live in Postgres and win; secrets write-only; boot-critical parameters — DATABASE_URL, PORTAL_ADDR, PORTAL_ENV, JWT_SECRET, CREDENTIAL_ENC_KEY — remain env-only).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: runtime settings — Admin → Paramètres"
```
