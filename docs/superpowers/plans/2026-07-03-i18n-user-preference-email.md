# i18n User Preference + Email (Sub-project 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stored `users.language` that syncs the UI across devices on login and localizes approval-loop emails per recipient.

**Architecture:** A `users.language` column (fr/en) seeded on register from `Accept-Language`; a `PUT /api/me/language` endpoint persists the toggle when logged in; the login/register response already carries the `user` object, so exposing `language` there lets the client set `localStorage['lang']` on login. `internal/notify` fetches each recipient's language and renders each email in that language.

**Tech Stack:** Go 1.25 (pgx, chi), React/TS. Reuses `internal/i18n` (sub-project 2) for the request locale.

## Global Constraints

- Locale values are exactly `'fr'` | `'en'`; default `'fr'`. `users.language` is `TEXT NOT NULL DEFAULT 'fr' CHECK (language IN ('fr','en'))`.
- `language` travels in the login/register response `user` object, NOT in the JWT (no `Tokenizer.Issue` change).
- Email copy: keep the CURRENT French wording verbatim as each template's `fr`; the `en` is a faithful translation. Same `{baseURL}/admin/approvals` | `/applications` | `/` links + app/product/plan names.
- The `Accept-Language` transport for API messages is UNCHANGED. Emails read the stored preference directly.
- Go tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w`; `go vet ./...`. Frontend: `cd web && pnpm exec vitest run <file> --no-file-parallelism && pnpm build` (NOT `tsc --noEmit` — it's a no-op in web/).

---

## Task 1: Migration + auth language plumbing + register seed

**Files:**
- Create: `internal/db/migrations/0015_user_language.sql`
- Modify: `internal/auth/user.go`, `internal/auth/repo.go`, `internal/auth/handler.go`
- Test: `internal/auth/repo_test.go` (add), `internal/auth/handler_test.go` (add a case)

**Interfaces:**
- Produces: `User.Language string` (json `"language"`); `Repo.Create(ctx, email, passwordHash, name, lang string) (User, error)`; `Repo.SetLanguage(ctx, userID int64, lang string) error`; `UserStore.Create(...lang)` + `UserStore.SetLanguage(...)`. Register seeds `language` from `i18n.FromContext(r.Context())`.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0015_user_language.sql`:
```sql
ALTER TABLE users
  ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'
  CHECK (language IN ('fr','en'));
```

- [ ] **Step 2: Add the failing repo test**

Add to `internal/auth/repo_test.go` (create the file if absent; mirror the DB-test setup used elsewhere in `internal/auth` — a `pgxpool` from `DATABASE_URL`, unique email per run):
```go
func TestCreateSeedsAndSetLanguage(t *testing.T) {
	pool := testPool(t) // existing helper pattern; if none, dial DATABASE_URL like other repo tests
	repo := NewRepo(pool)
	ctx := context.Background()
	email := fmt.Sprintf("lang-%d@x.io", time.Now().UnixNano())

	u, err := repo.Create(ctx, email, "hash", "Ada", "en")
	if err != nil { t.Fatalf("create: %v", err) }
	if u.Language != "en" { t.Fatalf("seeded language = %q, want en", u.Language) }

	if err := repo.SetLanguage(ctx, u.ID, "fr"); err != nil { t.Fatalf("setlang: %v", err) }
	got, _, err := repo.GetByEmail(ctx, email)
	if err != nil { t.Fatalf("getbyemail: %v", err) }
	if got.Language != "fr" { t.Fatalf("after SetLanguage = %q, want fr", got.Language) }

	if err := repo.SetLanguage(ctx, u.ID, "de"); err == nil {
		t.Fatal("SetLanguage('de') should violate the CHECK")
	}
}
```
(If `internal/auth` has no existing DB-test helper, dial the pool inline: `pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))`, `t.Skip` if unset — match whatever `internal/subscriptions` or `internal/teams` repo tests already do.)

