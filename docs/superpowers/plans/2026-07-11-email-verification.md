# Email Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Opt-in email verification: with `REQUIRE_EMAIL_VERIFICATION=1`, new registrants must click an emailed link before they can log in.

**Architecture:** A random 128-bit token is emailed as a link and stored SHA-256-hashed in `users` with a 24 h expiry. Registration (feature on) creates the user unverified and withholds the JWT; login refuses unverified users with a distinct 403; public verify/resend endpoints complete the loop. The feature is inert unless the flag is set, and setting it without SMTP is a startup-fatal misconfiguration.

**Tech Stack:** Go (chi, pgx), Postgres migrations via `internal/db`, `internal/notify` SMTP sender, React + vitest frontend.

**Spec:** `docs/superpowers/specs/2026-07-11-email-verification-design.md`

## Global Constraints

- Flag: `REQUIRE_EMAIL_VERIFICATION` (`"1"` = on, anything else = off).
- Fail-fast: flag on + `!SMTPConfigured()` ⇒ `Config.Validate()` returns an error (in ALL envs, including dev — this misconfig locks users out everywhere).
- Token: 16 random bytes hex (32 chars, same shape as `subscriptions.GenerateKey`), stored as SHA-256 hex, valid 24 h, single-use, resend replaces it.
- Login gate error: HTTP 403, i18n code `auth.login.emailNotVerified` (401 remains bad-credentials).
- Verify failure: HTTP 410, i18n code `auth.verify.invalidOrExpired`.
- Resend: always 204 (no account-existence oracle), rate-limited per email.
- Migration `0019_email_verification.sql`, `email_verified BOOLEAN NOT NULL DEFAULT TRUE` (grandfathers existing rows).
- Backend tests: `go test ./internal/...` (DB-backed tests skip without Postgres; run the compose stack for them). Frontend: `cd web && npx vitest run src/...`.
- Commit after every task; message style `feat(auth): ...` / `test: ...` matching repo history.

---

### Task 1: Config flag + fail-fast validation

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.RequireEmailVerification bool`; `Config.Validate()` errors when the flag is on without SMTP. Consumed by Task 6 (server wiring) and `cmd/portal/main.go` (already calls `Validate()`).

- [ ] **Step 1: Write the failing tests** — append to `internal/config/config_test.go` (check existing helpers in that file for how env is set; it uses `t.Setenv`):

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run 'TestRequireEmailVerification|TestValidateRejects' -v`
Expected: FAIL — `Load().RequireEmailVerification` undefined (compile error).

- [ ] **Step 3: Implement** — in `internal/config/config.go`:

Add to the `Config` struct (near the SMTP fields):

```go
	RequireEmailVerification bool
```

Add to `Load()`'s struct literal:

```go
		RequireEmailVerification: get("REQUIRE_EMAIL_VERIFICATION", "") == "1",
```

In `Validate()` (line ~109), add BEFORE the `if c.isDevLike() { return nil }` short-circuit:

```go
	if c.RequireEmailVerification && !c.SMTPConfigured() {
		return fmt.Errorf("REQUIRE_EMAIL_VERIFICATION=1 needs a mail server: set SMTP_HOST and SMTP_FROM, or unset REQUIRE_EMAIL_VERIFICATION")
	}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): REQUIRE_EMAIL_VERIFICATION flag, fatal without SMTP"
```

---

### Task 2: Migration 0019 (columns + grandfathering)

**Files:**
- Create: `internal/db/migrations/0019_email_verification.sql`
- Test: `internal/db/migrate_email_verification_test.go`

**Interfaces:**
- Produces: `users.email_verified BOOLEAN NOT NULL DEFAULT TRUE`, `users.verify_token_hash TEXT NULL`, `users.verify_token_expires_at TIMESTAMPTZ NULL`. Consumed by Task 3 repo queries.

- [ ] **Step 1: Write the failing test** — create `internal/db/migrate_email_verification_test.go` (pattern of `migrate_teams_test.go`: connect-or-skip, then assert):

