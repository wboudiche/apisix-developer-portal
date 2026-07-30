package settings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
)

// Effective is an immutable snapshot of the running configuration: every
// registry key resolved to either its DB override or its boot-time env
// default. Once built, an *Effective is never mutated — Service swaps in a
// new one on change, so readers holding a stale pointer see a consistent
// point-in-time view without locking.
type Effective struct {
	Values map[string]string // key -> effective wire value (secrets decrypted)
	Source map[string]string // key -> "env" | "db"
}

func (e *Effective) Get(key string) string { return e.Values[key] }
func (e *Effective) Bool(key string) bool  { return e.Values[key] == "1" }
func (e *Effective) SMTPConfigured() bool {
	return e.Get("SMTP_HOST") != "" && e.Get("SMTP_FROM") != ""
}
func (e *Effective) SandboxConfigured() bool {
	return e.Get("APISIX_SANDBOX_ADMIN_URL") != "" && e.Get("APISIX_SANDBOX_GATEWAY_URL") != ""
}

// ProbeResult is one connectivity/sanity check performed against a candidate
// configuration before it is persisted.
type ProbeResult struct {
	Name   string `json:"name"` // "apisix" | "sandbox" | "smtp"
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Prober runs live checks (e.g. dial SMTP, call the APISIX admin API) against
// a candidate snapshot before Set commits it. touched lists the keys the
// caller is changing, so a Prober can skip checks unrelated to the edit.
type Prober interface {
	Probe(ctx context.Context, candidate *Effective, touched map[string]bool) []ProbeResult
}

// FieldErrors reports per-field validation/invariant failures. It is never
// forceable: an unknown/read-only key or a broken invariant is a bug in the
// request, not a live-environment condition force can waive.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return fmt.Sprintf("settings: %d invalid field(s)", len(f)) }

// ProbeError reports a live connectivity check failure. Unlike FieldErrors,
// callers may pass force=true to Set to persist the change anyway.
type ProbeError struct{ Results []ProbeResult }

func (p *ProbeError) Error() string { return "settings: probe failed" }

var (
	ErrUnknownKey  = errors.New("settings: unknown key")
	ErrReadOnlyKey = errors.New("settings: read-only key")
)

// Service is the runtime-settings store: it merges env defaults with DB
// overrides into an Effective snapshot, republishing atomically on every
// change and notifying registered hooks.
type Service struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
	prober Prober
	env    map[string]string // boot-time env values, immutable after NewService

	mu    sync.Mutex // serializes writers (Set/Reset) and hook execution
	snap  atomic.Pointer[Effective]
	hooks []func(*Effective)
}

// envValues maps every registry key to its boot-time Config value. Booleans
// render as "1"/"" to match the wire/env convention used throughout registry.
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

// NewService loads DB overrides and builds the initial snapshot. Rows that
// fail to decrypt or match no registry key are logged and skipped — the env
// default applies instead — so a corrupted or stale row can never prevent
// boot.
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
		// A row may have been persisted before this key gained stricter
		// validation (e.g. a free-form TRUSTED_PROXIES value saved before CIDR
		// validation was added). Keep it anyway: dropping a previously-accepted
		// value here would silently change runtime behavior on the next boot,
		// which is worse than booting with a legacy value. Just log it so an
		// admin notices and can fix it via the settings UI.
		if err := Validate(d, v); err != nil {
			log.Printf("settings: override %q fails current validation (%v); keeping it, fix via the settings UI", k, err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// build resolves every registry key against overrides (DB) falling back to
// env, producing a fresh immutable Effective.
func (s *Service) build(overrides map[string]string) *Effective {
	e := &Effective{Values: map[string]string{}, Source: map[string]string{}}
	for _, d := range registry {
		if v, ok := overrides[d.Key]; ok {
			e.Values[d.Key], e.Source[d.Key] = v, "db"
		} else {
			e.Values[d.Key], e.Source[d.Key] = s.env[d.Key], "env"
		}
	}
	return e
}

// Snapshot returns the current effective configuration. Lock-free: it is a
// single atomic pointer load, safe to call from any goroutine at any rate.
func (s *Service) Snapshot() *Effective { return s.snap.Load() }

// EnvDefault returns the boot-time env value for key, ignoring any DB
// override — used by the admin UI to show "reset to: <value>".
func (s *Service) EnvDefault(key string) string { return s.env[key] }

// OnChange registers a hook invoked (serially, under the writer lock) after
// every successful Set/Reset swap, with the new snapshot.
func (s *Service) OnChange(h func(*Effective)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, h)
}

// candidate returns the current snapshot with values overlaid (not
// persisted) — used to evaluate invariants/probes before committing a Set.
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

// checkKeys validates that every key is known and editable, and that its
// value passes type validation. Unknown/read-only short-circuit immediately
// (400-class errors); type failures accumulate into FieldErrors (422).
//
// Keys are walked in sorted order rather than map order: the unknown and
// read-only branches return on the first offender, so with several bad keys
// map iteration would make the reported one vary between identical requests.
func checkKeys(values map[string]string) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fe := FieldErrors{}
	for _, k := range keys {
		d, ok := Lookup(k)
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownKey, k)
		}
		if !d.Editable {
			return fmt.Errorf("%w: %s", ErrReadOnlyKey, k)
		}
		if err := Validate(d, values[k]); err != nil {
			fe[k] = err.Error()
		}
	}
	if len(fe) > 0 {
		return fe
	}
	return nil
}

