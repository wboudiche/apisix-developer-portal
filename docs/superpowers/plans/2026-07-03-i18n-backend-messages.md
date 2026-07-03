# i18n Backend Messages (Sub-project 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Localize the API's ~163 user-facing error messages by the request locale (French/English), driven by an `Accept-Language` header the frontend sends.

**Architecture:** A Go `internal/i18n` package (fr/en `map[string]string` catalogs + `T(lang, key, args…)` + context plumbing + an `Accept-Language` middleware). A new `httpx.ErrorT(w, r, status, key)` writes the localized `{"error":…}`. Every `httpx.Error("literal")` site swaps to `ErrorT("key")`, package-by-package. The frontend sends `Accept-Language` on every request.

**Tech Stack:** Go 1.25 (stdlib net/http), chi; React/TS client. Postgres unaffected.

## Global Constraints

- New package `internal/i18n`; touches `internal/httpx`, `internal/server`, and the handler packages `auth`/`catalog`/`subscriptions`/`applications`/`admin`/`teams`/`tryit`/`ratings`; plus `web/src/api/client.ts`.
- Catalogs `internal/i18n/catalog_fr.go` (`var fr = map[string]string{…}`) + `catalog_en.go` (`var en = map[string]string{…}`), keyed by dotted area keys. Every task that adds `fr` keys adds the matching `en` keys in the SAME task; a runtime parity test enforces identical key sets.
- Locale `i18n.Lang` = `"fr"` | `"en"`; `DefaultLang = "fr"`. Middleware: first `Accept-Language` tag, `en*`→`en`, else `fr`; absent/garbage → `fr`.
- Today's messages are **English** → `en[key]` = the current string VERBATIM, `fr[key]` = a faithful French translation. (The one French straggler `"abonnez-vous pour noter cette API"` is reversed.) Identical strings reuse one key.
- `httpx.ErrorT(w, r, status, key, args…)` reads the ctx locale, keeps the `{"error":…}` JSON shape. Handlers already have `r` in scope — a literal→`(r,key)` swap, no signature changes.
- **Existing handler tests** that assert an English error message will otherwise get the French default: in each package sweep, add `Accept-Language: en` to those requests (so they keep asserting the English string) OR update the assertion to the French default — whichever is smaller per test.
- Go tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w`; `go vet ./...`. Frontend: `cd web && pnpm exec vitest run <file> --no-file-parallelism && pnpm build`.

---

## Task 1: Foundation — i18n package, middleware, ErrorT, frontend header

**Files:**
- Create: `internal/i18n/i18n.go`, `internal/i18n/catalog_fr.go`, `internal/i18n/catalog_en.go`, `internal/i18n/i18n_test.go`
- Modify: `internal/httpx/respond.go` (ErrorT), `internal/server/server.go` (wrap mux), `web/src/api/client.ts` (langHeaders)
- Test: `web/src/api/client.lang.test.ts` (create)

**Interfaces:**
- Produces: `i18n.Lang`, `i18n.DefaultLang`, `i18n.T(lang, key, args…) string`, `i18n.FromContext(ctx) Lang`, `i18n.WithLang(ctx, Lang) context.Context`, `i18n.Middleware(next http.Handler) http.Handler`; `httpx.ErrorT(w, r, status, key, args…)`; frontend `langHeaders(token?)`.

- [ ] **Step 1: Write the failing Go test**

Create `internal/i18n/i18n_test.go`:
```go
package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTFallbackAndInterp(t *testing.T) {
	fr["test.hi"] = "Bonjour %s"
	en["test.hi"] = "Hello %s"
	if got := T("en", "test.hi", "Ada"); got != "Hello Ada" {
		t.Errorf("en = %q", got)
	}
	if got := T("fr", "test.hi", "Ada"); got != "Bonjour Ada" {
		t.Errorf("fr = %q", got)
	}
	// unknown lang falls back to fr; unknown key falls back to the key itself
	if got := T("de", "test.hi", "Ada"); got != "Bonjour Ada" {
		t.Errorf("fallback lang = %q", got)
	}
	if got := T("en", "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q", got)
	}
	delete(fr, "test.hi"); delete(en, "test.hi")
}

func TestParity(t *testing.T) {
	for k := range fr {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q in fr but not en", k)
		}
	}
	for k := range en {
		if _, ok := fr[k]; !ok {
			t.Errorf("key %q in en but not fr", k)
		}
	}
}

func TestMiddlewareResolvesLocale(t *testing.T) {
	cases := map[string]Lang{"fr": "fr", "en-US,en;q=0.9": "en", "": "fr", "de-DE": "fr"}
	for header, want := range cases {
		var got Lang
		h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = FromContext(r.Context()) }))
		req := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			req.Header.Set("Accept-Language", header)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != want {
			t.Errorf("Accept-Language %q → %q, want %q", header, got, want)
		}
	}
}

