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

// StubProber is exported (unusually, for a _test.go helper) so the external
// settings_test package (handler_test.go) can construct it too — see that
// file's package comment for why handler tests live outside package settings.
type StubProber struct {
	Results []ProbeResult
	calls   int
	mu      sync.Mutex
}

func (p *StubProber) Probe(_ context.Context, _ *Effective, _ map[string]bool) []ProbeResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.Results
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

// NewTestService is exported (unusually, for a _test.go helper) so the external
// settings_test package (handler_test.go) can build a DB-backed *Service too.
func NewTestService(t *testing.T, prober Prober) *Service {
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
	svc := NewTestService(t, &StubProber{})
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
	svc := NewTestService(t, &StubProber{})
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
	svc := NewTestService(t, &StubProber{})
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
	svc := NewTestService(t, &StubProber{})
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
	p := &StubProber{Results: []ProbeResult{{Name: "smtp", OK: false, Detail: "connection refused"}}}
	svc := NewTestService(t, p)
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
	svc := NewTestService(t, &StubProber{})
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
	svc, err := NewService(pool, cipher, config.Load(), &StubProber{})
	if err != nil {
		t.Fatalf("NewService must tolerate bad rows: %v", err)
	}
	snap := svc.Snapshot()
	if snap.Get("SMTP_PASSWORD") != "envpw" || snap.Source["SMTP_PASSWORD"] != "env" {
		t.Fatalf("bad row must fall back to env: %q/%q", snap.Get("SMTP_PASSWORD"), snap.Source["SMTP_PASSWORD"])
	}
}

func TestConcurrentReadersDuringSwap(t *testing.T) {
	svc := NewTestService(t, &StubProber{})
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

func TestResetCannotBreakInvariant(t *testing.T) {
	svc := NewTestService(t, &StubProber{})
	ctx := context.Background()
	// env has SMTP (from NewTestService Setenv); enable verification, then
	// override SMTP_HOST in DB, then try to reset SMTP_FROM's env... simpler:
	// clear env SMTP_FROM so the DB override is the only thing keeping SMTP on.
	t.Setenv("SMTP_FROM", "")
	svc2 := func() *Service { // rebuild service with the new env
		pool := testPool(t)
		cipher, _ := crypto.New(config.DevCredentialEncKey)
		s, err := NewService(pool, cipher, config.Load(), &StubProber{})
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