// invariants checks cross-field rules that must hold in any effective
// configuration, DB-backed or not. Never forceable.
func invariants(c *Effective) error {
	if c.Bool("REQUIRE_EMAIL_VERIFICATION") && !c.SMTPConfigured() {
		return FieldErrors{"REQUIRE_EMAIL_VERIFICATION": "requires SMTP_HOST and SMTP_FROM to be set"}
	}
	return nil
}

// Set validates, enforces invariants, probes (unless force), persists all-or-
// nothing, swaps the snapshot, audits, and runs hooks.
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
			`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id, forced) VALUES($1,$2,$3,$4,$5)`,
			k, oldV, newV, adminID, force); err != nil {
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

// Reset deletes a key's DB override, reverting it to its env default.
//
// The post-reset state is computed and invariant-checked BEFORE the DELETE is
// committed: a reset that would leave the configuration invalid (e.g.
// clearing SMTP_HOST while REQUIRE_EMAIL_VERIFICATION is on with no other
// SMTP override) fails with FieldErrors and leaves the DB row — and the
// live snapshot — untouched.
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

	// Compute the post-reset candidate from the current DB overrides (not
	// yet mutated) minus this key, and validate it BEFORE touching the DB.
	overrides, err := s.loadOverrides(ctx)
	if err != nil {
		return err
	}
	delete(overrides, key)
	next := s.build(overrides)
	if err := invariants(next); err != nil {
		// Resetting must not create an invalid state (e.g. resetting
		// SMTP_HOST to an empty env default while verification is on).
		return err
	}

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
	// Reset always reverts to the env default, never a forced override, so
	// forced is unconditionally FALSE here (unlike Set, which records the
	// caller's actual force flag).
	if _, err := tx.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id, forced) VALUES($1,$2,$3,$4,FALSE)`,
		key, oldV, newV, adminID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.snap.Store(next)
	for _, h := range s.hooks {
		h(next)
	}
	return nil
}

// Test runs the configured Prober against a hypothetical candidate without
// persisting anything — used by the admin UI's "test connection" action.
//
// Unlike Set/Reset, a probe never writes a portal_settings_audit row (nothing
// is persisted), so it would otherwise leave no trail even though it sends
// stored write-only secrets (e.g. APISIX_ADMIN_KEY) as a header to an
// admin-supplied candidate URL. Log the acting admin and the touched keys
// (never the values) so that action is still auditable from the app log.
func (s *Service) Test(ctx context.Context, values map[string]string, adminID int64) []ProbeResult {
	if err := checkKeys(values); err != nil {
		return []ProbeResult{{Name: "validation", OK: false, Detail: err.Error()}}
	}
	touched := map[string]bool{}
	keys := make([]string, 0, len(values))
	for k := range values {
		touched[k] = true
		keys = append(keys, k)
	}
	cand := s.candidate(values)
	// Forensic detail: name the candidate endpoint(s) actually being probed —
	// URLs/hosts only, NEVER secret values (e.g. APISIX_ADMIN_KEY) — so a log
	// review can tell which target an admin's probe hit, not just which keys
	// were touched.
	var targets []string
	if groupTouched(touched, "apisix") {
		targets = append(targets, "apisix="+cand.Get("APISIX_ADMIN_URL"))
	}
	if groupTouched(touched, "sandbox") {
		targets = append(targets, "sandbox="+cand.Get("APISIX_SANDBOX_ADMIN_URL"))
	}
	if groupTouched(touched, "smtp") {
		targets = append(targets, "smtp="+cand.Get("SMTP_HOST")+":"+cand.Get("SMTP_PORT"))
	}
	log.Printf("settings: probe test by admin=%d touched=%v targets=%v", adminID, keys, targets)
	if s.prober == nil {
		return nil
	}
	return s.prober.Probe(ctx, cand, touched)
}
