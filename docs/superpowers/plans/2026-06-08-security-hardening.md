# Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate the code-addressable findings from `docs/security-review.md` (C1, H1, H3, H4, H5, M1, M2, M3, M4, M6, L1, L2, L3) per `docs/superpowers/specs/2026-06-08-security-hardening-design.md`.

**Architecture:** Backend changes are mostly new middleware (rate-limit, security-headers) + tightened validation (config guard, upstream SSRF block, contextPath) + an admin role DB re-check + AES-GCM encryption-at-rest for API keys. Frontend adds 401→logout. Config files bind the admin/DB ports to loopback and tighten `allow_admin`.

**Tech Stack:** Go (chi, pgx, golang-jwt, crypto/aes), React 19 + TS (Vitest), docker-compose, Apache APISIX. Run Go commands from repo root; web commands from `web/`.

**Conventions for every task:** IDE/gopls diagnostics are stale — trust only command output. One commit per task. The docker stack is UP; DB-backed and E2E tests run for real (`RUN_E2E=1`). After Task 8 (encryption-at-rest) the dev DB's pre-existing plaintext credential rows become unreadable — **run `docker compose down -v && docker compose up -d` once after Task 8** so the DB is fresh (dev DBs are disposable per spec). The dev stack must set `PORTAL_ENV=dev` and `UPSTREAM_ALLOW_PRIVATE=1` (added in Tasks 1/7) for existing tests to pass.

---

### Task 1: Config — fail-closed env guard, min JWT secret, credential enc key (H1)

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (append)

- [ ] **Step 1: Write failing tests** — append to `internal/config/config_test.go`:

```go
func TestValidateFailsClosedByDefault(t *testing.T) {
	// Unset env (default "") must be treated as production: built-in dev secrets rejected.
	c := config.Config{Env: "", JWTSecret: config.DevJWTSecret, APISIXAdminKey: "real", CredentialEncKey: "real", DatabaseURL: "x"}
	if err := c.Validate(); err == nil {
		t.Fatal("empty PORTAL_ENV must be production and reject the dev JWT secret")
	}
}

func TestValidateRejectsShortJWTSecretInProd(t *testing.T) {
	c := config.Config{Env: "production", JWTSecret: "short", APISIXAdminKey: "real-key-value", CredentialEncKey: "real-enc-key-value", DatabaseURL: "x"}
	if err := c.Validate(); err == nil {
		t.Fatal("prod must reject a JWT secret shorter than 32 bytes")
	}
}

func TestValidateAllowsDevExplicitly(t *testing.T) {
	c := config.Config{Env: "dev", JWTSecret: config.DevJWTSecret, APISIXAdminKey: config.DevAPISIXAdminKey, CredentialEncKey: config.DevCredentialEncKey}
	if err := c.Validate(); err != nil {
		t.Fatalf("explicit dev env must allow dev secrets: %v", err)
	}
}

func TestValidatePassesProdWithRealSecrets(t *testing.T) {
	c := config.Config{
		Env: "production", DatabaseURL: "postgres://...",
		JWTSecret: "a-very-long-production-jwt-secret-32b+",
		APISIXAdminKey: "rotated-admin-key", CredentialEncKey: "rotated-credential-enc-key-32bytes!!",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("prod with real secrets must pass: %v", err)
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/config/... -count=1` → FAIL (CredentialEncKey field + new behavior missing).

- [ ] **Step 3: Edit `internal/config/config.go`** — apply these exact changes:

Add the dev enc-key constant to the `const` block. The value is the **base64
encoding of 32 bytes** (here, of the ASCII string
`dev-credential-encryption-key-32`) — `CREDENTIAL_ENC_KEY` is always base64 of
32 raw bytes and is base64-decoded by `crypto.New` (Task 8); prod keys come
from `openssl rand -base64 32`:
```go
const (
	DevJWTSecret        = "dev-secret-change-me"
	DevAPISIXAdminKey   = "edd1c9f034335f136f87ad84b625c8f1"
	DevCredentialEncKey = "ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=" // base64(32 bytes)
)
```

Add the field to `Config`:
```go
	AdminEmail       string
	Env              string
	CredentialEncKey string
```

In `Load()` add (after `AdminEmail`):
```go
		CredentialEncKey: get("CREDENTIAL_ENC_KEY", DevCredentialEncKey),
```

Change `isDevLike` so **empty is NOT dev-like** (unset env → production):
```go
func (c Config) isDevLike() bool {
	switch strings.ToLower(strings.TrimSpace(c.Env)) {
	case "dev", "development", "test":
		return true
	default:
		return false
	}
}
```

Extend `UsesDevSecrets`:
```go
func (c Config) UsesDevSecrets() bool {
	return c.JWTSecret == DevJWTSecret || c.APISIXAdminKey == DevAPISIXAdminKey || c.CredentialEncKey == DevCredentialEncKey
}
```

Replace `Validate` body's bad-secret block + add length check:
```go
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
	if c.CredentialEncKey == DevCredentialEncKey {
		bad = append(bad, "CREDENTIAL_ENC_KEY")
	}
	if len(bad) > 0 {
		return fmt.Errorf("refusing to start in %q environment with built-in dev secrets; set %s to secure value(s)", c.Env, strings.Join(bad, ", "))
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes in %q environment", c.Env)
	}
	return nil
}
```

- [ ] **Step 4: Run** `go test ./internal/config/... -count=1` → PASS. Then `go build ./...`.

- [ ] **Step 5: Commit**
```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "fix(config): fail-closed env guard, min JWT secret length, credential enc key (H1)"
```

---

### Task 2: bcrypt cost + password length policy (L3, M6)

**Files:**
- Modify: `internal/auth/user.go`
- Modify: `internal/auth/handler.go` (register length guard message already exists; keep)
- Test: `internal/auth/user_test.go` (append)

- [ ] **Step 1: Write failing tests** — append to `internal/auth/user_test.go`:

```go
func TestHashPasswordRejectsTooLong(t *testing.T) {
	// bcrypt silently truncates at 72 bytes — reject explicitly.
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := auth.HashPassword(string(long)); err == nil {
		t.Fatal("passwords longer than 72 bytes must be rejected")
	}
}

func TestHashPasswordUsesCost12(t *testing.T) {
	h, err := auth.HashPassword("a-normal-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil || cost != 12 {
		t.Fatalf("want cost 12, got %d (err %v)", cost, err)
	}
}
```