```go
package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEmailVerificationMigration(t *testing.T) {
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
	// A plain insert (no email_verified column) must default to TRUE — this is
	// the same path every pre-migration row takes, i.e. grandfathering.
	suf := time.Now().Format("150405.000000000")
	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','U','developer') RETURNING id`,
		"verif+"+suf+"@e.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid) })
	var verified bool
	if err := pool.QueryRow(ctx,
		`SELECT email_verified FROM users WHERE id=$1`, uid).Scan(&verified); err != nil {
		t.Fatalf("email_verified column missing: %v", err)
	}
	if !verified {
		t.Fatal("default must be TRUE (grandfathering)")
	}
	// Token columns exist and are nullable.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET verify_token_hash='h', verify_token_expires_at=now() WHERE id=$1`, uid); err != nil {
		t.Fatalf("token columns: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/db/ -run TestEmailVerificationMigration -v`
Expected: FAIL with `email_verified column missing` (or SKIP if no local Postgres — in that case run `make up` first; the compose stack maps Postgres to `127.0.0.1:5432`).

- [ ] **Step 3: Create the migration** — `internal/db/migrations/0019_email_verification.sql`:

```sql
-- Email verification (opt-in via REQUIRE_EMAIL_VERIFICATION). DEFAULT TRUE
-- grandfathers every existing account; the register path explicitly inserts
-- FALSE when the feature is enabled.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN IF NOT EXISTS verify_token_hash TEXT,
  ADD COLUMN IF NOT EXISTS verify_token_expires_at TIMESTAMPTZ;
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/db/ -run TestEmailVerificationMigration -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/0019_email_verification.sql internal/db/migrate_email_verification_test.go
git commit -m "feat(db): email_verified + verify token columns (grandfathered TRUE)"
```

---

### Task 3: Auth repo — unverified create, verify, resend-reset + token helpers

**Files:**
- Modify: `internal/auth/user.go` (User struct + token helpers)
- Modify: `internal/auth/repo.go`
- Test: `internal/auth/repo_test.go` (DB-backed, connect-or-skip pattern already in the file), `internal/auth/user_test.go`

**Interfaces:**
- Produces (consumed by Task 4's handler code and its `memRepo` fakes):

```go
// user.go
type User struct { ... Verified bool `json:"-"` }        // added field, not serialized
func GenerateVerifyToken() (plain, hash string)           // 32-hex plain, sha256-hex hash
func HashVerifyToken(plain string) string                 // sha256 hex

// repo.go — sentinel errors
var ErrTokenInvalid = errors.New("auth: verification token invalid or expired")
var ErrAlreadyVerified = errors.New("auth: email already verified")

// repo.go — methods (all on *Repo)
CreateUnverified(ctx context.Context, email, passwordHash, name, lang, verifyTokenHash string, expiresAt time.Time) (User, error)
VerifyByTokenHash(ctx context.Context, tokenHash string) error            // nil | ErrTokenInvalid
ResetVerifyToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error) // User | ErrUserNotFound | ErrAlreadyVerified
```

- `GetByEmail` additionally scans `email_verified` into `User.Verified`.

- [ ] **Step 1: Write failing helper tests** — append to `internal/auth/user_test.go`:

```go
func TestGenerateVerifyToken(t *testing.T) {
	plain, hash := GenerateVerifyToken()
	if len(plain) != 32 {
		t.Fatalf("plain len = %d, want 32 hex chars", len(plain))
	}
	if hash != HashVerifyToken(plain) {
		t.Fatal("hash must equal HashVerifyToken(plain)")
	}
	if hash == plain {
		t.Fatal("hash must not equal the plain token")
	}
	p2, _ := GenerateVerifyToken()
	if p2 == plain {
		t.Fatal("two tokens must differ")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/auth/ -run TestGenerateVerifyToken -v`
Expected: FAIL — `GenerateVerifyToken` undefined.

- [ ] **Step 3: Implement helpers** — in `internal/auth/user.go` add imports `crypto/rand`, `crypto/sha256`, `encoding/hex` and:

```go
// Verified reports whether the account's email address has been confirmed.
// Only meaningful when REQUIRE_EMAIL_VERIFICATION is enabled; the column
// defaults to TRUE so it never blocks pre-feature accounts.
```

Add `Verified bool \`json:"-"\`` to the `User` struct, then:

```go
// GenerateVerifyToken returns a random email-verification token and its
// SHA-256 hex digest (only the digest is stored). Panics on CSPRNG failure,
// like subscriptions.GenerateKey.
func GenerateVerifyToken() (plain, hash string) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	plain = hex.EncodeToString(b)
	return plain, HashVerifyToken(plain)
}

// HashVerifyToken returns the SHA-256 hex digest under which a verification
// token is stored and looked up.
func HashVerifyToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run helper test to verify pass**

Run: `go test ./internal/auth/ -run TestGenerateVerifyToken -v`
Expected: PASS.

- [ ] **Step 5: Write failing repo tests** — append to `internal/auth/repo_test.go` (reuse the file's existing connect-or-skip helper if present; otherwise copy its inline pattern):

```go
func TestEmailVerificationRepoFlow(t *testing.T) {
	pool := testPool(t) // use/extract the file's existing pool setup; skip if none
	repo := NewRepo(pool)
	ctx := context.Background()
	email := "verify+" + time.Now().Format("150405.000000000") + "@e2e.test"

	plain, hash := GenerateVerifyToken()
	u, err := repo.CreateUnverified(ctx, email, "x", "V User", "fr", hash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateUnverified: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })

	got, _, err := repo.GetByEmail(ctx, email)
	if err != nil || got.Verified {
		t.Fatalf("fresh user must be unverified (err=%v verified=%v)", err, got.Verified)
	}

	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken("wrong-token")); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("wrong token: got %v, want ErrTokenInvalid", err)
	}
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); err != nil {
		t.Fatalf("VerifyByTokenHash: %v", err)
	}
	got, _, _ = repo.GetByEmail(ctx, email)
	if !got.Verified {
		t.Fatal("user must be verified after VerifyByTokenHash")
	}
	// Single-use: same token again fails (hash was cleared).
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("reused token: got %v, want ErrTokenInvalid", err)
	}
	// Resend on a verified account refuses.
	if _, err := repo.ResetVerifyToken(ctx, email, "newhash", time.Now().Add(time.Hour)); !errors.Is(err, ErrAlreadyVerified) {
		t.Fatalf("reset on verified: got %v, want ErrAlreadyVerified", err)
	}
	// Resend on an unknown account reports not-found (handler still answers 204).
	if _, err := repo.ResetVerifyToken(ctx, "nobody@nowhere.test", "h", time.Now()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("reset unknown: got %v, want ErrUserNotFound", err)
	}
}

func TestExpiredTokenIsInvalid(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	email := "expired+" + time.Now().Format("150405.000000000") + "@e2e.test"
	plain, hash := GenerateVerifyToken()
	u, err := repo.CreateUnverified(ctx, email, "x", "", "en", hash, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("CreateUnverified: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token: got %v, want ErrTokenInvalid", err)
	}
	// A reset (resend) revives the flow with a fresh token.
	plain2, hash2 := GenerateVerifyToken()
	_ = plain2
	if _, err := repo.ResetVerifyToken(ctx, email, hash2, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("ResetVerifyToken: %v", err)
	}
	if err := repo.VerifyByTokenHash(ctx, hash2); err != nil {
		t.Fatalf("verify after reset: %v", err)
	}
}
```

If `repo_test.go` has no shared `testPool(t)` helper, add one at the top of the file matching its existing inline connection code:

```go
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("no database: %v", err)
	}
	return pool
}
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/auth/ -run 'TestEmailVerificationRepoFlow|TestExpiredTokenIsInvalid' -v`
Expected: FAIL — `CreateUnverified` undefined.

- [ ] **Step 7: Implement repo methods** — in `internal/auth/repo.go` (add `time` import):

Add the sentinels next to the existing ones:

```go
// ErrTokenInvalid is returned by VerifyByTokenHash when no user carries the
// hash or the token has expired.
var ErrTokenInvalid = errors.New("auth: verification token invalid or expired")

// ErrAlreadyVerified is returned by ResetVerifyToken for accounts that no
// longer need verification.
var ErrAlreadyVerified = errors.New("auth: email already verified")
```

Refactor `Create` to delegate, keeping its exact signature and behavior:

```go
// Create inserts a developer user AND their personal team (a team of one) in a
// single transaction, returning the user. The user is email-verified (the
// column default) — used when REQUIRE_EMAIL_VERIFICATION is off.
func (r *Repo) Create(ctx context.Context, email, passwordHash, name, lang string) (User, error) {
	return r.create(ctx, email, passwordHash, name, lang, "", nil)
}

// CreateUnverified is Create with email_verified=FALSE plus a pending
// verification token (hash + expiry) — used when REQUIRE_EMAIL_VERIFICATION
// is on.
func (r *Repo) CreateUnverified(ctx context.Context, email, passwordHash, name, lang, verifyTokenHash string, expiresAt time.Time) (User, error) {
	return r.create(ctx, email, passwordHash, name, lang, verifyTokenHash, &expiresAt)
}

func (r *Repo) create(ctx context.Context, email, passwordHash, name, lang, tokenHash string, expiresAt *time.Time) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var u User
	if expiresAt == nil {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, language)
			 VALUES ($1,$2,$3,'developer',$4)
			 RETURNING id, email, name, role, language, email_verified`,
			email, passwordHash, name, lang,
		).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified)
	} else {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, language, email_verified, verify_token_hash, verify_token_expires_at)
			 VALUES ($1,$2,$3,'developer',$4, FALSE, $5, $6)
			 RETURNING id, email, name, role, language, email_verified`,
			email, passwordHash, name, lang, tokenHash, *expiresAt,
		).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	// (keep the existing personal-team INSERTs and Commit exactly as they are)
	teamName := name
	if teamName == "" {
		teamName = email
	}
	var teamID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO teams(name, personal) VALUES($1, true) RETURNING id`, teamName).Scan(&teamID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'owner')`, teamID, u.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}