- [ ] **Step 3: Run it to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/auth/ -run TestCreateSeedsAndSetLanguage -v`
Expected: FAIL — `Create` takes 4 args / `Language` undefined.

- [ ] **Step 4: Add `Language` to the User struct**

In `internal/auth/user.go`, add the field:
```go
type User struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Language string `json:"language"`
}
```

- [ ] **Step 5: Thread language through the repo**

In `internal/auth/repo.go`, change `Create` to accept + insert + return `lang`, scan `language` in both `Create` and `GetByEmail`, and add `SetLanguage`:
```go
func (r *Repo) Create(ctx context.Context, email, passwordHash, name, lang string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var u User
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role, language)
		 VALUES ($1,$2,$3,'developer',$4)
		 RETURNING id, email, name, role, language`,
		email, passwordHash, name, lang,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
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

// SetLanguage updates the user's stored UI language ('fr'|'en').
func (r *Repo) SetLanguage(ctx context.Context, userID int64, lang string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET language=$2 WHERE id=$1`, userID, lang)
	return err
}
```
And in `GetByEmail`, add `language` to the SELECT + Scan:
```go
func (r *Repo) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, language, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &hash)
	return u, hash, err
}
```

- [ ] **Step 6: Update the UserStore interface + register seed**

In `internal/auth/handler.go`, widen the `UserStore` interface and seed the language in `register`. Add the import `apisix-portal/internal/i18n`.
```go
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name, lang string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
	SetLanguage(ctx context.Context, userID int64, lang string) error
}
```
In `register`, change the `Create` call to pass the request locale (the i18n middleware already resolved it into the context):
```go
	lang := string(i18n.FromContext(r.Context()))
	u, err := h.store.Create(r.Context(), c.Email, hash, c.Name, lang)