Add `"golang.org/x/crypto/bcrypt"` to that test file's imports if not present.

- [ ] **Step 2: Run** `go test ./internal/auth/... -run TestHashPassword -count=1` → FAIL.

- [ ] **Step 3: Edit `internal/auth/user.go`**:

```go
package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong is returned when a password exceeds bcrypt's 72-byte limit
// (bcrypt silently truncates past 72 bytes, so we reject rather than lose entropy).
var ErrPasswordTooLong = errors.New("password must be at most 72 bytes")

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// HashPassword returns a bcrypt hash (cost 12) of the plaintext password.
func HashPassword(plain string) (string, error) {
	if len(plain) > 72 {
		return "", ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 4: Run** `go test ./internal/auth/... -count=1` → PASS (the handler maps a hash error to 500 already; an over-long password now yields 500 — acceptable; the register guard still requires ≥8). `go build ./...`.

- [ ] **Step 5: Commit**
```bash
git add internal/auth/user.go internal/auth/user_test.go
git commit -m "fix(auth): bcrypt cost 12 and reject >72-byte passwords (L3, M6)"
```

---

### Task 3: Login timing + register enumeration neutralization (M3)

**Files:**
- Modify: `internal/auth/handler.go`
- Test: `internal/auth/handler_test.go` (append)

- [ ] **Step 1: Write a failing test** — append to `internal/auth/handler_test.go` (match the file's existing fake-store style; read it first). The behavioral assertion we can make deterministically is that login on an absent user still performs a bcrypt comparison (so timing is equalized). Use a fake store whose GetByEmail returns an error and assert login returns 401 (unchanged) — then assert the dummy-compare path is exercised by checking a package-level hook OR simply assert the response is 401 and a constant-time path ran. Since timing isn't unit-assertable, add a focused test that register returns a neutral 201-or-conflict without leaking which:

```go
func TestLoginAbsentUserStillReturns401(t *testing.T) {
	h := auth.NewHandler(failingStore{}, auth.NewTokenizer("test-secret-at-least-32-bytes-long!!"))
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"nope@x.io","password":"whatever1"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("absent user login: got %d want 401", rr.Code)
	}
}
```

Add a `failingStore` fake in the test file if one doesn't already exist:
```go
type failingStore struct{}
func (failingStore) Create(ctx context.Context, email, hash, name string) (auth.User, error) {
	return auth.User{}, errors.New("nope")
}
func (failingStore) GetByEmail(ctx context.Context, email string) (auth.User, string, error) {
	return auth.User{}, "", errors.New("not found")
}
```
(Imports: `context`, `errors`, `net/http`, `net/http/httptest`, `strings`, `testing`.)

- [ ] **Step 2: Run** `go test ./internal/auth/... -run TestLoginAbsentUser -count=1` → it likely PASSES already (login returns 401). This test pins the behavior; the real change is the timing-equalization code. Proceed to implement and keep it green.

- [ ] **Step 3: Edit `internal/auth/handler.go`** `login` to always run a bcrypt compare:

Add a package-level dummy hash constant near the top (a precomputed bcrypt hash so absent-user logins spend comparable time):
```go
// dummyHash is a valid bcrypt hash compared against when the user is absent, so
// login response time does not reveal whether an account exists (M3).
var dummyHash = "$2a$12$C6UzMDM.H6dfI/f/IKcEeO3Wn7p9pZ9p7Q1q1q1q1q1q1q1q1q1qu"
```
NOTE: generate a REAL cost-12 hash at implement time with `go run` or a quick test (`bcrypt.GenerateFromPassword([]byte("x"), 12)`) and paste it — the placeholder above must be replaced with a genuine hash string or CompareHashAndPassword will error out instantly and defeat the purpose. Verify `bcrypt.Cost([]byte(dummyHash)) == 12`.

Replace the `login` lookup/check block:
```go
	u, hash, err := h.store.GetByEmail(r.Context(), c.Email)
	if err != nil {
		// Equalize timing: run a comparison against a dummy hash so an absent
		// account is indistinguishable from a wrong password.
		CheckPassword(dummyHash, c.Password)
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !CheckPassword(hash, c.Password) {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
```

For register enumeration: the 409 distinct response is a known trade-off (the UI needs to tell users an email is taken). Keep the 409 but ADD a clarifying code comment that this is an accepted, documented trade-off (the timing oracle — the more serious leak — is what we close). Do not change the 409 behavior.

```go
	if errors.Is(err, ErrEmailTaken) {
		// NOTE: a distinct 409 is an intentional UX trade-off (users must learn an
		// email is taken). The timing oracle on login is the closed leak (M3).
		httpx.Error(w, http.StatusConflict, "email already registered")
		return
	}
```

- [ ] **Step 4: Run** `go test ./internal/auth/... -count=1` → PASS. `go build ./...`.

- [ ] **Step 5: Commit**
```bash
git add internal/auth/handler.go internal/auth/handler_test.go
git commit -m "fix(auth): equalize login timing for absent accounts (M3)"
```

---

### Task 4: RequireAdmin re-checks role against the DB (H5)

**Files:**
- Modify: `internal/auth/repo.go` (add GetRole)
- Modify: `internal/auth/middleware.go` (RequireAdmin takes a role lookup)
- Modify: `internal/server/server.go` (wire the lookup)
- Modify: `internal/auth/middleware_test.go` (update RequireAdmin call sites)
- Test: `internal/auth/middleware_test.go` (add the demoted-admin case)

- [ ] **Step 1: Add `GetRole` to `internal/auth/repo.go`**:
```go
// GetRole returns the current role of the user with the given id.
func (r *Repo) GetRole(ctx context.Context, userID int64) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, userID).Scan(&role)
	return role, err
}
```

- [ ] **Step 2: Write the failing test** — in `internal/auth/middleware_test.go`, add (and update existing `RequireAdmin(tk)` call sites to the new signature `RequireAdmin(tk, lookup)`):

```go
func TestRequireAdminRechecksDBRole(t *testing.T) {
	tk := auth.NewTokenizer("test-secret-at-least-32-bytes-long!!")
	// token CLAIMS admin, but the DB now says developer (demoted)
	tokStr, _ := tk.Issue(7, "a@b.c", "admin")
	lookup := func(ctx context.Context, id int64) (string, error) { return "developer", nil }
	called := false
	h := auth.RequireAdmin(tk, lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokStr)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("demoted admin must get 403 and not reach handler; code=%d called=%v", rr.Code, called)
	}
}