```

Update `GetByEmail` to scan the flag:

```go
func (r *Repo) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, language, email_verified, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified, &hash)
	return u, hash, err
}
```

Add the two verification methods:

```go
// VerifyByTokenHash marks the user carrying this token hash as verified and
// burns the token. ErrTokenInvalid covers unknown, already-used and expired
// tokens alike (they are indistinguishable to the caller by design).
func (r *Repo) VerifyByTokenHash(ctx context.Context, tokenHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users
		 SET email_verified = TRUE, verify_token_hash = NULL, verify_token_expires_at = NULL
		 WHERE verify_token_hash = $1 AND verify_token_expires_at > now()`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenInvalid
	}
	return nil
}

// ResetVerifyToken stores a fresh token hash/expiry for an unverified account
// (invalidating any previous link) and returns the user for email rendering.
func (r *Repo) ResetVerifyToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error) {
	var u User
	var verified bool
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, language, email_verified FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	if verified {
		return User{}, ErrAlreadyVerified
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE users SET verify_token_hash=$2, verify_token_expires_at=$3 WHERE id=$1`,
		u.ID, tokenHash, expiresAt)
	return u, err
}
```

- [ ] **Step 8: Run to verify pass** (needs the compose Postgres: `make up` if not running)

Run: `go test ./internal/auth/ -v 2>&1 | tail -20`
Expected: PASS (including pre-existing repo tests — `Create` behavior is unchanged).

- [ ] **Step 9: Commit**

```bash
git add internal/auth/user.go internal/auth/user_test.go internal/auth/repo.go internal/auth/repo_test.go
git commit -m "feat(auth): verification token helpers + repo create/verify/reset"
```

---

### Task 4: Verification email template in notify

**Files:**
- Modify: `internal/notify/notifier.go` (templates map)
- Create: `internal/notify/verification.go`
- Test: `internal/notify/verification_test.go`

**Interfaces:**
- Produces (consumed by Task 5 handler and Task 7 wiring):

```go
// SendVerificationEmail renders the localized verification email and sends it.
func SendVerificationEmail(ctx context.Context, s Sender, lang, email, name, link string) error
```

- [ ] **Step 1: Write the failing test** — `internal/notify/verification_test.go` (check `notifier_test.go` first: it defines a fake `Sender`; reuse it if exported within the package, else define the one below):

```go
package notify

import (
	"context"
	"strings"
	"testing"
)

type captureSender struct {
	to      []string
	subject string
	body    string
}

func (c *captureSender) Send(_ context.Context, to []string, subject, body string) error {
	c.to, c.subject, c.body = to, subject, body
	return nil
}

func TestSendVerificationEmailFrench(t *testing.T) {
	s := &captureSender{}
	err := SendVerificationEmail(context.Background(), s, "fr", "dev@x.io", "Walid", "http://localhost:8088/verify-email?token=abc")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(s.to) != 1 || s.to[0] != "dev@x.io" {
		t.Fatalf("to = %v", s.to)
	}
	if !strings.Contains(s.subject, "Vérifiez") {
		t.Fatalf("subject = %q, want French", s.subject)
	}
	if !strings.Contains(s.body, "http://localhost:8088/verify-email?token=abc") {
		t.Fatal("body must contain the link")
	}
	if !strings.Contains(s.body, "Walid") {
		t.Fatal("body must greet the user by name")
	}
	if !strings.Contains(s.body, "24") {
		t.Fatal("body must mention the 24 h validity")
	}
}