```
(Leave the rest of `register` unchanged — the response already returns `u`, which now includes `language`.)

- [ ] **Step 7: Add the register-seed handler test**

Add to `internal/auth/handler_test.go` a case that posts register with `Accept-Language: en` and asserts the response `user.language == "en"`. Use the existing handler-test harness (a `fakeStore` or the real store — match the file's pattern). If the tests use a `fakeStore`, update its `Create` signature to accept `lang` and record it, add a `SetLanguage` method, and assert the recorded lang:
```go
func TestRegisterSeedsLanguageFromAcceptLanguage(t *testing.T) {
	// build the handler with the test's existing store + tokenizer
	req := httptest.NewRequest("POST", "/api/auth/register",
		strings.NewReader(`{"email":"seed@x.io","password":"password1","name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	req = req.WithContext(i18n.WithLang(req.Context(), "en")) // mirror what the middleware does in prod
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated { t.Fatalf("code=%d body=%s", rec.Code, rec.Body) }
	var out struct{ User struct{ Language string `json:"language"` } `json:"user"` }
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.User.Language != "en" { t.Fatalf("seeded language=%q, want en", out.User.Language) }
}
```
NOTE: the handler test bypasses the real middleware, so set the context locale explicitly with `i18n.WithLang` (as shown). In production the outermost `i18n.Middleware` sets it from the header.

- [ ] **Step 8: Run the auth tests**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/auth/ -v && go build ./... && go vet ./internal/auth/`
Expected: PASS. (Fix any OTHER `Create(`/`Store.Create` call sites the compiler flags — e.g. `internal/server` wiring uses `authRepo` which satisfies the interface structurally, so no change there; only `auth` package call sites + fakes change.)

- [ ] **Step 9: Commit**

```bash
gofmt -w internal/auth/
git add internal/db/migrations/0015_user_language.sql internal/auth/
git commit -m "feat(i18n): users.language column + register seed from Accept-Language"
```

---

## Task 2: `PUT /api/me/language` endpoint

**Files:**
- Modify: `internal/auth/handler.go`, `internal/server/server.go`
- Test: `internal/auth/handler_test.go` (add)

**Interfaces:**
- Consumes: `UserStore.SetLanguage` (Task 1), `auth.UserID(ctx)`, `auth.RequireAuth`.
- Produces: exported `func (h *Handler) PutLanguage(w http.ResponseWriter, r *http.Request)`; route `PUT /api/me/language` behind `requireAuth`.

- [ ] **Step 1: Write the failing endpoint test**

Add to `internal/auth/handler_test.go`:
```go
func TestPutLanguage(t *testing.T) {
	// h built with the test store; store records SetLanguage(userID, lang)
	call := func(ctxLang string, body string, uid int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/api/me/language", strings.NewReader(body))
		if uid != 0 { req = req.WithContext(auth.WithUserID(req.Context(), uid)) }
		rec := httptest.NewRecorder()
		h.PutLanguage(rec, req)
		return rec
	}
	if rec := call("", `{"language":"en"}`, 7); rec.Code != http.StatusNoContent {
		t.Fatalf("valid put code=%d", rec.Code)
	}
	if rec := call("", `{"language":"de"}`, 7); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad value code=%d, want 400", rec.Code)
	}
	if rec := call("", `{"language":"fr"}`, 0); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-user code=%d, want 401", rec.Code)
	}
}
```
(Package the test as `auth` or `auth_test` consistently with the file; `WithUserID`/`UserID` are exported from the package.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/auth/ -run TestPutLanguage -v`
Expected: FAIL — `PutLanguage` undefined.

- [ ] **Step 3: Implement `PutLanguage`**

In `internal/auth/handler.go`:
```go
type languagePref struct {
	Language string `json:"language"`
}

// PutLanguage persists the authenticated user's UI language preference. Mounted
// at PUT /api/me/language behind RequireAuth.
func (h *Handler) PutLanguage(w http.ResponseWriter, r *http.Request) {
	uid := UserID(r.Context())
	if uid == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.middleware.missingToken")
		return
	}
	var p languagePref
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || (p.Language != "fr" && p.Language != "en") {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if err := h.store.SetLanguage(r.Context(), uid, p.Language); err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.register.createFailed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```
(Reuses existing catalog keys `auth.middleware.missingToken`, `common.invalidBody`; `auth.register.createFailed` is an acceptable generic 500 here — OR add a dedicated `common.internalError` key to both catalogs if you prefer, but reuse keeps parity simple.)

- [ ] **Step 4: Mount the route**

In `internal/server/server.go`, after the existing `requireAuth`-mounted routes, add:
```go
	mux.Handle("/api/me/language", requireAuth(http.HandlerFunc(authH.PutLanguage)))
```
(`authH` is the `*auth.Handler` already constructed at `server.go:49`.)

- [ ] **Step 5: Run tests + build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/auth/ ./internal/server/ -v && go build ./... && go vet ./internal/auth/ ./internal/server/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/auth/ internal/server/
git add internal/auth/handler.go internal/server/server.go internal/auth/handler_test.go
git commit -m "feat(i18n): PUT /api/me/language to persist the language preference"
```

---

## Task 3: Per-recipient email localization (`internal/notify`)

**Files:**
- Modify: `internal/notify/repo.go`, `internal/notify/notifier.go`
- Test: `internal/notify/notifier_test.go` (add cases), `internal/notify/repo_test.go` (if present, update signatures)

**Interfaces:**
- Produces: `type Recipient struct { Email, Lang string }`; `Repo.OwnerEmailsForApp(ctx, appID) ([]Recipient, string, error)`; `Repo.AdminEmails(ctx) ([]Recipient, error)`; `Resolver` updated to match; `emailTemplates` keyed by `(kind, lang)`; `deliver` renders + sends one message per recipient in that recipient's language.

- [ ] **Step 1: Write the failing template + deliver tests**

Add to `internal/notify/notifier_test.go` (reuse the existing fake `Sender` + fake `Resolver` in that file; if the fake Resolver returns `[]string`, update it to `[]Recipient`):
```go
func TestEmailTemplateParity(t *testing.T) {
	for _, kind := range []string{kindRequested, kindApproved, kindRejected} {
		for _, lang := range []string{"fr", "en"} {
			tpl, ok := emailTemplates[kind][lang]
			if !ok || tpl.subject == "" || tpl.body == "" {
				t.Errorf("missing/empty template kind=%s lang=%s", kind, lang)
			}
		}
	}
}

func TestDeliverLocalizesPerRecipient(t *testing.T) {
	sender := &fakeSender{} // records []sent{to, subject, body}
	repo := &fakeResolver{
		admins: []Recipient{{"fr-admin@x.io", "fr"}, {"en-admin@x.io", "en"}},
		app:    "Mon App", product: "Une API", plan: "Silver",
	}
	n := NewNotifier(sender, repo, "http://portal")
	n.deliver(kindRequested, 1, 2, 3) // synchronous

	if len(sender.sent) != 2 { t.Fatalf("expected 2 messages, got %d", len(sender.sent)) }
	bySubject := map[string]sent{}
	for _, s := range sender.sent { bySubject[s.to[0]] = s }
	if !strings.Contains(bySubject["fr-admin@x.io"].subject, "Nouvelle demande") {
		t.Errorf("fr admin subject = %q", bySubject["fr-admin@x.io"].subject)
	}
	if !strings.Contains(bySubject["en-admin@x.io"].subject, "New subscription") {
		t.Errorf("en admin subject = %q", bySubject["en-admin@x.io"].subject)
	}
}

func TestDeliverUnknownLangFallsBackToFrench(t *testing.T) {
	sender := &fakeSender{}
	repo := &fakeResolver{admins: []Recipient{{"x@x.io", "de"}}, app: "A", product: "P", plan: "Pl"}
	NewNotifier(sender, repo, "http://portal").deliver(kindRequested, 1, 2, 3)
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].subject, "Nouvelle demande") {
		t.Fatalf("unknown lang should fall back to fr: %+v", sender.sent)
	}
}
```
(Match the actual field names of the existing fakes; the snippet shows intent. `fakeSender.Send(ctx, to, subject, body)` appends a `sent{to,subject,body}`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/notify/ -run 'TestEmailTemplate|TestDeliver' -v`
Expected: FAIL — `Recipient`/`emailTemplates` undefined, Resolver returns `[]string`.