func TestRequireAdminLookupErrorIs500(t *testing.T) {
	tk := auth.NewTokenizer("test-secret-at-least-32-bytes-long!!")
	tokStr, _ := tk.Issue(7, "a@b.c", "admin")
	lookup := func(ctx context.Context, id int64) (string, error) { return "", errors.New("db down") }
	h := auth.RequireAdmin(tk, lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tokStr)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("lookup failure must be 500 (could not verify), got %d", rr.Code)
	}
}
```

- [ ] **Step 3: Run** `go test ./internal/auth/... -run TestRequireAdmin -count=1` → FAIL (signature mismatch).

- [ ] **Step 4: Edit `internal/auth/middleware.go`** — change `RequireAdmin` to take a lookup and authorize on the DB role:

```go
// RoleLookup returns the current role for a user id (satisfied by *Repo.GetRole).
type RoleLookup func(ctx context.Context, userID int64) (string, error)

// RequireAdmin requires a valid Bearer JWT AND that the user's CURRENT role in
// the database is "admin" (the token claim alone is not trusted, so a demoted
// admin loses access immediately rather than for the token's lifetime). H5.
func RequireAdmin(tk *Tokenizer, lookup RoleLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tk.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			role, err := lookup(r.Context(), claims.UserID)
			if err != nil {
				// A failed lookup (DB outage) is "could not verify", not "admin only".
				httpx.Error(w, http.StatusInternalServerError, "could not verify role")
				return
			}
			if role != "admin" {
				httpx.Error(w, http.StatusForbidden, "admin only")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			ctx = WithRole(ctx, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 5: Wire it in `internal/server/server.go`** — change:
```go
	requireAdmin := auth.RequireAdmin(tok)
```
to:
```go
	requireAdmin := auth.RequireAdmin(tok, authRepo.GetRole)
```
(`authRepo` already exists in `New`.)

- [ ] **Step 6: Run** `go test ./internal/auth/... ./internal/... -count=1` (update any other `RequireAdmin(tk)` call site the compiler flags). `go build ./...`. Then the E2E authz test must still pass: `RUN_E2E=1 go test ./internal/e2e/... -run TestAuthzNegatives -count=1` (admin token is a real admin in the DB → still 403 for non-admin, 200/expected for admin).

- [ ] **Step 7: Commit**
```bash
git add internal/auth/repo.go internal/auth/middleware.go internal/auth/middleware_test.go internal/server/server.go
git commit -m "fix(auth): RequireAdmin re-checks role against DB, not just the JWT claim (H5)"
```

---

### Task 5: Per-IP rate limiter on auth endpoints (H3)

**Files:**
- Create: `internal/httpx/ratelimit.go`
- Create: `internal/httpx/ratelimit_test.go`
- Modify: `internal/server/server.go` (wrap `/api/auth/`)

No new dependency — a small in-memory token-bucket keyed by client IP.

- [ ] **Step 1: Write the failing test** — `internal/httpx/ratelimit_test.go`:
```go
package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/httpx"
)

func TestRateLimiterAllowsBurstThen429s(t *testing.T) {
	rl := httpx.NewRateLimiter(3, 0) // burst 3, no refill during the test
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	do := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	for i := 0; i < 3; i++ {
		if c := do(); c != 200 {
			t.Fatalf("burst call %d: got %d want 200", i, c)
		}
	}
	if c := do(); c != http.StatusTooManyRequests {
		t.Fatalf("over-burst: got %d want 429", c)
	}
}

func TestRateLimiter429SetsRetryAfter(t *testing.T) {
	rl := httpx.NewRateLimiter(1, 0.5)
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.RemoteAddr = "1.2.3.4:5555"
	h.ServeHTTP(httptest.NewRecorder(), req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") == "" {
		t.Fatalf("429 must carry Retry-After; code=%d header=%q", rr.Code, rr.Header().Get("Retry-After"))
	}
}

func TestRateLimiterEvictsStaleBuckets(t *testing.T) {
	rl := httpx.NewRateLimiter(5, 1)
	now := time.Unix(0, 0)
	rl.SetNow(func() time.Time { return now })
	for i := 0; i < 100; i++ {
		rl.Allow(fmt.Sprintf("ip-%d", i))
	}
	// Advance past the idle TTL and trigger a sweep with fresh traffic.
	now = now.Add(time.Hour)
	rl.Allow("fresh")
	if n := rl.Len(); n > 1 {
		t.Fatalf("stale buckets must be evicted on sweep; %d remain", n)
	}
}

func TestRateLimiterIsolatesByIP(t *testing.T) {
	rl := httpx.NewRateLimiter(1, 0)
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	call := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.RemoteAddr = ip + ":1"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if call("1.1.1.1") != 200 || call("2.2.2.2") != 200 {
		t.Fatal("first call per distinct IP must pass")
	}
	if call("1.1.1.1") != http.StatusTooManyRequests {
		t.Fatal("second call from same IP must 429")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/httpx/... -count=1` → FAIL (no RateLimiter).

- [ ] **Step 3: Create `internal/httpx/ratelimit.go`** (the test file needs
`fmt` and `time` in its imports for the eviction test):
```go
package httpx

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	sweepEvery = time.Minute      // how often Allow scans for stale buckets
	idleTTL    = 10 * time.Minute // buckets idle this long are dropped
)

// RateLimiter is a simple in-memory token bucket keyed by an arbitrary string
// (client IP for the middleware, lowercased email for the login handler). It
// is process-local (fine for a single-node portal; a distributed deploy needs
// shared state). Stale buckets are swept periodically so unique keys cannot
// grow memory without bound.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	burst      float64
	refill     float64 // tokens per second
	retryAfter string  // seconds, precomputed for the 429 header
	now        func() time.Time
	lastSweep  time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter creates a limiter allowing `burst` requests immediately, then
// refilling at `refillPerSec` tokens/second per key.
func NewRateLimiter(burst, refillPerSec float64) *RateLimiter {
	retry := 1
	if refillPerSec > 0 {
		retry = int(math.Ceil(1 / refillPerSec))
	}
	return &RateLimiter{
		buckets:    make(map[string]*bucket),
		burst:      burst,
		refill:     refillPerSec,
		retryAfter: strconv.Itoa(retry),
		now:        time.Now,
	}
}

// SetNow overrides the limiter's clock. Test hook only.
func (rl *RateLimiter) SetNow(fn func() time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.now = fn
}

// Len reports the number of tracked buckets. Test hook only.
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Allow reports whether a request for key may proceed, consuming a token.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	if now.Sub(rl.lastSweep) >= sweepEvery {
		for k, b := range rl.buckets {
			if now.Sub(b.last) >= idleTTL {
				delete(rl.buckets, k)
			}
		}
		rl.lastSweep = now
	}
	b := rl.buckets[key]
	if b == nil {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	} else {
		b.tokens += rl.refill * now.Sub(b.last).Seconds()
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns an http middleware that 429s requests over the per-IP limit.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.Allow(clientIP(r)) {
				w.Header().Set("Retry-After", rl.retryAfter)
				Error(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```
(The earlier test snippets call the exported `Allow`/`SetNow`/`Len`; the first
two tests use `rl.Middleware()` unchanged.)

- [ ] **Step 4: Run** `go test ./internal/httpx/... -count=1` → PASS.

- [ ] **Step 5: Wire into `internal/server/server.go`** — create a limiter and wrap the auth handler mount. Replace:
```go
	mux.Handle("/api/auth/", authH)
```
with:
```go
	authLimiter := httpx.NewRateLimiter(10, 0.5) // 10 burst, refill 1 every 2s, per IP
	mux.Handle("/api/auth/", authLimiter.Middleware()(authH))
```
Add `"apisix-portal/internal/httpx"` to server.go imports if not present. NOTE: burst 10 is high enough that the E2E's sequential auth calls (a handful per test) pass; confirm in Step 6.

- [ ] **Step 5b: Per-account bucket on login** — per-IP alone doesn't stop
distributed stuffing of one account. Give the auth `Handler` a second limiter
keyed by lowercased email:
  - Change `auth.NewHandler(store, tk)` to `auth.NewHandler(store, tk, loginLimiter *httpx.RateLimiter)` (nil = disabled, so existing unit tests can pass nil; update the handler-test call sites the compiler flags).
  - In `login`, after decoding the body and before the store lookup:
```go
	if h.loginLimiter != nil && !h.loginLimiter.Allow(strings.ToLower(c.Email)) {
		w.Header().Set("Retry-After", "2")
		httpx.Error(w, http.StatusTooManyRequests, "too many attempts")
		return
	}
```
  - In `server.New`, construct it with `auth.NewHandler(authRepo, tok, httpx.NewRateLimiter(10, 0.5))`.
  - Add a unit test: 11 sequential logins for the same email from rotating `RemoteAddr`s → the 11th gets 429 even though each IP is fresh.

- [ ] **Step 6: Run** `go build ./...`, `go test ./internal/... -count=1`, then `RUN_E2E=1 go test ./internal/e2e/... -count=1` → all pass (E2E auth calls stay under the burst).

- [ ] **Step 7: Commit**
```bash
git add internal/httpx/ratelimit.go internal/httpx/ratelimit_test.go internal/server/server.go
git commit -m "fix(server): per-IP rate limiter on auth endpoints (H3)"
```

---

### Task 6: Security-headers middleware (M2)

**Files:**
- Create: `internal/httpx/headers.go`
- Create: `internal/httpx/headers_test.go`
- Modify: `internal/server/server.go` (wrap the whole mux)

- [ ] **Step 1: Write the failing test** — `internal/httpx/headers_test.go`:
```go
package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/httpx"
)

func TestSecurityHeadersSet(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rr.Header().Get(k); got != v {
			t.Fatalf("%s: got %q want %q", k, got, v)
		}
	}
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("CSP header must be set")
	}
}

func TestAPIResponsesAreNoStore(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/applications/1/credentials", nil))
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("/api/ responses must be no-store (they can carry live keys), got %q", got)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Header().Get("Cache-Control") != "" {
		t.Fatal("non-API paths must not be forced no-store")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/httpx/... -run TestSecurityHeaders -count=1` → FAIL.

- [ ] **Step 3: Create `internal/httpx/headers.go`**:
```go
package httpx

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets baseline security response headers on every response.
// CSP is tuned for the SPA: same-origin by default, the app's Google Fonts
// origins allowed, framing denied. style-src keeps 'unsafe-inline' because the
// React app uses inline styles — a deliberate CSP weakening. HSTS is
// intentionally omitted here (added at the TLS-terminating proxy in
// production). /api/ responses are marked no-store: the credentials endpoint
// returns the live API key over GET and must never land in an HTTP cache.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run** `go test ./internal/httpx/... -count=1` → PASS.

- [ ] **Step 5: Wrap the mux in `internal/server/server.go`** — change the return:
```go
	return logRequests(mux)
```
to:
```go
	return httpx.SecurityHeaders(logRequests(mux))
```

- [ ] **Step 6: Run** `go build ./...`, `go test ./internal/... -count=1`, `RUN_E2E=1 go test ./internal/e2e/... -count=1` → all pass.

- [ ] **Step 7: Commit**
```bash
git add internal/httpx/headers.go internal/httpx/headers_test.go internal/server/server.go
git commit -m "fix(server): baseline security headers + CSP (M2)"
```

---

### Task 7: SSRF block in validUpstream + contextPath validation/overlap (H4, M1)

**Files:**
- Modify: `internal/admin/product.go`
- Test: `internal/admin/product_test.go` (append)

The overlap check (M1) requires DB state; do the **format** validation in the pure `validate()` (unit-tested here) and the **uniqueness/overlap** check in the service/repo layer where the DB is available. This task covers format validation + SSRF (both pure); the overlap-uniqueness is enforced via a DB query in the same task's service edit.

- [ ] **Step 1: Write failing tests** — append to `internal/admin/product_test.go`
(the file is package `admin`, so the unexported `lookupIP` hook is stubbable
directly; restore it with `t.Cleanup`):
```go
func stubResolver(t *testing.T, table map[string][]net.IP) {
	t.Helper()
	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if ips, ok := table[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupIP = orig })
}

func TestValidUpstreamBlocksPrivateByDefault(t *testing.T) {
	// allowPrivate=false (production): loopback/link-local/RFC1918 rejected.
	for _, h := range []string{"127.0.0.1:80", "[::1]:80", "169.254.169.254:80", "10.0.0.5:8080", "192.168.1.1:9000", "localhost:8080"} {
		if ValidUpstream(h, false) {
			t.Fatalf("%s must be rejected when private targets are blocked", h)
		}
	}
}

func TestValidUpstreamResolvesHostnames(t *testing.T) {
	stubResolver(t, map[string][]net.IP{
		"api.example.com": {net.ParseIP("93.184.216.34")},
		"evil.example":    {net.ParseIP("203.0.113.7"), net.ParseIP("169.254.169.254")},
		"127.1":           {net.ParseIP("127.0.0.1")}, // libc shorthand for loopback
	})
	if !ValidUpstream("api.example.com:443", false) {
		t.Fatal("public host resolving to public IPs must be allowed")
	}
	// ANY private resolved address rejects the host (SSRF via attacker DNS).
	if ValidUpstream("evil.example:80", false) {
		t.Fatal("host with a private resolved address must be rejected")
	}
	// Shorthand IPs are not IP literals to ParseIP but resolve to loopback.
	if ValidUpstream("127.1:80", false) {
		t.Fatal("shorthand loopback (127.1) must be rejected")
	}
	// Unresolvable hosts are rejected (fail closed).
	if ValidUpstream("nonexistent.example.com:80", false) {
		t.Fatal("unresolvable host must be rejected")
	}
}

func TestValidUpstreamAllowsPrivateWithFlag(t *testing.T) {
	if !ValidUpstream("echo:8080", true) {
		t.Fatal("dev flag must allow internal docker hosts like echo:8080")
	}
}

func TestValidContextPath(t *testing.T) {
	ok := []string{"/orders", "/v1/orders", "/a-b_c"}
	bad := []string{"orders", "/orders/*", "/orders ", "/", "//x", "/a;b"}
	for _, p := range ok {
		if !ValidContextPath(p) {
			t.Fatalf("%q should be valid", p)
		}
	}
	for _, p := range bad {
		if ValidContextPath(p) {
			t.Fatalf("%q should be invalid", p)
		}
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/admin/... -run "TestValid" -count=1` → FAIL.

- [ ] **Step 3: Edit `internal/admin/product.go`** — replace `validUpstream` with an exported `ValidUpstream(s string, allowPrivate bool)`, add `ValidContextPath`, and thread an `allowPrivate` into `validate`:

```go
package admin

import (
	"net"
	"regexp"
	"strings"
)

// ... Product struct unchanged ...

var ctxPathRe = regexp.MustCompile(`^/[A-Za-z0-9](?:[A-Za-z0-9/_-]*[A-Za-z0-9])?$`)

// ValidContextPath enforces a safe route prefix: must start with "/", only
// alnum/_/-//, no spaces or wildcards, no trailing slash, not bare "/" (M1).
func ValidContextPath(p string) bool {
	if p == "/" || strings.Contains(p, "//") {
		return false
	}
	return ctxPathRe.MatchString(p)
}

// lookupIP resolves a hostname at validation time. Overridden in tests.
var lookupIP = net.LookupIP

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified()
}

// ValidUpstream checks host:port shape and, unless allowPrivate is set, rejects
// targets in loopback / link-local / private ranges to prevent SSRF (H4).
// Hostnames are resolved here and rejected if ANY resolved address is private —
// this catches libc shorthand IPs ("127.1") and attacker domains pointing at
// internal ranges. Residual risk: DNS can change between validation and
// proxying (rebinding); the long-term fix is an operator allow-list. The dev
// stack sets allowPrivate so docker-internal hosts (echo:8080) work.
func ValidUpstream(s string, allowPrivate bool) bool {
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" || port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	if allowPrivate {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	// IP literal (SplitHostPort already unbracketed IPv6).
	if ip := net.ParseIP(host); ip != nil {
		return !isPrivateIP(ip)
	}
	// A hostname with no dot is an internal/docker name → block.
	if !strings.Contains(host, ".") {
		return false
	}
	// Resolve and fail closed: unresolvable or any-private → reject.
	ips, err := lookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return false
		}
	}
	return true
}

func (p Product) validate(allowPrivate bool) string {
	if strings.TrimSpace(p.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(p.Slug) == "" {
		return "slug is required"
	}
	if strings.TrimSpace(p.Category) == "" {
		return "category is required"
	}
	if !ValidContextPath(p.ContextPath) {
		return "contextPath must look like /path (alphanumerics, -, _, /, no wildcard)"
	}
	if p.UpstreamURL != "" && !ValidUpstream(p.UpstreamURL, allowPrivate) {
		return "upstreamUrl must be host:port and not target a private/internal address"
	}
	return ""
}
```

- [ ] **Step 4: Update `validate()` call sites.** Grep `grep -rn "\.validate()" internal/admin/`. The service/handler that calls `p.validate()` must pass `allowPrivate`. Thread an `allowPrivate bool` field into the admin `Service` (set from `os.Getenv("UPSTREAM_ALLOW_PRIVATE") == "1"` at construction in `internal/server/server.go` where `admin.NewService` is called — read its signature and add the parameter, OR read the env directly inside the service). Minimal approach: read the env inside `validate`'s caller. Implementer picks the cleanest; the env var name is `UPSTREAM_ALLOW_PRIVATE`. Update `admin.NewService`/handler accordingly and adjust `internal/admin/*_test.go` call sites that construct the service or call validate.

- [ ] **Step 5: contextPath overlap (M1, DB).** Two layers — a unique index for exact duplicates (race-proof) and a prefix-overlap query (check-then-insert; low residual race on an admin-only surface, acceptable):
  - **New migration** `internal/db/migrations/0007_context_path_unique.sql`:
```sql
CREATE UNIQUE INDEX IF NOT EXISTS api_products_context_path_key ON api_products (context_path);
```
    (Seed paths in `0002_seed.sql` are already distinct, so this applies cleanly.)
  - **Repo:** add `ErrContextPathTaken`. The existing `isUniqueViolation` (pg code 23505) currently maps every unique violation to `ErrSlugTaken` — disambiguate via `pgErr.ConstraintName`: `api_products_context_path_key` → `ErrContextPathTaken`, otherwise `ErrSlugTaken`. Add the **prefix-overlap** check, called by the service before create/update:
```go
// ContextPathOverlaps reports whether p would collide with an existing
// product's route prefix: equal, or a path-prefix on a "/" boundary in either
// direction (/v1 vs /v1/orders — APISIX's /v1/* shadows /v1/orders/*).
func (r *Repo) ContextPathOverlaps(ctx context.Context, p string, exceptID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM api_products
		   WHERE id <> $2
		     AND (context_path = $1
		          OR context_path LIKE $1 || '/%'
		          OR $1 LIKE context_path || '/%'))`,
		p, exceptID).Scan(&exists)
	return exists, err
}
```
  - **Service:** create/update call `ContextPathOverlaps` (exceptID 0 on create) and return `ErrContextPathTaken` on overlap — mirror the `ErrSlugTaken` flow exactly. **Handler:** map it to 409 `"contextPath conflicts with an existing product"`.
  - Tests: handler test with a fake service returning `ErrContextPathTaken` → 409 (mirror `TestUpdateSlugTakenReturns409`); a DB-backed repo test (if the repo has one for slugs, mirror it) asserting `/v1` vs `/v1/orders` overlaps in both directions and `/v1` vs `/v1beta` does NOT (the `/` boundary matters).

- [ ] **Step 6: Run** `go build ./...`, `go test ./internal/admin/... -count=1`, then with the dev flag set `UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1` → lifecycle still passes (echo:8080 allowed via flag; contextPath `/e2e_<uniq>` is valid).

- [ ] **Step 7: Commit**
```bash
git add internal/admin/ internal/db/migrations/0007_context_path_unique.sql
git commit -m "fix(admin): block SSRF upstreams + validate/dedupe contextPath (H4, M1)"
```

---

### Task 8: crypto/rand error + encryption-at-rest for API keys (L1, L2)

**Files:**
- Create: `internal/crypto/aesgcm.go`
- Create: `internal/crypto/aesgcm_test.go`
- Modify: `internal/subscriptions/repo.go` (GenerateKey error + encrypt/decrypt)
- Modify: `internal/subscriptions/service.go` / `internal/server/server.go` (pass the enc key to the repo)

- [ ] **Step 1: Write the failing crypto test** — `internal/crypto/aesgcm_test.go`:
```go
package crypto_test

import (
	"strings"
	"testing"

	"apisix-portal/internal/crypto"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// The key is base64 of 32 raw bytes (config.DevCredentialEncKey has the
	// same shape).
	c, err := crypto.New("ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ct, err := c.Encrypt("ax_live_secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == "ax_live_secret" || ct == "" {
		t.Fatal("ciphertext must differ from plaintext and be non-empty")
	}
	if !strings.HasPrefix(ct, "v1:") {
		t.Fatalf("ciphertext must carry the v1: version prefix for future key rotation, got %q", ct)
	}
	pt, err := c.Decrypt(ct)
	if err != nil || pt != "ax_live_secret" {
		t.Fatalf("decrypt: got %q err %v", pt, err)
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	// Not base64 at all.
	if _, err := crypto.New("not-base64!!!"); err == nil {
		t.Fatal("non-base64 key must be rejected")
	}
	// Valid base64 but not 32 decoded bytes.
	if _, err := crypto.New("dG9vLXNob3J0"); err == nil { // base64("too-short")
		t.Fatal("key that does not decode to 32 bytes must be rejected")
	}
}

func TestDecryptRejectsUnversionedCiphertext(t *testing.T) {
	c, _ := crypto.New("ZGV2LWNyZWRlbnRpYWwtZW5jcnlwdGlvbi1rZXktMzI=")
	if _, err := c.Decrypt("AAAAAAAAAAAAAAAAAAAAAAAAAAAA"); err == nil {
		t.Fatal("ciphertext without the v1: prefix must be rejected")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/crypto/... -count=1` → FAIL.

- [ ] **Step 3: Create `internal/crypto/aesgcm.go`**:
```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// Cipher encrypts/decrypts short secrets (API keys) with AES-256-GCM. The key
// is supplied as base64 of 32 raw bytes (a raw ASCII passphrase would carry
// far less than 256 bits of entropy). Ciphertext is "v1:" + base64(nonce||ct);
// the version prefix lets a future key rotation re-encrypt v1 rows in place
// instead of forcing a DB wipe.
type Cipher struct{ aead cipher.AEAD }

// v1Prefix tags the ciphertext format/key version.
const v1Prefix = "v1:"

// New builds a Cipher from a base64-encoded 32-byte key
// (generate with: openssl rand -base64 32).
func New(b64Key string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, errors.New("credential encryption key must be base64 (of 32 raw bytes)")
	}
	if len(key) != 32 {
		return nil, errors.New("credential encryption key must decode to exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns "v1:" + base64(nonce || ciphertext).
func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return v1Prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. Ciphertext without a known version prefix is
// rejected (it is either corrupt or a pre-encryption plaintext row).
func (c *Cipher) Decrypt(enc string) (string, error) {
	b64, ok := strings.CutPrefix(enc, v1Prefix)
	if !ok {
		return "", errors.New("ciphertext missing version prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	pt, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
```

- [ ] **Step 4: Run** `go test ./internal/crypto/... -count=1` → PASS.

- [ ] **Step 5: Fix GenerateKey (L1) + wire encryption into the repo (L2).**

In `internal/subscriptions/repo.go`:
- Make `GenerateKey` panic on entropy failure (callers can't recover; the genKey type stays `func() string`):
```go
// GenerateKey returns a random 32-hex-char API key. It panics if the system
// CSPRNG fails (an unrecoverable entropy outage) rather than emitting a
// predictable all-zero key.
func GenerateKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("subscriptions: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
```
- Give `Repo` a cipher and encrypt on write / decrypt on read. Change `NewRepo` to accept a `*crypto.Cipher`:
```go
type Repo struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

func NewRepo(pool *pgxpool.Pool, cipher *crypto.Cipher) *Repo { return &Repo{pool: pool, cipher: cipher} }
```
- In `GetOrCreateCredential`: encrypt `want.APIKey` before the INSERT, and decrypt the `api_key` column from `RETURNING` before returning. (The upsert RETURNING yields the stored ciphertext — decrypt it so the service/handler always sees plaintext.)
- In `GetCredential` and `ConsumersForPlan`: decrypt the `api_key` column after scanning.
Add `"apisix-portal/internal/crypto"` to the repo imports.

NOTE the upsert subtlety: `want.APIKey` is `genKey()` (plaintext) → encrypt to `encKey` → INSERT `encKey`; on conflict the existing ciphertext is RETURNED → decrypt it. So:
```go
	plain := genKey()
	encKey, err := r.cipher.Encrypt(plain)
	if err != nil {
		return Credential{}, err
	}
	var stored string
	err = r.pool.QueryRow(ctx,
		`INSERT INTO credentials(application_id, api_key, consumer_username) VALUES($1,$2,$3)
		 ON CONFLICT (application_id) DO UPDATE SET application_id = credentials.application_id
		 RETURNING application_id, api_key, consumer_username`,
		appID, encKey, consumerName(appID),
	).Scan(&c.ApplicationID, &stored, &c.ConsumerUsername)
	if err != nil {
		return Credential{}, err
	}
	c.APIKey, err = r.cipher.Decrypt(stored)
	if err != nil {
		return Credential{}, err
	}
	return c, nil
```
Apply the analogous decrypt to `GetCredential` (single row) and `ConsumersForPlan` (loop). The `genKey func() string` parameter stays unchanged.

- [ ] **Step 6: Wire the cipher where the repo is constructed.** In `internal/server/server.go` `New`, build the cipher from `cfg.CredentialEncKey` and pass it to `subscriptions.NewRepo`:
```go
	cipher, err := crypto.New(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("credential cipher: %v", err)
	}
	subRepo := subscriptions.NewRepo(pool, cipher)
```
Add `"apisix-portal/internal/crypto"` import. (`New` returns `http.Handler`; a fatal on a bad key at startup is acceptable — the dev key decodes to 32 bytes.) Update any other `subscriptions.NewRepo(pool)` call sites (tests) to pass a cipher built from `crypto.New(config.DevCredentialEncKey)`.

- [ ] **Step 7: Reset the dev DB and run everything.** Because pre-existing credential rows are plaintext and will fail to decrypt:
```bash
docker compose down -v && docker compose up -d && sleep 12
```
Then: `go build ./...`, `go test ./internal/... -count=1`, and
`UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1` → all pass (fresh apps get encrypted keys; the gateway still receives the decrypted plaintext key, so 200/429 still hold).

- [ ] **Step 8: Commit**
```bash
git add internal/crypto/ internal/subscriptions/repo.go internal/server/server.go internal/subscriptions/*_test.go
git commit -m "fix(credentials): encrypt API keys at rest + propagate crypto/rand failure (L2, L1)"
```

---

### Task 9: Lock down admin & DB ports; tighten allow_admin (C1, M5 dev)

**Files:**
- Modify: `docker-compose.yml`
- Modify: `deploy/apisix/config.yaml`

- [ ] **Step 1: Edit `docker-compose.yml`** — bind published ports to loopback so they are not reachable from other hosts on the network:
  - postgres: `ports: ["127.0.0.1:5432:5432"]`
  - apisix admin: change `"19180:9180"` to `"127.0.0.1:19180:9180"` (leave the gateway `9080` as-is, or also bind to loopback if you only test locally — keep `9080` published normally so the gateway is reachable).
  - Also add `PORTAL_ENV=dev` and `UPSTREAM_ALLOW_PRIVATE=1` to the portal service's environment IF the portal runs in compose (check whether docker-compose runs the portal; in this repo the portal is run via `go run`/`make run` on the host, so instead these env vars are set by the developer/E2E — document in docs/testing.md and ensure `make run` / the E2E set them).

- [ ] **Step 2: Edit `deploy/apisix/config.yaml`** — restrict the admin allow-list. Replace:
```yaml
    allow_admin:
      - 0.0.0.0/0
```
with:
```yaml
    allow_admin:
      - 127.0.0.0/8
      - 172.16.0.0/12
      - 192.168.0.0/16
      - 10.0.0.0/8
```
(127/8 for loopback; the RFC1918 ranges cover the Docker bridge so the portal container/host-published-loopback path still reaches the admin API. This removes the literal `0.0.0.0/0` any-source rule.)

- [ ] **Step 3: Restart the stack and verify the admin API is still reachable from the host loopback but the config no longer says 0.0.0.0/0**:
```bash
docker compose down && docker compose up -d && sleep 12
curl -s -o /dev/null -w "%{http_code}\n" -H "X-API-KEY: edd1c9f034335f136f87ad84b625c8f1" http://localhost:19180/apisix/admin/routes   # expect 200
grep -c "0.0.0.0/0" deploy/apisix/config.yaml   # expect 0
```
Then re-run the E2E to confirm provisioning still works:
`UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1` → pass.

- [ ] **Step 4: Commit**
```bash
git add docker-compose.yml deploy/apisix/config.yaml
git commit -m "fix(stack): bind admin/DB ports to loopback, restrict allow_admin (C1, M5)"
```

---

### Task 10: Frontend 401 → logout (M4)

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/auth/AuthProvider.tsx` (if needed for a logout hook)
- Test: `web/src/api/client.test.ts` (append)

- [ ] **Step 1: Read** `web/src/api/client.ts` and `web/src/auth/AuthProvider.tsx` to see how the token is stored/cleared (localStorage keys `token`/`user`). The cleanest cross-cutting hook: in `parse`/`sendAuthed`, on `res.status === 401`, clear `localStorage` auth keys and redirect to `/login`.

- [ ] **Step 2: Write the failing test** — append to `web/src/api/client.test.ts` (match its fetch-mock idiom):
```ts
describe('401 handling', () => {
  it('clears stored auth and redirects to /login on 401', async () => {
    localStorage.setItem('token', 'jwt'); localStorage.setItem('user', '{}')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":"invalid token"}', { status: 401 })))
    // jsdom: capture location assignment
    const orig = window.location
    // @ts-expect-error redefine for test
    delete window.location
    // @ts-expect-error minimal stub
    window.location = { ...orig, assign: vi.fn(), href: '' }
    await expect(adminGetProducts('jwt')).rejects.toBeTruthy()
    expect(localStorage.getItem('token')).toBeNull()
  })
})
```
(Adjust to the file's actual import list / mock helper. The key assertions: token cleared on 401.)

- [ ] **Step 3: Run** `cd web && npx vitest run src/api/client.test.ts` → FAIL.

- [ ] **Step 4: Edit `web/src/api/client.ts`** — add a 401 handler used by `parse` and `sendAuthed`:
```ts
function handle401(status: number) {
  if (status === 401 && typeof window !== 'undefined') {
    try { localStorage.removeItem('token'); localStorage.removeItem('user') } catch { /* ignore */ }
    if (window.location.pathname !== '/login') window.location.href = '/login'
  }
}
```
Call `handle401(res.status)` inside `parse` (before throwing on `!res.ok`) and inside `sendAuthed`'s non-ok branch. Keep throwing `ApiError` so existing call sites still see an error.

- [ ] **Step 5: Run** `cd web && npx vitest run` → all pass (expected count + the new test); `npx tsc -b --force` clean.

- [ ] **Step 6: Commit**
```bash
git add web/src/api/client.ts web/src/api/client.test.ts
git commit -m "fix(web): clear auth and redirect to /login on 401 (M4)"
```

---

### Task 11: Update the review checklist + full verification

**Files:**
- Modify: `docs/security-review.md` (flip the fixed items to ☑ and note encrypt-at-rest for L2)
- Modify: `docs/testing.md` (note the required env vars: `PORTAL_ENV=dev`, `UPSTREAM_ALLOW_PRIVATE=1` for local E2E)

- [ ] **Step 1: Update the remediation checklist** in `docs/security-review.md` — mark C1, H1, H3, H4, H5, M1, M2, M3, M4, M6, L1, L2, L3 as done (☑), leave H2/M5/L4 as documented/open, and add a one-line note under L2 that it was implemented as AES-GCM encryption-at-rest (not hashing) to preserve key display + re-provisioning.

- [ ] **Step 2: Update `docs/testing.md`** — add that local E2E now requires `PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1` and that the dev DB must be recreated (`docker compose down -v`) after enabling credential encryption.

- [ ] **Step 3: Full gates** (stack up, dev DB fresh from Task 8/9):
```bash
go vet ./... && go test ./internal/... ./cmd/... -count=1
UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1
cd web && npx vitest run && npx tsc -b --force && npm run build
```
All green.

- [ ] **Step 4: Commit**
```bash
git add docs/security-review.md docs/testing.md
git commit -m "docs: mark remediated findings; document E2E env vars (security hardening)"
```

---

## Self-review notes (already applied)

- Spec coverage: H1→T1, L3/M6→T2, M3→T3, H5→T4, H3→T5, M2→T6, H4/M1→T7, L1/L2→T8, C1/M5→T9, M4→T10, docs→T11. H2/M5(prod)/L4 remain documented per spec.
- The `RequireAdmin` signature change (T4) ripples to server.New and middleware tests — the task calls that out and updates call sites.
- `subscriptions.NewRepo` signature change (T8) ripples to all constructors/tests — the task calls that out; the `genKey func() string` type is deliberately kept to avoid wider churn (L1 panics instead of returning an error).
- Dev-stack escape hatches are explicit: `PORTAL_ENV=dev` (T1 env default is now prod) and `UPSTREAM_ALLOW_PRIVATE=1` (T7) must be set for local E2E — documented in T9/T11; the E2E commands in every task include them from T7 onward.
- Determinism/E2E: the dev DB must be recreated after T8 (plaintext→encrypted); rate-limit burst (10) is above the E2E's per-IP auth call count; loopback admin binding (T9) keeps `localhost:19180` reachable for the harness.
- The dummy bcrypt hash in T3 must be a REAL cost-12 hash (the task flags this explicitly — a placeholder would break timing-equalization).

### 2026-06-10 spec-review amendments (applied above, keep in sync with the spec)
- **T4:** `RequireAdmin` lookup failure → 500 (could not verify), not 403; extra test.
- **T5:** limiter buckets are swept (idle > 10 min, checked once a minute) so unique IPs can't grow memory unboundedly; 429s carry `Retry-After`; Step 5b adds a per-account (lowercased-email) bucket inside `login` — `auth.NewHandler` gains a third param (nil = disabled).
- **T6:** `/api/` responses get `Cache-Control: no-store` (credentials GET returns the live key); CSP's `'unsafe-inline'` style-src is noted as a deliberate weakening.
- **T7:** `ValidUpstream` uses `net.SplitHostPort` (IPv6 literals) and resolves hostnames via an overridable `lookupIP` (closes `127.1`-style shorthand and attacker-DNS bypasses; fail closed on unresolvable). Residual DNS-rebinding TOCTOU documented. contextPath gets migration `0007` (unique index, race-proof exact dups; disambiguate 23505 by `ConstraintName`) + boundary-aware prefix-overlap query (`/v1` vs `/v1beta` must NOT collide).
- **T8:** `CREDENTIAL_ENC_KEY` is base64 of 32 random bytes, decoded in `crypto.New` (dev default constant updated in T1); ciphertext carries a `v1:` prefix so future key rotation doesn't force a DB wipe. E2E note: unresolvable-host validation only runs when `UPSTREAM_ALLOW_PRIVATE` is unset, so no DNS flakiness in the dev/E2E path.