func TestSendVerificationEmailEnglishAndFallbacks(t *testing.T) {
	s := &captureSender{}
	// Unknown language falls back like the notifier (normalizeLang), empty name
	// falls back to the email address in the greeting.
	if err := SendVerificationEmail(context.Background(), s, "de", "dev@x.io", "", "http://l/verify-email?token=t"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(s.subject, "Verify") && !strings.Contains(s.subject, "Vérifiez") {
		t.Fatalf("subject = %q, want a known-language fallback", s.subject)
	}
	if !strings.Contains(s.body, "dev@x.io") {
		t.Fatal("empty name must fall back to the email address")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/notify/ -run TestSendVerificationEmail -v`
Expected: FAIL — `SendVerificationEmail` undefined.

- [ ] **Step 3: Implement** — create `internal/notify/verification.go`:

```go
package notify

import (
	"context"
	"fmt"
)

// verification email — same fr/en table pattern as emailTemplates; body args
// are (greetingName, link).
var verifyTemplates = map[string]emailTemplate{
	"fr": {
		subject: "Vérifiez votre adresse e-mail",
		body:    "Bonjour %s,\n\nConfirmez votre adresse e-mail pour activer votre compte du portail développeur :\n\n%s\n\nCe lien est valable 24 heures. Si vous n'êtes pas à l'origine de cette inscription, ignorez ce message.\n",
	},
	"en": {
		subject: "Verify your email address",
		body:    "Hello %s,\n\nConfirm your email address to activate your developer portal account:\n\n%s\n\nThis link is valid for 24 hours. If you did not sign up, you can ignore this message.\n",
	},
}

// SendVerificationEmail renders the localized verification email and sends it
// via the given Sender. lang falls back like the notifier's other emails;
// an empty name falls back to the email address in the greeting.
func SendVerificationEmail(ctx context.Context, s Sender, lang, email, name, link string) error {
	if name == "" {
		name = email
	}
	tpl := verifyTemplates[normalizeLang(lang)]
	return s.Send(ctx, []string{email}, tpl.subject, fmt.Sprintf(tpl.body, name, link))
}
```

(`normalizeLang` and `emailTemplate` already exist in the package — check `notifier.go`; if `normalizeLang` defaults to `"fr"`, both tests pass as written.)

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/notify/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/verification.go internal/notify/verification_test.go
git commit -m "feat(notify): localized email-verification message"
```

---

### Task 5: Auth handler — register/login gate + verify/resend endpoints

**Files:**
- Modify: `internal/auth/handler.go`
- Modify: `internal/i18n/catalog_fr.go`, `internal/i18n/catalog_en.go`
- Test: `internal/auth/handler_test.go`

**Interfaces:**
- Consumes: Task 3 repo methods + helpers, Task 4 `notify.SendVerificationEmail`.
- Produces (consumed by Task 7 server wiring):

```go
// VerificationConfig wires the opt-in email-verification gate into the handler.
type VerificationConfig struct {
	Sender   notify.Sender        // required
	BaseURL  string               // PORTAL_BASE_URL, for the link
	Limiter  *httpx.RateLimiter   // per-email resend limiter (nil = unlimited)
	TokenTTL time.Duration        // 0 => 24h
	GenToken func() (plain, hash string) // nil => GenerateVerifyToken
}
func (h *Handler) EnableEmailVerification(vc VerificationConfig)
```

- `UserStore` interface gains: `CreateUnverified(...)`, `VerifyByTokenHash(...)`, `ResetVerifyToken(...)` with the Task 3 signatures (satisfied by `*Repo`).
- New routes (registered only when enabled): `POST /api/auth/verify` `{token}` → 204 | 410 `auth.verify.invalidOrExpired`; `POST /api/auth/resend-verification` `{email}` → 204 | 429.
- Register (enabled): 201 `{"user": ..., "verificationRequired": true}` (NO token). Login (enabled, unverified): 403 `auth.login.emailNotVerified`.

- [ ] **Step 1: Extend the memRepo fake** — in `internal/auth/handler_test.go`, extend the stored struct and add the three methods (the interface change breaks compilation otherwise):

```go
// inside memRepo's map value struct: add fields
//   verified  bool
//   tokenHash string
//   expires   time.Time
// (declare the struct once as a named type to avoid repeating it)

type memUser struct {
	u         User
	hash      string
	verified  bool
	tokenHash string
	expires   time.Time
}

type memRepo struct {
	byEmail map[string]*memUser
	nextID  int64
}

func newMemRepo() *memRepo { return &memRepo{byEmail: map[string]*memUser{}} }

func (m *memRepo) Create(_ context.Context, email, hash, name, lang string) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer", Language: lang, Verified: true}
	m.byEmail[email] = &memUser{u: u, hash: hash, verified: true}
	return u, nil
}

func (m *memRepo) CreateUnverified(_ context.Context, email, hash, name, lang, tokenHash string, expiresAt time.Time) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer", Language: lang}
	m.byEmail[email] = &memUser{u: u, hash: hash, tokenHash: tokenHash, expires: expiresAt}
	return u, nil
}

func (m *memRepo) GetByEmail(_ context.Context, email string) (User, string, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, "", errors.New("not found")
	}
	u := v.u
	u.Verified = v.verified
	return u, v.hash, nil
}

func (m *memRepo) VerifyByTokenHash(_ context.Context, tokenHash string) error {
	for _, v := range m.byEmail {
		if v.tokenHash == tokenHash && time.Now().Before(v.expires) {
			v.verified, v.tokenHash = true, ""
			return nil
		}
	}
	return ErrTokenInvalid
}

func (m *memRepo) ResetVerifyToken(_ context.Context, email, tokenHash string, expiresAt time.Time) (User, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, ErrUserNotFound
	}
	if v.verified {
		return User{}, ErrAlreadyVerified
	}
	v.tokenHash, v.expires = tokenHash, expiresAt
	u := v.u
	return u, nil
}

func (m *memRepo) SetLanguage(_ context.Context, userID int64, lang string) error {
	for _, v := range m.byEmail {
		if v.u.ID == userID {
			v.u.Language = lang
			return nil
		}
	}
	return errors.New("not found")
}
```

(Adjust the file's existing tests that construct `memRepo` literals if any break — the constructor `newMemRepo()` keeps most call sites unchanged. Add imports `time`, `apisix-portal/internal/notify` as needed.)

- [ ] **Step 2: Write the failing handler tests** — append to `internal/auth/handler_test.go`:

```go
type fakeVerifSender struct {
	to   string
	body string
	n    int
}

func (f *fakeVerifSender) Send(_ context.Context, to []string, _ string, body string) error {
	f.n++
	f.to = strings.Join(to, ",")
	f.body = body
	return nil
}

func verifHandler(store UserStore, sender *fakeVerifSender) *Handler {
	h := NewHandler(store, NewTokenizer("test-secret"), nil)
	h.EnableEmailVerification(VerificationConfig{
		Sender:  sender,
		BaseURL: "http://localhost:8088",
		GenToken: func() (string, string) {
			return "fixedtoken", HashVerifyToken("fixedtoken")
		},
	})
	return h
}

func postAuth(h *Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req = req.WithContext(i18n.WithLang(req.Context(), i18n.Lang("en")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterWithVerificationWithholdsToken(t *testing.T) {
	sender := &fakeVerifSender{}
	h := verifHandler(newMemRepo(), sender)
	rec := postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough", Name: "D"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, hasToken := body["token"]; hasToken {
		t.Fatal("register must NOT return a token when verification is required")
	}
	if body["verificationRequired"] != true {
		t.Fatal("response must carry verificationRequired: true")
	}
	if sender.n != 1 || sender.to != "d@x.io" {
		t.Fatalf("verification email not sent (n=%d to=%q)", sender.n, sender.to)
	}
	if !strings.Contains(sender.body, "http://localhost:8088/verify-email?token=fixedtoken") {
		t.Fatalf("email body must contain the link, got %q", sender.body)
	}
}

func TestRegisterWithoutVerificationKeepsOldBehavior(t *testing.T) {
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	rec := postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == nil || body["token"] == "" {
		t.Fatal("feature off: register must still auto-login with a token")
	}
}

func TestLoginBlockedUntilVerified(t *testing.T) {
	sender := &fakeVerifSender{}
	store := newMemRepo()
	h := verifHandler(store, sender)
	postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})

	rec := postAuth(h, "/api/auth/login", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unverified login status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}

	rec = postAuth(h, "/api/auth/verify", map[string]string{"token": "fixedtoken"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("verify status = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}

	rec = postAuth(h, "/api/auth/login", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusOK {
		t.Fatalf("verified login status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestVerifyBadTokenReturns410(t *testing.T) {
	h := verifHandler(newMemRepo(), &fakeVerifSender{})
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "nope"})
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rec.Code)
	}
}

func TestResendAlways204AndOnlyMailsUnverified(t *testing.T) {
	sender := &fakeVerifSender{}
	store := newMemRepo()
	h := verifHandler(store, sender)
	postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})
	base := sender.n

	// unknown account: 204, no email
	rec := postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "ghost@x.io"})
	if rec.Code != http.StatusNoContent || sender.n != base {
		t.Fatalf("unknown: code=%d mails=%d, want 204 and no mail", rec.Code, sender.n-base)
	}
	// unverified account: 204 + email
	rec = postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusNoContent || sender.n != base+1 {
		t.Fatalf("unverified: code=%d mails=%d, want 204 and one mail", rec.Code, sender.n-base)
	}
	// verify, then resend: 204, no new email
	postAuth(h, "/api/auth/verify", map[string]string{"token": "fixedtoken"})
	rec = postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusNoContent || sender.n != base+1 {
		t.Fatalf("verified: code=%d mails=%d, want 204 and no mail", rec.Code, sender.n-base-1)
	}
}

func TestVerifyRoutesAbsentWhenDisabled(t *testing.T) {
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "x"})
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("feature off: verify route must not exist, got %d", rec.Code)
	}
}
```

(`i18n.WithLang(ctx, i18n.Lang("en"))` is the real API — see `internal/i18n/i18n.go:17`.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/auth/ -run 'TestRegisterWith|TestLoginBlocked|TestVerify|TestResend' -v`
Expected: FAIL — `EnableEmailVerification`/`VerificationConfig` undefined.

- [ ] **Step 4: Implement the handler** — in `internal/auth/handler.go` (add imports `log`, `time`, `apisix-portal/internal/notify`):

Extend the interface:

```go
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name, lang string) (User, error)
	CreateUnverified(ctx context.Context, email, passwordHash, name, lang, verifyTokenHash string, expiresAt time.Time) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
	SetLanguage(ctx context.Context, userID int64, lang string) error
	VerifyByTokenHash(ctx context.Context, tokenHash string) error
	ResetVerifyToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error)
}
```

Add the config type and enable method:

```go
// VerificationConfig wires the opt-in email-verification gate (spec
// 2026-07-11). Zero-value fields get safe defaults in EnableEmailVerification.
type VerificationConfig struct {
	Sender   notify.Sender
	BaseURL  string
	Limiter  *httpx.RateLimiter
	TokenTTL time.Duration
	GenToken func() (plain, hash string)
}

// EnableEmailVerification switches the handler into verified-only mode:
// register withholds the JWT and emails a link, login refuses unverified
// accounts, and the verify/resend endpoints are mounted.
func (h *Handler) EnableEmailVerification(vc VerificationConfig) {
	if vc.TokenTTL == 0 {
		vc.TokenTTL = 24 * time.Hour
	}
	if vc.GenToken == nil {
		vc.GenToken = GenerateVerifyToken
	}
	h.verify = &vc
	h.router.Post("/api/auth/verify", h.verifyEmail)
	h.router.Post("/api/auth/resend-verification", h.resendVerification)
}
```

Add `verify *VerificationConfig` to the `Handler` struct.

Rework `register`'s tail (after the password-hash block; the decode/validate part stays):

```go
	lang := string(i18n.FromContext(r.Context()))
	if h.verify != nil {
		plain, tokenHash := h.verify.GenToken()
		u, err := h.store.CreateUnverified(r.Context(), c.Email, hash, c.Name, lang, tokenHash, time.Now().Add(h.verify.TokenTTL))
		if errors.Is(err, ErrEmailTaken) {
			httpx.ErrorT(w, r, http.StatusConflict, "auth.register.emailTaken")
			return
		}
		if err != nil {
			httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.register.createFailed")
			return
		}
		h.sendVerification(u, lang, plain)
		httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "verificationRequired": true})
		return
	}
	// existing path, unchanged: Create + Issue + 201 {user, token}