func TestFromContextDefault(t *testing.T) {
	if FromContext(context.Background()) != DefaultLang {
		t.Error("empty context should yield DefaultLang")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/i18n/ -v`
Expected: FAIL — package/exports missing.

- [ ] **Step 3: Implement the i18n package**

`internal/i18n/i18n.go`:
```go
// Package i18n localizes user-facing API messages by request locale.
package i18n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Lang string

const DefaultLang Lang = "fr"

type ctxKey struct{}

func WithLang(ctx context.Context, l Lang) context.Context { return context.WithValue(ctx, ctxKey{}, l) }

func FromContext(ctx context.Context) Lang {
	if l, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return l
	}
	return DefaultLang
}

// T returns the localized message for key in lang, falling back to French then
// the key itself. When args are given, they are applied with fmt.Sprintf.
func T(lang Lang, key string, args ...any) string {
	cat := fr
	if lang == "en" {
		cat = en
	}
	msg, ok := cat[key]
	if !ok {
		if msg, ok = fr[key]; !ok {
			msg = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// parse reduces an Accept-Language header to a supported Lang.
func parse(header string) Lang {
	tag := strings.TrimSpace(strings.SplitN(strings.SplitN(header, ",", 2)[0], ";", 2)[0])
	if strings.HasPrefix(strings.ToLower(tag), "en") {
		return "en"
	}
	return DefaultLang
}

// Middleware stores the request's resolved locale in the context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithLang(r.Context(), parse(r.Header.Get("Accept-Language")))))
	})
}
```
`internal/i18n/catalog_fr.go`:
```go
package i18n

// fr holds French message strings keyed by dotted area keys. Grown per package
// sweep; kept in key-set parity with en (enforced by TestParity).
var fr = map[string]string{}
```
`internal/i18n/catalog_en.go`:
```go
package i18n

// en holds English message strings (the pre-i18n verbatim wording).
var en = map[string]string{}
```
(The test seeds/deletes `test.hi` itself, so the maps start empty; package sweeps add entries.)

- [ ] **Step 4: Add `httpx.ErrorT`**

In `internal/httpx/respond.go`, add (import `net/http` already present; add `apisix-portal/internal/i18n`):
```go
// ErrorT writes a localized {"error": msg} body, resolving the message for the
// request's locale from the i18n catalog.
func ErrorT(w http.ResponseWriter, r *http.Request, status int, key string, args ...any) {
	Error(w, status, i18n.T(i18n.FromContext(r.Context()), key, args...))
}
```

- [ ] **Step 5: Wire the middleware in `server.go`**

In `internal/server/server.go`, the return is currently:
`return httpx.SecurityHeaders(httpx.MaxBodyBytes(1 << 20)(logRequests(mux)))`
Wrap it with the i18n middleware (outermost, so the locale is in context for all handlers):
```go
	return i18n.Middleware(httpx.SecurityHeaders(httpx.MaxBodyBytes(1 << 20)(logRequests(mux))))
```
(add the import `apisix-portal/internal/i18n`.)

- [ ] **Step 6: Frontend — send Accept-Language on every request**

In `web/src/api/client.ts`, replace `authHeaders` with `langHeaders` (and keep an `authHeaders = langHeaders` alias if many call sites use the old name — OR rename all call sites):
```ts
function langHeaders(token?: string): HeadersInit {
  const h: Record<string, string> = { 'Accept-Language': localStorage.getItem('lang') || 'fr' }
  if (token) { h['Content-Type'] = 'application/json'; h['Authorization'] = `Bearer ${token}` }
  return h
}
```
- Replace every `authHeaders(token)` call with `langHeaders(token)`.
- For the PUBLIC/unauthed calls that currently send no headers (e.g. `getProducts`, `getProduct`, `getProductSpec`, `getChangelog`, `getPlans`), add `{ headers: langHeaders() }` to their `fetch` (or merge into existing options). `login`/`register` POST bodies: use `langHeaders()` + keep `Content-Type` (pass a token-less variant that still sets Content-Type for POST — adjust `langHeaders` to also set Content-Type when a body is sent, or set it explicitly at those two call sites).

Create `web/src/api/client.lang.test.ts`:
```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { getProducts } from './client'

afterEach(() => vi.unstubAllGlobals())

it('sends Accept-Language from localStorage', async () => {
  localStorage.setItem('lang', 'en')
  const f = vi.fn(async () => new Response(JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', f)
  await getProducts({})
  const opts = f.mock.calls[0][1]
  expect((opts?.headers as Record<string, string>)['Accept-Language']).toBe('en')
})
```

- [ ] **Step 7: Run to verify all pass**

Run: `go test ./internal/i18n/ ./internal/httpx/ && go build ./... && go vet ./internal/i18n/ ./internal/httpx/ ./internal/server/`
Then: `cd web && pnpm exec vitest run src/api/client.lang.test.ts --no-file-parallelism && pnpm build`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/i18n/ internal/httpx/respond.go internal/server/server.go web/src/api/client.ts web/src/api/client.lang.test.ts
git commit -m "feat(i18n): backend message catalog + Accept-Language middleware + ErrorT + frontend header"
```

---

## Tasks 2–7: Per-package error sweep (same pattern each)

Each task converts one (or two grouped) handler package(s): **for every `httpx.Error(w, status, "message")` in the package, (a) add an entry to `internal/i18n/catalog_en.go` (`en[key]` = the current string VERBATIM) and `catalog_fr.go` (`fr[key]` = a faithful French translation), namespaced by area (e.g. `subscribe.*`, `admin.*`, `teams.*`, `common.*` for shared ones like `common.badAppID`); (b) replace the call with `httpx.ErrorT(w, r, status, "key")` (add `args…` if the original interpolated). Reuse one key for identical strings.** Then update that package's existing tests: any test asserting the English message must now either set `Accept-Language: en` on the request (keeps asserting English) or assert the French default — pick the smaller change per test.

**Per-area step pattern (repeat for each task):**
- [ ] Add the package's keys to `catalog_en.go` (verbatim English) + `catalog_fr.go` (French).
- [ ] Swap every `httpx.Error(...)` → `httpx.ErrorT(r, "key")` in the package's non-test `.go`.
- [ ] Update the package's tests (add `Accept-Language: en` to message-asserting requests, or assert French) so they pass; the `i18n` parity test still passes.
- [ ] Run: `DATABASE_URL=... go test ./internal/<pkg>/ ./internal/i18n/ && go vet ./internal/<pkg>/` → PASS.
- [ ] Grep gate: `grep -n 'httpx.Error(' internal/<pkg>/*.go | grep -v _test` → empty (all migrated to `ErrorT`).
- [ ] Commit `feat(i18n): localize <pkg> error messages`.

### Task 2: auth (18 sites)
`internal/auth/*.go`. Namespaces `auth.*` / `common.*`.

### Task 3: catalog (7) + applications (6)
`internal/catalog/*.go` + `internal/applications/*.go`. Namespaces `catalog.*`, `app.*`, `common.*`.

### Task 4: subscriptions (32 sites)
`internal/subscriptions/*.go`. Namespace `subscribe.*` / `common.*`. (Includes `ErrProductDeprecated`→409 "This API no longer accepts new subscriptions.", `already subscribed`, key/sandbox/oidc errors.)

### Task 5: admin (43 sites — largest)
`internal/admin/*.go`. Namespace `admin.*` / `common.*`. (Product/plan/changelog CRUD validation + conflict messages.)

### Task 6: teams (25 sites)
`internal/teams/*.go`. Namespace `teams.*` / `common.*`. (`cannot remove the last owner`, `already a member`, personal-team guards, `owner only`, etc.)

### Task 7: tryit (22) + ratings (10)
`internal/tryit/*.go` + `internal/ratings/*.go`. Namespaces `tryit.*`, `ratings.*`, `common.*`. (Ratings has the one French straggler `"abonnez-vous pour noter cette API"` → `fr` = that, `en` = English translation.)

---

## Task 8: Full suite + live verification

**Files:** none (verification; small cleanups if a grep finds a straggler).

- [ ] **Step 1: Whole-backend grep + suite**

Run:
```bash
grep -rn 'httpx.Error(' internal/ | grep -v _test   # should be empty or only deliberate non-user-facing literals (list + justify any)
DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...
```
Expected: the grep is empty (all migrated); the suite is green.

- [ ] **Step 2: Frontend suite + build**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' --no-file-parallelism && pnpm build`
Expected: green.

- [ ] **Step 3: Live verification**

Bring up the stack; run the portal (`:8090`) + vite (`:5173`). In the browser:
1. UI in **French**: trigger a blocked action — e.g. subscribe an app to the deprecated `currencyconverterapi`, or (as a team owner) add a member by an unknown email — the error toast/message is **French**.
2. Toggle to **English**, repeat the same action → the error is **English**.
3. Confirm in the network tab that requests carry `Accept-Language: fr`/`en`.
Also spot-check with `curl`: `POST …/subscriptions` for the deprecated product with `-H 'Accept-Language: fr'` → French `error`; with `en` → English `error`.
**Look at the error in both languages.**

- [ ] **Step 4: No commit** (verification; note results in the ledger).

---

## Self-Review notes

- **Spec coverage:** i18n package + `T` + parity + middleware (T1) ✅; `httpx.ErrorT` + mux wrap (T1) ✅; frontend `Accept-Language` (T1) ✅; the 163-site sweep across auth/catalog/applications/subscriptions/admin/teams/tryit/ratings (T2–T7) ✅; full suite + per-locale live (T8) ✅. Sub-project 3 (user pref + email) out of scope.
- **Type consistency:** `i18n.T(lang, key, args…)`, `FromContext`/`WithLang`/`Middleware`, `httpx.ErrorT(w, r, status, key, args…)` signatures are fixed in T1 and used unchanged by every sweep. Catalog keys are the shared contract between `catalog_fr.go`/`catalog_en.go` (parity-tested).
- **Implementer notes:** the maps `fr`/`en` are package-private (lowercase) in `internal/i18n` — the sweep tasks edit those two files directly. Preserve each current English message BYTE-FOR-BYTE as the `en` value (the frontend/e2e may assert it). `httpx` importing `i18n` is a one-way dep (i18n imports neither httpx nor server) — no cycle. Existing handler tests are the main churn: prefer adding `Accept-Language: en` to the request over rewording assertions.