- [ ] **Step 3: Add `Recipient` + per-recipient repo queries**

In `internal/notify/repo.go`, add the type and change the two recipient queries to also select `language`:
```go
// Recipient is an email address plus the recipient's stored UI language.
type Recipient struct {
	Email string
	Lang  string
}
```
`OwnerEmailsForApp` — change the SELECT to include `u.language` and build `[]Recipient`:
```go
func (r *Repo) OwnerEmailsForApp(ctx context.Context, appID int64) ([]Recipient, string, error) {
	var name string
	if err := r.pool.QueryRow(ctx, `SELECT name FROM applications WHERE id=$1`, appID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.email, u.language FROM applications a
		 JOIN team_members tm ON tm.team_id = a.team_id AND tm.role='owner'
		 JOIN users u ON u.id = tm.user_id
		 WHERE a.id=$1`, appID)
	if err != nil {
		return nil, name, err
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Email, &rc.Lang); err != nil {
			return nil, name, err
		}
		out = append(out, rc)
	}
	return out, name, rows.Err()
}
```
`AdminEmails` — same treatment:
```go
func (r *Repo) AdminEmails(ctx context.Context) ([]Recipient, error) {
	rows, err := r.pool.Query(ctx, `SELECT email, language FROM users WHERE role='admin'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Email, &rc.Lang); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Add the bilingual templates + per-recipient render in `notifier.go`**

In `internal/notify/notifier.go`, update the `Resolver` interface signatures, add the template table, and rewrite `deliver` to render per recipient. Replace the recipient-typed interface methods:
```go
type Resolver interface {
	OwnerEmailsForApp(ctx context.Context, appID int64) ([]Recipient, string, error)
	AdminEmails(ctx context.Context) ([]Recipient, error)
	ProductName(ctx context.Context, productID int64) (string, error)
	PlanName(ctx context.Context, planID int64) (string, error)
}

type emailTemplate struct{ subject, body string }

// emailTemplates[kind][lang]. body is a fmt format string; the arg order per
// kind is fixed across languages (see render()).
var emailTemplates = map[string]map[string]emailTemplate{
	kindRequested: {
		"fr": {
			subject: "Nouvelle demande d'abonnement à examiner",
			body:    "Une nouvelle demande d'abonnement attend votre validation.\n\nApplication : %s\nAPI : %s\nForfait : %s\n\nExaminez-la ici : %s/admin/approvals\n",
		},
		"en": {
			subject: "New subscription request to review",
			body:    "A new subscription request is awaiting your approval.\n\nApplication: %s\nAPI: %s\nPlan: %s\n\nReview it here: %s/admin/approvals\n",
		},
	},
	kindApproved: {
		"fr": {
			subject: "Votre abonnement est approuvé",
			body:    "Bonne nouvelle ! L'abonnement de %s à %s (%s) est approuvé.\n\nRetrouvez vos identifiants ici : %s/applications\n",
		},
		"en": {
			subject: "Your subscription is approved",
			body:    "Good news! The subscription of %s to %s (%s) is approved.\n\nFind your credentials here: %s/applications\n",
		},
	},
	kindRejected: {
		"fr": {
			subject: "Votre demande d'abonnement a été refusée",
			body:    "La demande d'abonnement de %s à %s n'a pas été approuvée.\n\nParcourez le catalogue : %s/\n",
		},
		"en": {
			subject: "Your subscription request was declined",
			body:    "The subscription request of %s to %s was not approved.\n\nBrowse the catalog: %s/\n",
		},
	},
}

func normalizeLang(l string) string {
	if l == "en" {
		return "en"
	}
	return "fr"
}
```
Now rewrite `deliver` to resolve `[]Recipient`, compute the per-kind format args, and send one localized message per recipient:
```go
func (n *Notifier) deliver(kind string, appID, productID, planID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("notify: recovered panic in deliver: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()

	product, err := n.repo.ProductName(ctx, productID)
	if err != nil {
		log.Printf("notify: product name (product=%d): %v", productID, err)
	}
	if product == "" {
		product = "une API"
	}
	owners, appName, err := n.repo.OwnerEmailsForApp(ctx, appID)
	if err != nil {
		log.Printf("notify: owner emails (app=%d): %v", appID, err)
	}
	if appName == "" {
		appName = "votre application"
	}

	var to []Recipient
	var args []any
	switch kind {
	case kindRequested:
		admins, err := n.repo.AdminEmails(ctx)
		if err != nil {
			log.Printf("notify: admin emails: %v", err)
			return
		}
		to = admins
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "un forfait"
		}
		args = []any{appName, product, plan, n.baseURL}
	case kindApproved:
		to = owners
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "votre forfait"
		}
		args = []any{appName, product, plan, n.baseURL}
	case kindRejected:
		to = owners
		args = []any{appName, product, n.baseURL}
	default:
		return
	}

	for _, rc := range to {
		if rc.Email == "" {
			continue
		}
		tpl := emailTemplates[kind][normalizeLang(rc.Lang)]
		body := fmt.Sprintf(tpl.body, args...)
		if err := n.sender.Send(ctx, []string{rc.Email}, tpl.subject, body); err != nil {
			log.Printf("notify: send %q to %s: %v", kind, rc.Email, err)
		}
	}
}
```
NOTE: the per-kind `args` order matches the `%s` order in BOTH language bodies (appName, product, [plan,] baseURL). The product/plan/appName default strings stay French (they're content fallbacks, rare, and match today's behavior).

- [ ] **Step 5: Run the notify tests + build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/notify/ -v && go build ./... && go vet ./internal/notify/`
Expected: PASS. (Update the fake `Resolver` in the test file to return `[]Recipient`; fix any `repo_test.go` assertions that expected `[]string`.)

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/notify/
git add internal/notify/
git commit -m "feat(i18n): localize approval-loop emails per recipient language"
```

---

## Task 4: Frontend — login sync + toggle persist

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/auth/AuthProvider.tsx`, `web/src/components/TopBar.tsx`
- Test: `web/src/auth/language-sync.test.tsx` (create)

**Interfaces:**
- Consumes: `PUT /api/me/language` (Task 2); `resp.user.language` (Task 1).
- Produces: `User.language?: 'fr' | 'en'`; `setMyLanguage(token, lang)`; login/register sync `setLang`; toggle persists when a token exists.

- [ ] **Step 1: Write the failing test**

Create `web/src/auth/language-sync.test.tsx`:
```tsx
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { setMyLanguage } from '../api/client'

beforeEach(() => { localStorage.clear() })
afterEach(() => vi.unstubAllGlobals())

it('setMyLanguage PUTs /api/me/language with the token + body', async () => {
  const f = vi.fn(async () => new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', f)
  await setMyLanguage('tok123', 'en')
  const [url, opts] = f.mock.calls[0]
  expect(url).toBe('/api/me/language')
  expect(opts.method).toBe('PUT')
  expect((opts.headers as Record<string, string>).Authorization).toBe('Bearer tok123')
  expect(JSON.parse(opts.body as string)).toEqual({ language: 'en' })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/auth/language-sync.test.tsx --no-file-parallelism`
Expected: FAIL — `setMyLanguage` not exported.

- [ ] **Step 3: Add the type + client fn**

In `web/src/api/types.ts`, add `language` to `User`:
```ts
export interface User {
  id: number
  email: string
  name: string
  role: string
  language?: 'fr' | 'en'
}
```
In `web/src/api/client.ts`, add (near the other authed calls; reuse `sendAuthed` + `langHeaders`):
```ts
export async function setMyLanguage(token: string, language: 'fr' | 'en'): Promise<void> {
  await sendAuthed('PUT', '/api/me/language', token, { language })
}
```
(`sendAuthed(method, url, token, body?)` — the same helper used by `setOidcClient` at `client.ts:186`; it applies `langHeaders(token)` [Authorization + Content-Type + Accept-Language] and throws the backend error on a non-2xx.)

- [ ] **Step 4: Sync on login/register in `AuthProvider`**

In `web/src/auth/AuthProvider.tsx`, import `useLang` and call `setLang` from `apply`:
```tsx
import { useLang } from '../i18n/LanguageProvider'
```
Inside `AuthProvider`, add `const { setLang } = useLang()` and update `apply`:
```tsx
  function apply(res: { user: User; token: string }) {
    setUser(res.user); setToken(res.token)
    localStorage.setItem('token', res.token)
    localStorage.setItem('user', JSON.stringify(res.user))
    if (res.user.language) setLang(res.user.language)
  }
```
(`AuthProvider` is mounted inside `LanguageProvider` in `main.tsx`, so `useLang()` is in scope.)

- [ ] **Step 5: Persist the toggle when logged in (`TopBar`)**

In `web/src/components/TopBar.tsx`, the toggle at line ~221 currently does `setLang(...)`. Pull `token` from `useAuth` (already used for `user`) and also persist when logged in. Change the handler to:
```tsx
        onClick={() => {
          const next = lang === 'fr' ? 'en' : 'fr'
          setLang(next)
          if (token) setMyLanguage(token, next).catch(() => { /* best-effort */ })
        }}
```
Add `setMyLanguage` to the `client` import and ensure `token` is destructured from the auth hook (e.g. `const { user, token } = useAuth()` — match TopBar's existing auth-hook usage).

- [ ] **Step 6: Run the test + full frontend build**

Run: `cd web && pnpm exec vitest run src/auth/language-sync.test.tsx --no-file-parallelism && pnpm build`
Expected: PASS + build green. (If TopBar's test file exists and asserts the toggle, it should still pass — the toggle still calls `setLang`; the new `setMyLanguage` is a no-op without a token.)

- [ ] **Step 7: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/auth/AuthProvider.tsx web/src/components/TopBar.tsx web/src/auth/language-sync.test.tsx
git commit -m "feat(i18n): sync language on login + persist toggle to the account"
```

---

## Task 5: Full suite + live verification

**Files:** none (verification; small fixes only if something fails).

- [ ] **Step 1: Full backend + frontend suites**

Run:
```bash
DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...
cd web && pnpm exec vitest run --exclude 'e2e/**' --no-file-parallelism && pnpm build
```
Expected: all green.

- [ ] **Step 2: Live — cross-device UI sync**

Bring up the stack (`make up`, `make run` on `PORTAL_ADDR` if `:8080` is held, vite). In the browser:
1. Register/log in a user; toggle to **English**; reload → still English.
2. Open a **fresh browser profile** (empty `localStorage`), log in as the SAME user → the UI comes up **English** (the stored preference synced via the login response). Toggle back to French there; in the first browser, log out + log in again → French.
Confirm via the network tab that the toggle (while logged in) fires `PUT /api/me/language` and login response `user.language` matches.

- [ ] **Step 3: Live — per-recipient email localization**

With Mailpit up (`:8025`): create (or reuse) two admin users with different stored languages (register one under `Accept-Language: en`, or `PUT /api/me/language` for one), then trigger a pending subscription (subscribe an app to a product). In Mailpit, confirm the **French admin gets the French email** and the **English admin gets the English email** (subject + body + the `/admin/approvals` link). Then approve/reject and confirm the owner's email is in the owner's language. **Look at both emails.**

- [ ] **Step 4: No commit** (verification; note results in the ledger).

---

## Self-Review notes

- **Spec coverage:** migration + `users.language` (T1) ✅; register seed from `Accept-Language` (T1) ✅; `PUT /api/me/language` (T2) ✅; login-response `language` → client sync (T1 exposes it, T4 consumes) ✅; toggle-persist-when-logged-in (T4) ✅; per-recipient email localization (T3) ✅; API-message transport unchanged (nothing touches the middleware) ✅; full suite + cross-device + two-language-email live (T5) ✅.
- **Type consistency:** `Repo.Create(...,lang)` + `UserStore.Create(...,lang)` + `SetLanguage` (T1) are consumed by T2's `PutLanguage`; `Recipient{Email,Lang}` + the `Resolver` signatures (T3) are internal to notify; `User.language?` + `setMyLanguage(token,lang)` (T4) match the T1 response shape and T2 endpoint.
- **Implementer notes:** `language` is NEVER added to the JWT (`Tokenizer.Issue` untouched). The migration is applied by the existing embedded-migration loader (numeric prefix, runs once). Register's locale comes from `i18n.FromContext` (the outermost middleware already set it) — do NOT re-parse `Accept-Language`. Keep French email copy verbatim as the `fr` template. The `notify` fake `Resolver`/`Sender` in the existing test file must be updated to the `[]Recipient` return shape.