```

Add the mail helper (best-effort like notify.deliver):

```go
// sendVerification emails the verification link; failures are logged only —
// the resend endpoint is the recovery path (spec: best-effort like notify).
func (h *Handler) sendVerification(u User, lang, plainToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	link := h.verify.BaseURL + "/verify-email?token=" + plainToken
	if err := notify.SendVerificationEmail(ctx, h.verify.Sender, lang, u.Email, u.Name, link); err != nil {
		log.Printf("auth: verification email to %s: %v", u.Email, err)
	}
}
```

In `login`, after the `CheckPassword` success check and before `Issue`:

```go
	if h.verify != nil && !u.Verified {
		httpx.ErrorT(w, r, http.StatusForbidden, "auth.login.emailNotVerified")
		return
	}
```

Add the two endpoints:

```go
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	switch err := h.store.VerifyByTokenHash(r.Context(), HashVerifyToken(body.Token)); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrTokenInvalid):
		httpx.ErrorT(w, r, http.StatusGone, "auth.verify.invalidOrExpired")
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.verify.failed")
	}
}

// resendVerification always answers 204 so responses never disclose whether
// an account exists or is verified (same discipline as the login timing
// equalization, M3).
func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if h.verify.Limiter != nil && !h.verify.Limiter.Allow(strings.ToLower(body.Email)) {
		if ra := h.verify.Limiter.RetryAfter(); ra != "" {
			w.Header().Set("Retry-After", ra)
		}
		httpx.ErrorT(w, r, http.StatusTooManyRequests, "auth.login.tooManyAttempts")
		return
	}
	plain, tokenHash := h.verify.GenToken()
	u, err := h.store.ResetVerifyToken(r.Context(), body.Email, tokenHash, time.Now().Add(h.verify.TokenTTL))
	if err == nil {
		lang := u.Language
		if lang == "" {
			lang = string(i18n.FromContext(r.Context()))
		}
		h.sendVerification(u, lang, plain)
	} else if !errors.Is(err, ErrUserNotFound) && !errors.Is(err, ErrAlreadyVerified) {
		log.Printf("auth: resend verification for %s: %v", body.Email, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Add i18n catalog entries** — in `internal/i18n/catalog_fr.go` (match the file's map style):

```go
	"auth.login.emailNotVerified": "adresse e-mail non vérifiée — consultez votre boîte de réception",
	"auth.verify.invalidOrExpired": "lien de vérification invalide ou expiré",
	"auth.verify.failed": "échec de la vérification",
```

and in `catalog_en.go`:

```go
	"auth.login.emailNotVerified": "email address not verified — check your inbox",
	"auth.verify.invalidOrExpired": "verification link invalid or expired",
	"auth.verify.failed": "verification failed",
```

- [ ] **Step 6: Run to verify pass**

Run: `go test ./internal/auth/ ./internal/i18n/ -v 2>&1 | tail -15`
Expected: PASS, including all pre-existing auth handler tests (feature-off paths unchanged).

- [ ] **Step 7: Commit**

```bash
git add internal/auth/handler.go internal/auth/handler_test.go internal/i18n/catalog_fr.go internal/i18n/catalog_en.go
git commit -m "feat(auth): email verification gate — register/login + verify/resend endpoints"
```

---

### Task 6: Server wiring + compose/README documentation

**Files:**
- Modify: `internal/server/server.go:74-77` (SMTP block)
- Modify: `docker-compose.full.yml` (commented-out flag), `README.md` (flag doc)

**Interfaces:**
- Consumes: `cfg.RequireEmailVerification` (Task 1), `authH.EnableEmailVerification` (Task 5).

- [ ] **Step 1: Wire it** — in `internal/server/server.go`, restructure the SMTP block so the sender is shared:

```go
	if cfg.SMTPConfigured() {
		sender := notify.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
		subSvc.SetNotifier(notify.NewNotifier(sender, notify.NewRepo(pool), cfg.PortalBaseURL))
		if cfg.RequireEmailVerification {
			// Config.Validate() already guarantees SMTP is set when the flag is on;
			// resends are limited to 3 quick tries then 1/min per email address.
			authH.EnableEmailVerification(auth.VerificationConfig{
				Sender:  sender,
				BaseURL: cfg.PortalBaseURL,
				Limiter: httpx.NewRateLimiter(3, 1.0/60),
			})
		}
	}
```

(NOTE: `authH` is created at line ~50, before this block — no reordering needed. `httpx` is already imported.)

- [ ] **Step 2: Build + full test sweep**

Run: `go build ./... && go test ./... 2>&1 | grep -v '^ok\|no test files'`
Expected: no output (everything passes).

- [ ] **Step 3: Document** — in `docker-compose.full.yml`, under the portal service's `environment:`, add a commented line after the SMTP vars:

```yaml
      # REQUIRE_EMAIL_VERIFICATION: "1"  # new accounts must click the emailed link before login (needs SMTP_*)
```

In `README.md`, next to where SMTP/Mailpit is described, add one sentence: setting `REQUIRE_EMAIL_VERIFICATION=1` (with SMTP configured, otherwise the portal refuses to start) forces new registrations to confirm their email via the link before they can log in; in the dev stack the email lands in Mailpit (http://localhost:8025).

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go docker-compose.full.yml README.md
git commit -m "feat(server): wire opt-in email verification; document the flag"
```

---

### Task 7: Frontend — API client + AuthProvider

**Files:**
- Modify: `web/src/api/client.ts`, `web/src/api/types.ts`, `web/src/auth/AuthProvider.tsx`
- Test: `web/src/auth/AuthProvider.test.tsx` (append)

**Interfaces:**
- Produces (consumed by Tasks 8-10):

```ts
// types.ts
export interface RegisterResponse { user: User; token?: string; verificationRequired?: boolean }
// client.ts
export async function register(email, password, name): Promise<RegisterResponse>  // signature unchanged, type widened
export async function verifyEmail(token: string): Promise<void>                   // POST /api/auth/verify
export async function resendVerification(email: string): Promise<void>            // POST /api/auth/resend-verification
// AuthProvider
register: (email, password, name) => Promise<boolean>  // true = verification required (NOT logged in)
```

- [ ] **Step 1: Write the failing test** — append to `web/src/auth/AuthProvider.test.tsx` (follow the file's existing render/mocking pattern — it already tests `register`; mirror its setup):

```tsx
it('register returns true and stores no token when verification is required', async () => {
  vi.spyOn(api, 'register').mockResolvedValue({
    user: { id: 1, email: 'd@x.io', name: 'D', role: 'developer', language: 'fr' },
    verificationRequired: true,
  })
  let result: boolean | undefined
  function Probe() {
    const { register } = useAuth()
    return <button onClick={async () => { result = await register('d@x.io', 'longenough', 'D') }}>go</button>
  }
  render(<LanguageProvider><AuthProvider><Probe /></AuthProvider></LanguageProvider>)
  await userEvent.click(screen.getByText('go'))
  await waitFor(() => expect(result).toBe(true))
  expect(localStorage.getItem('token')).toBeNull()
})

it('register returns false and logs in when no verification is required', async () => {
  vi.spyOn(api, 'register').mockResolvedValue({
    user: { id: 1, email: 'd@x.io', name: 'D', role: 'developer', language: 'fr' },
    token: 'jwt-token',
  })
  let result: boolean | undefined
  function Probe() {
    const { register } = useAuth()
    return <button onClick={async () => { result = await register('d@x.io', 'longenough', 'D') }}>go</button>
  }
  render(<LanguageProvider><AuthProvider><Probe /></AuthProvider></LanguageProvider>)
  await userEvent.click(screen.getByText('go'))
  await waitFor(() => expect(result).toBe(false))
  expect(localStorage.getItem('token')).toBe('jwt-token')
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/auth/AuthProvider.test.tsx`
Expected: FAIL — type error / `register` resolves `AuthResponse` (token required), and AuthProvider `register` returns void.

- [ ] **Step 3: Implement**

`web/src/api/types.ts` — next to `AuthResponse`:

```ts
// Register may withhold the token when the server requires email verification.
export interface RegisterResponse {
  user: User
  token?: string
  verificationRequired?: boolean
}
```

`web/src/api/client.ts` — import `RegisterResponse`, then:

```ts
export async function register(email: string, password: string, name: string): Promise<RegisterResponse> {
  return parse<RegisterResponse>(await postJSON('/api/auth/register', { email, password, name }), '/api/auth/register')
}

export async function verifyEmail(token: string): Promise<void> {
  await parse<unknown>(await postJSON('/api/auth/verify', { token }), '/api/auth/verify')
}

export async function resendVerification(email: string): Promise<void> {
  await parse<unknown>(await postJSON('/api/auth/resend-verification', { email }), '/api/auth/resend-verification')
}
```

(`parse` already tolerates empty 204 bodies — `res.json().catch(() => ({}))`.)

`web/src/auth/AuthProvider.tsx`:

```ts
  register: (email: string, password: string, name: string) => Promise<boolean>
```

```ts
  const register = async (email: string, password: string, name: string): Promise<boolean> => {
    const res = await api.register(email, password, name)
    if (res.token) {
      apply({ user: res.user, token: res.token })
      return false
    }
    return true // verification required; not logged in
  }
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx tsc --noEmit && npx vitest run src/auth/`
Expected: PASS (typecheck + tests, including pre-existing ones).

- [ ] **Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/auth/AuthProvider.tsx web/src/auth/AuthProvider.test.tsx
git commit -m "feat(web): register may require verification; verify/resend API calls"
```

---

### Task 8: Frontend — Register page "check your inbox" state

**Files:**
- Modify: `web/src/pages/RegisterPage.tsx`, `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`
- Test: `web/src/pages/AuthPages.test.tsx` (append; this file already tests both auth pages)

**Interfaces:**
- Consumes: `register(): Promise<boolean>` from Task 7.

- [ ] **Step 1: Write the failing test** — append to `web/src/pages/AuthPages.test.tsx`, mirroring its existing register-flow test setup:

```tsx
it('shows the check-your-inbox panel when registration requires verification', async () => {
  vi.spyOn(api, 'register').mockResolvedValue({
    user: { id: 1, email: 'd@x.io', name: 'D', role: 'developer', language: 'fr' },
    verificationRequired: true,
  })
  renderRegister() // use the file's existing render helper for RegisterPage
  await userEvent.type(screen.getByLabelText(/e-mail/i), 'd@x.io')
  await userEvent.type(screen.getByLabelText(/mot de passe/i), 'longenough')
  await userEvent.click(screen.getByRole('button', { name: /créer/i }))
  expect(await screen.findByText(/vérifiez votre boîte/i)).toBeInTheDocument()
  // the form is replaced by the notice
  expect(screen.queryByRole('button', { name: /créer/i })).not.toBeInTheDocument()
})
```

(Adapt selectors to the file's existing helpers/labels — the register test already in the file shows the exact aria-labels; reuse them.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: FAIL — the panel doesn't exist; the page navigates instead.

- [ ] **Step 3: Implement** — `web/src/pages/RegisterPage.tsx`:

Add state `const [sent, setSent] = useState(false)` and change `onSubmit`'s success branch:

```tsx
      const needsVerification = await register(email, password, name)
      if (needsVerification) {
        setSent(true)
        setLoading(false)
      } else {
        nav('/')
      }
```

Render the notice instead of the form when `sent`:

```tsx
  if (sent) {
    return (
      <AuthShell>
        <div className="m-head">
          <h2>{t('auth.checkInboxTitle')}</h2>
          <p>{t('auth.checkInboxBody')}</p>
          <p><Link to="/login">{t('auth.login')}</Link></p>
        </div>
      </AuthShell>
    )
  }
```

i18n — `web/src/i18n/fr.ts` (inside the `auth` section):

```ts
    checkInboxTitle: 'Vérifiez votre boîte de réception',
    checkInboxBody: 'Nous venons de vous envoyer un lien de vérification. Cliquez-le pour activer votre compte (valable 24 h).',
```

`web/src/i18n/en.ts`:

```ts
    checkInboxTitle: 'Check your inbox',
    checkInboxBody: 'We just sent you a verification link. Click it to activate your account (valid for 24 h).',
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: PASS (existing register tests still pass — feature off returns `false` and still navigates).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/RegisterPage.tsx web/src/i18n/fr.ts web/src/i18n/en.ts web/src/pages/AuthPages.test.tsx
git commit -m "feat(web): check-your-inbox state after registration needing verification"
```

---

### Task 9: Frontend — Login page unverified notice + resend button

**Files:**
- Modify: `web/src/pages/LoginPage.tsx`, `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`
- Test: `web/src/pages/AuthPages.test.tsx` (append)

**Interfaces:**
- Consumes: `resendVerification(email)` from Task 7; `ApiError.status` (403 ⇒ unverified — the login endpoint's only 403).

- [ ] **Step 1: Write the failing test** — append to `web/src/pages/AuthPages.test.tsx`:

```tsx
it('offers resend when login fails with 403 (unverified email)', async () => {
  vi.spyOn(api, 'login').mockRejectedValue(new ApiError('email address not verified — check your inbox', 403))
  const resend = vi.spyOn(api, 'resendVerification').mockResolvedValue(undefined)
  renderLogin() // the file's existing render helper for LoginPage
  await userEvent.type(screen.getByLabelText(/e-mail/i), 'd@x.io')
  await userEvent.type(screen.getByLabelText(/mot de passe/i), 'longenough')
  await userEvent.click(screen.getByRole('button', { name: /connexion|se connecter/i }))
  const resendBtn = await screen.findByRole('button', { name: /renvoyer/i })
  await userEvent.click(resendBtn)
  await waitFor(() => expect(resend).toHaveBeenCalledWith('d@x.io'))
  expect(await screen.findByText(/envoyé/i)).toBeInTheDocument()
})
```

(Import `ApiError` from `../api/client`; adapt button-name regexes to the actual i18n strings in the file's existing login test.)

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: FAIL — no resend button appears.

- [ ] **Step 3: Implement** — `web/src/pages/LoginPage.tsx`:

Add state:

```tsx
  const [unverified, setUnverified] = useState(false)
  const [resent, setResent] = useState(false)
```

In `onSubmit`'s catch:

```tsx
    } catch (e) {
      if (e instanceof ApiError && e.status === 403) {
        setUnverified(true)
      }
      setErr(e instanceof Error ? e.message : t('auth.loginFailed'))
      setLoading(false)
    }
```

(import `ApiError` and `resendVerification` from `../api/client`.)

Under the existing error paragraph in the JSX:

```tsx
        {unverified && (
          <p className="form-err" role="alert">
            {resent ? t('auth.resendSent') : (
              <button type="button" className="linklike" onClick={async () => {
                try { await resendVerification(email) } catch { /* always answers 204; network errors get the same copy */ }
                setResent(true)
              }}>{t('auth.resendVerification')}</button>
            )}
          </p>
        )}
```

(If the stylesheet has no `linklike` class, check `web/src/styles` for an existing link-styled-button class and use that; otherwise add `.linklike { background:none;border:none;color:inherit;text-decoration:underline;cursor:pointer;padding:0;font:inherit }` next to the auth styles.)

i18n `fr.ts`:

```ts
    resendVerification: 'Renvoyer l’e-mail de vérification',
    resendSent: 'E-mail envoyé si un compte existe — pensez aussi aux indésirables.',
```

`en.ts`:

```ts
    resendVerification: 'Resend the verification email',
    resendSent: 'Email sent if an account exists — also check your spam folder.',
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/LoginPage.tsx web/src/i18n/fr.ts web/src/i18n/en.ts web/src/pages/AuthPages.test.tsx
git commit -m "feat(web): login surfaces unverified-email state with a resend action"
```

---

### Task 10: Frontend — /verify-email page + route

**Files:**
- Create: `web/src/pages/VerifyEmailPage.tsx`
- Modify: `web/src/App.tsx` (route), `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`
- Test: `web/src/pages/VerifyEmailPage.test.tsx`

**Interfaces:**
- Consumes: `verifyEmail(token)`, `resendVerification(email)` from Task 7.
- Produces: route `/verify-email?token=...` — the link target used in the backend email (Task 5's `BaseURL + "/verify-email?token="`).

- [ ] **Step 1: Write the failing test** — create `web/src/pages/VerifyEmailPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { VerifyEmailPage } from './VerifyEmailPage'
import { LanguageProvider } from '../i18n/LanguageProvider'
import * as api from '../api/client'
import { ApiError } from '../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  vi.restoreAllMocks()
})

function renderAt(url: string) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <LanguageProvider><VerifyEmailPage /></LanguageProvider>
    </MemoryRouter>
  )
}

describe('VerifyEmailPage', () => {
  it('verifies the token and shows success with a login link', async () => {
    const verify = vi.spyOn(api, 'verifyEmail').mockResolvedValue(undefined)
    renderAt('/verify-email?token=tok123')
    await waitFor(() => expect(verify).toHaveBeenCalledWith('tok123'))
    expect(await screen.findByText(/vérifiée/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /connexion|se connecter/i })).toBeInTheDocument()
  })

  it('shows the expired state with a resend form on 410', async () => {
    vi.spyOn(api, 'verifyEmail').mockRejectedValue(new ApiError('lien de vérification invalide ou expiré', 410))
    const resend = vi.spyOn(api, 'resendVerification').mockResolvedValue(undefined)
    renderAt('/verify-email?token=stale')
    expect(await screen.findByText(/invalide ou expiré/i)).toBeInTheDocument()
    await userEvent.type(screen.getByPlaceholderText(/e-mail/i), 'd@x.io')
    await userEvent.click(screen.getByRole('button', { name: /renvoyer/i }))
    await waitFor(() => expect(resend).toHaveBeenCalledWith('d@x.io'))
    expect(await screen.findByText(/envoyé/i)).toBeInTheDocument()
  })

  it('shows the invalid state immediately when no token is present', async () => {
    const verify = vi.spyOn(api, 'verifyEmail')
    renderAt('/verify-email')
    expect(await screen.findByText(/invalide ou expiré/i)).toBeInTheDocument()
    expect(verify).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/VerifyEmailPage.test.tsx`
Expected: FAIL — module `./VerifyEmailPage` not found.

- [ ] **Step 3: Implement** — create `web/src/pages/VerifyEmailPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { useT } from '../i18n/LanguageProvider'
import { verifyEmail, resendVerification } from '../api/client'

type State = 'loading' | 'success' | 'invalid'

export function VerifyEmailPage() {
  const t = useT()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [state, setState] = useState<State>(token ? 'loading' : 'invalid')
  const [email, setEmail] = useState('')
  const [resent, setResent] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    verifyEmail(token)
      .then(() => { if (!cancelled) setState('success') })
      .catch(() => { if (!cancelled) setState('invalid') })
    return () => { cancelled = true }
  }, [token])

  return (
    <AuthShell>
      <div className="m-head">
        {state === 'loading' && <p>{t('auth.verifying')}</p>}
        {state === 'success' && (
          <>
            <h2>{t('auth.verifySuccessTitle')}</h2>
            <p>{t('auth.verifySuccessBody')}</p>
            <p><Link to="/login">{t('auth.login')}</Link></p>
          </>
        )}
        {state === 'invalid' && (
          <>
            <h2>{t('auth.verifyFailedTitle')}</h2>
            <p>{t('auth.verifyFailedBody')}</p>
            {resent ? <p>{t('auth.resendSent')}</p> : (
              <form onSubmit={async e => {
                e.preventDefault()
                try { await resendVerification(email) } catch { /* endpoint always answers 204 */ }
                setResent(true)
              }}>
                <div className="field">
                  <div className="wrap">
                    <input type="email" required placeholder={t('auth.emailPlaceholder')}
                      aria-label={t('auth.emailAriaLabel')}
                      value={email} onChange={e => setEmail(e.target.value)} />
                  </div>
                </div>
                <button type="submit" className="submit"><span className="label">{t('auth.resendVerification')}</span></button>
              </form>
            )}
            <p><Link to="/login">{t('auth.login')}</Link></p>
          </>
        )}
      </div>
    </AuthShell>
  )
}
```

Route in `web/src/App.tsx` (import + after the `/register` route):

```tsx
      <Route path="/verify-email" element={<VerifyEmailPage />} />
```

i18n `fr.ts`:

```ts
    verifying: 'Vérification en cours…',
    verifySuccessTitle: 'Adresse vérifiée',
    verifySuccessBody: 'Votre adresse e-mail est confirmée. Vous pouvez maintenant vous connecter.',
    verifyFailedTitle: 'Lien invalide ou expiré',
    verifyFailedBody: 'Ce lien de vérification est invalide ou expiré. Déjà vérifié ? Connectez-vous simplement. Sinon, demandez un nouveau lien :',
```

`en.ts`:

```ts
    verifying: 'Verifying…',
    verifySuccessTitle: 'Email verified',
    verifySuccessBody: 'Your email address is confirmed. You can now log in.',
    verifyFailedTitle: 'Link invalid or expired',
    verifyFailedBody: 'This verification link is invalid or expired. Already verified? Just log in. Otherwise, request a new link:',
```

- [ ] **Step 4: Run to verify pass**

Run: `cd web && npx tsc --noEmit && npx vitest run src/pages/VerifyEmailPage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/VerifyEmailPage.tsx web/src/pages/VerifyEmailPage.test.tsx web/src/App.tsx web/src/i18n/fr.ts web/src/i18n/en.ts
git commit -m "feat(web): /verify-email page (success, expired+resend, loading)"
```

---

### Task 11: Full-stack verification with Mailpit

**Files:** none (manual/e2e pass; fixes go in the task that owns the broken file)

- [ ] **Step 1: Run the full suites**

Run: `go test ./... 2>&1 | grep -v '^ok\|no test files'; cd web && npx tsc --noEmit && npx vitest run src/`
Expected: everything passes.

- [ ] **Step 2: Rebuild the stack with the flag on**

```bash
# enable the flag for this run
sed -i 's|# REQUIRE_EMAIL_VERIFICATION: "1".*|REQUIRE_EMAIL_VERIFICATION: "1"|' docker-compose.full.yml
docker compose -f docker-compose.yml -f docker-compose.full.yml up -d --build portal web
```

- [ ] **Step 3: Drive the flow end-to-end**

```bash
# 1. register — expect 201 with verificationRequired and NO token
curl -sS -X POST http://localhost:8090/api/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"e2e-verify@test.local","password":"longenough","name":"E2E"}'
# 2. login before verifying — expect 403
curl -sS -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8090/api/auth/login \
  -H 'Content-Type: application/json' -d '{"email":"e2e-verify@test.local","password":"longenough"}'
# 3. pull the token out of the Mailpit API
MSG=$(curl -sS 'http://localhost:8025/api/v1/search?query=to:e2e-verify@test.local' | python3 -c 'import json,sys;print(json.load(sys.stdin)["messages"][0]["ID"])')
TOKEN=$(curl -sS "http://localhost:8025/api/v1/message/$MSG" | python3 -c 'import json,sys,re;print(re.search(r"token=([0-9a-f]+)", json.load(sys.stdin)["Text"]).group(1))')
# 4. verify — expect 204
curl -sS -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8090/api/auth/verify \
  -H 'Content-Type: application/json' -d "{\"token\":\"$TOKEN\"}"
# 5. login again — expect 200 with a JWT
curl -sS -X POST http://localhost:8090/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"e2e-verify@test.local","password":"longenough"}' | head -c 120; echo
```

Also click through the browser flow once: register at http://localhost:8088/register (check-inbox panel), open the link from http://localhost:8025 (success page), log in.

- [ ] **Step 4: Restore the compose default and clean up**

```bash
sed -i 's|^\( *\)REQUIRE_EMAIL_VERIFICATION: "1"|\1# REQUIRE_EMAIL_VERIFICATION: "1"  # new accounts must click the emailed link before login (needs SMTP_*)|' docker-compose.full.yml
docker exec apisix-portal-postgres-1 psql -U portal -d portal -c "DELETE FROM users WHERE email='e2e-verify@test.local'"
git diff --stat  # expect: nothing (compose restored)
```

- [ ] **Step 5: Final commit if anything was fixed during the pass; otherwise done.**
