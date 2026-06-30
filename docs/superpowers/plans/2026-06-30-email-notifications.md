# Email Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Email the approval loop — admins when a subscription request is pending, the developer when it's approved or rejected — over bring-your-own SMTP, best-effort and async.

**Architecture:** A new `internal/notify` package (`Sender` SMTP impl + `Notifier` + a read `Repo` + plaintext French templates). The subscriptions `Service` gains a narrow `Notifier` interface via a `SetNotifier` setter and calls it best-effort at the Subscribe/Approve/Reject hooks; each call resolves recipients, renders, and sends in a background goroutine. Inert when SMTP isn't configured.

**Tech Stack:** Go 1.25 (stdlib `net/smtp`, pgx), docker-compose (Mailpit for dev/test).

## Global Constraints

- Module `apisix-portal`. New package `internal/notify`; integration in `internal/subscriptions`; config in `internal/config`; wiring in `internal/server`.
- **Scope = approval loop only:** admins ← new pending request (`Subscribe`); developer ← approved (`Approve`) / rejected (`Reject`). No other events.
- **SMTP bring-your-own, inert when unset:** `SMTPConfigured()` = `SMTP_HOST != "" && SMTP_FROM != ""`; when false the notifier is unset and every call is a no-op.
- **Best-effort, async:** the action never blocks on or fails because of email. Each Notifier method launches a goroutine wrapping a synchronous, testable `deliver`; all errors are `log.Printf`'d and dropped; empty recipient lists are skipped.
- Recipients: developer = the app owner's email; admins = `users` where `role='admin'`. Plaintext French bodies with a `PORTAL_BASE_URL` link.
- Tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w` every touched file; `go vet ./...`.

---

## Task 1: Config (SMTP + base URL) + Mailpit compose service

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `docker-compose.yml`

**Interfaces:**
- Produces: `Config.SMTPHost/SMTPPort/SMTPUsername/SMTPPassword/SMTPFrom/PortalBaseURL string`; `func (c Config) SMTPConfigured() bool`.

- [ ] **Step 1: Write the failing config test**

In `internal/config/config_test.go` add (mirror the existing `TestSandboxConfigDefaultsAndPredicate`/`TestOIDCConfigDefaultsAndPredicate` style):
```go
func TestSMTPConfigDefaultsAndPredicate(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	c := Load()
	if c.SMTPPort != "587" {
		t.Errorf("SMTPPort default = %q, want 587", c.SMTPPort)
	}
	if c.PortalBaseURL != "http://localhost:5173" {
		t.Errorf("PortalBaseURL default = %q", c.PortalBaseURL)
	}
	if c.SMTPConfigured() {
		t.Error("SMTPConfigured() = true with no host/from")
	}
	c.SMTPHost, c.SMTPFrom = "mail.example.com", "portal@example.com"
	if !c.SMTPConfigured() {
		t.Error("SMTPConfigured() = false with host+from set")
	}
	c.SMTPFrom = ""
	if c.SMTPConfigured() {
		t.Error("SMTPConfigured() = true with from empty")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -run TestSMTPConfig -v`
Expected: FAIL — fields/method undefined.

- [ ] **Step 3: Add the config fields + predicate**

In `internal/config/config.go`, add to `Config`:
```go
	SMTPHost      string
	SMTPPort      string
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	PortalBaseURL string
```
In `Load()` (near the other `get(...)` lines):
```go
		SMTPHost:      get("SMTP_HOST", ""),
		SMTPPort:      get("SMTP_PORT", "587"),
		SMTPUsername:  get("SMTP_USERNAME", ""),
		SMTPPassword:  get("SMTP_PASSWORD", ""),
		SMTPFrom:      get("SMTP_FROM", ""),
		PortalBaseURL: get("PORTAL_BASE_URL", "http://localhost:5173"),
```
Add the predicate near the other `Config` methods:
```go
// SMTPConfigured reports whether email notifications are wired up. When false,
// the notifier is unset and every notification call is a no-op.
func (c Config) SMTPConfigured() bool { return c.SMTPHost != "" && c.SMTPFrom != "" }
```

- [ ] **Step 4: Add the Mailpit dev service**

In `docker-compose.yml`, add (a catch-all SMTP sink + web inbox; dev/test only, like `echo`):
```yaml
  mailpit:
    image: axllent/mailpit:latest
    ports:
      - "127.0.0.1:8025:8025"
      - "127.0.0.1:1025:1025"
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/config/ && go vet ./internal/config/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go docker-compose.yml
git commit -m "feat(notify): SMTP config + SMTPConfigured + Mailpit dev service"
```

---

## Task 2: notify.Sender — SMTP + buildMessage

**Files:**
- Create: `internal/notify/sender.go`
- Test: `internal/notify/sender_test.go`

**Interfaces:**
- Produces:
  - `type Sender interface { Send(ctx context.Context, to []string, subject, body string) error }`
  - `func NewSMTPSender(host, port, username, password, from string) *SMTPSender` (satisfies `Sender`).
  - `func buildMessage(from string, to []string, subject, body string) []byte` (unexported, pure).

- [ ] **Step 1: Write the failing test**

Create `internal/notify/sender_test.go`:
```go
package notify

import (
	"strings"
	"testing"
)

func TestBuildMessageHeadersAndBody(t *testing.T) {
	msg := string(buildMessage("portal@example.com", []string{"dev@example.com", "ops@example.com"}, "Sujet é", "Bonjour\nLigne 2"))
	for _, want := range []string{
		"From: portal@example.com\r\n",
		"To: dev@example.com, ops@example.com\r\n",
		"Subject: Sujet é\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"Date: ",
		"\r\n\r\nBonjour\r\nLigne 2", // blank line then CRLF-normalized body
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n--- got ---\n%s", want, msg)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/notify/ -run TestBuildMessage -v`
Expected: FAIL — package/`buildMessage` undefined.

- [ ] **Step 3: Implement sender.go**

Create `internal/notify/sender.go`:
```go
// Package notify sends best-effort email notifications for the subscription
// approval loop over bring-your-own SMTP.
package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Sender delivers one email to one or more recipients.
type Sender interface {
	Send(ctx context.Context, to []string, subject, body string) error
}

// SMTPSender sends via net/smtp. PlainAuth is used when a username is set
// (STARTTLS is negotiated by smtp.SendMail when the server advertises it);
// no auth otherwise (e.g. a local Mailpit).
type SMTPSender struct {
	host, port, username, password, from string
}

func NewSMTPSender(host, port, username, password, from string) *SMTPSender {
	return &SMTPSender{host: host, port: port, username: username, password: password, from: from}
}

func (s *SMTPSender) Send(_ context.Context, to []string, subject, body string) error {
	if len(to) == 0 {
		return nil
	}
	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	addr := net.JoinHostPort(s.host, s.port)
	return smtp.SendMail(addr, auth, s.from, to, buildMessage(s.from, to, subject, body))
}

// buildMessage renders an RFC 5322 plaintext message with CRLF line endings.
// The body's \n are normalized to \r\n.
func buildMessage(from string, to []string, subject, body string) []byte {
	crlfBody := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(crlfBody)
	return []byte(b.String())
}

var _ Sender = (*SMTPSender)(nil)
```
NOTE on the timeout: `smtp.SendMail` does not take a context; the ~20s bound from the spec is provided by running `Send` inside the Notifier's best-effort goroutine (Task 4), which isolates any slow/hung send from the request path. The `ctx` parameter is part of the interface for symmetry and future use. `buildMessage` uses `time.Now()` — this is non-deterministic but the test only asserts the `Date:` prefix, not the value.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/notify/ -run TestBuildMessage && go vet ./internal/notify/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/sender.go internal/notify/sender_test.go
git commit -m "feat(notify): SMTP Sender + RFC5322 buildMessage"
```

---

## Task 3: notify.Repo — recipient + name resolution

**Files:**
- Create: `internal/notify/repo.go`
- Test: `internal/notify/repo_test.go`

**Interfaces:**
- Produces (on `*Repo` with `func NewRepo(pool *pgxpool.Pool) *Repo`):
  - `OwnerEmailForApp(ctx, appID int64) (email, appName string, err error)`
  - `AdminEmails(ctx) ([]string, error)`
  - `ProductName(ctx, productID int64) (string, error)`
  - `PlanName(ctx, planID int64) (string, error)`

- [ ] **Step 1: Write the failing DB test**

Create `internal/notify/repo_test.go` (mirror the live-DB setup of `internal/applications/repo_test.go`: skip/connect via `db.Connect`, `db.Migrate`, seed via the pool, `t.Cleanup`):
```go
package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	suf := time.Now().Format("150405.000000000")
	var uid, appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','Dev','developer') RETURNING id`,
		"owner+"+suf+"@e.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name) VALUES($1,$2) RETURNING id`,
		uid, "App "+suf).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, NewRepo(pool), appID
}

func TestOwnerEmailAndAdmins(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	email, name, err := repo.OwnerEmailForApp(ctx, appID)
	if err != nil || email == "" || name == "" {
		t.Fatalf("OwnerEmailForApp = %q,%q,%v", email, name, err)
	}
	admins, err := repo.AdminEmails(ctx)
	if err != nil {
		t.Fatalf("AdminEmails: %v", err)
	}
	for _, a := range admins {
		if a == "" {
			t.Fatal("empty admin email")
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/notify/ -run TestOwnerEmailAndAdmins -v`
Expected: FAIL — package/methods undefined.

- [ ] **Step 3: Implement repo.go**

Create `internal/notify/repo.go`:
```go
package notify

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// OwnerEmailForApp returns the app owner's email + the app name.
func (r *Repo) OwnerEmailForApp(ctx context.Context, appID int64) (string, string, error) {
	var email, name string
	err := r.pool.QueryRow(ctx,
		`SELECT u.email, a.name FROM applications a JOIN users u ON u.id = a.owner_id WHERE a.id=$1`, appID).
		Scan(&email, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return email, name, err
}

// AdminEmails returns the emails of all admin users.
func (r *Repo) AdminEmails(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT email FROM users WHERE role='admin'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) ProductName(ctx context.Context, productID int64) (string, error) {
	var n string
	err := r.pool.QueryRow(ctx, `SELECT name FROM api_products WHERE id=$1`, productID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return n, err
}

func (r *Repo) PlanName(ctx context.Context, planID int64) (string, error) {
	var n string
	err := r.pool.QueryRow(ctx, `SELECT name FROM plans WHERE id=$1`, planID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return n, err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/notify/ && go vet ./internal/notify/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/repo.go internal/notify/repo_test.go
git commit -m "feat(notify): recipient/name resolution repo"
```

---

## Task 4: notify.Notifier — templates + deliver + async methods

**Files:**
- Create: `internal/notify/notifier.go`
- Test: `internal/notify/notifier_test.go`

**Interfaces:**
- Consumes: `Sender` (Task 2); the `Repo` methods (Task 3).
- Produces:
  - `type Resolver interface { OwnerEmailForApp(...); AdminEmails(...); ProductName(...); PlanName(...) }` (satisfied by `*Repo`; lets tests fake it).
  - `func NewNotifier(sender Sender, repo Resolver, baseURL string) *Notifier`
  - `SubscriptionRequested(appID, productID, planID int64)`,
    `SubscriptionApproved(appID, productID, planID int64)`,
    `SubscriptionRejected(appID, productID int64)` — async, best-effort.
  - `deliver(kind string, appID, productID, planID int64)` — unexported, synchronous (the goroutine body; tests call it directly).

- [ ] **Step 1: Write the failing test**

Create `internal/notify/notifier_test.go`:
```go
package notify

import (
	"context"
	"strings"
	"testing"
)

type fakeSender struct {
	to      []string
	subject string
	body    string
	calls   int
}

func (f *fakeSender) Send(_ context.Context, to []string, subject, body string) error {
	f.calls++
	f.to, f.subject, f.body = to, subject, body
	return nil
}

type fakeResolver struct{}

func (fakeResolver) OwnerEmailForApp(_ context.Context, _ int64) (string, string, error) {
	return "dev@example.com", "Mon App", nil
}
func (fakeResolver) AdminEmails(_ context.Context) ([]string, error) {
	return []string{"admin@example.com"}, nil
}
func (fakeResolver) ProductName(_ context.Context, _ int64) (string, error) { return "Orders API", nil }
func (fakeResolver) PlanName(_ context.Context, _ int64) (string, error)    { return "Gold", nil }

func TestDeliverApprovedToOwner(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, fakeResolver{}, "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3)
	if fs.calls != 1 || len(fs.to) != 1 || fs.to[0] != "dev@example.com" {
		t.Fatalf("to=%v calls=%d", fs.to, fs.calls)
	}
	if !strings.Contains(fs.body, "Orders API") || !strings.Contains(fs.body, "Mon App") ||
		!strings.Contains(fs.body, "https://portal.example/applications") {
		t.Fatalf("body missing details: %s", fs.body)
	}
}

func TestDeliverRequestedToAdmins(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, fakeResolver{}, "https://portal.example")
	n.deliver(kindRequested, 1, 2, 3)
	if fs.calls != 1 || len(fs.to) != 1 || fs.to[0] != "admin@example.com" {
		t.Fatalf("to=%v", fs.to)
	}
	if !strings.Contains(fs.body, "/admin/approvals") {
		t.Fatalf("admin body missing link: %s", fs.body)
	}
}

func TestDeliverSkipsEmptyRecipients(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, emptyResolver{}, "https://portal.example")
	n.deliver(kindApproved, 1, 2, 3) // owner email "" -> no send
	if fs.calls != 0 {
		t.Fatalf("expected no send, got %d", fs.calls)
	}
}

type emptyResolver struct{ fakeResolver }

func (emptyResolver) OwnerEmailForApp(_ context.Context, _ int64) (string, string, error) {
	return "", "", nil
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/notify/ -run TestDeliver -v`
Expected: FAIL — `Notifier`/`kind*` undefined.

- [ ] **Step 3: Implement notifier.go**

Create `internal/notify/notifier.go`:
```go
package notify

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	kindRequested = "requested"
	kindApproved  = "approved"
	kindRejected  = "rejected"
)

const deliverTimeout = 20 * time.Second

// Resolver resolves recipient emails + display names (satisfied by *Repo).
type Resolver interface {
	OwnerEmailForApp(ctx context.Context, appID int64) (string, string, error)
	AdminEmails(ctx context.Context) ([]string, error)
	ProductName(ctx context.Context, productID int64) (string, error)
	PlanName(ctx context.Context, planID int64) (string, error)
}

// Notifier renders + sends the approval-loop emails, best-effort and async.
type Notifier struct {
	sender  Sender
	repo    Resolver
	baseURL string
}

func NewNotifier(sender Sender, repo Resolver, baseURL string) *Notifier {
	return &Notifier{sender: sender, repo: repo, baseURL: baseURL}
}

func (n *Notifier) SubscriptionRequested(appID, productID, planID int64) {
	go n.deliver(kindRequested, appID, productID, planID)
}
func (n *Notifier) SubscriptionApproved(appID, productID, planID int64) {
	go n.deliver(kindApproved, appID, productID, planID)
}
func (n *Notifier) SubscriptionRejected(appID, productID int64) {
	go n.deliver(kindRejected, appID, productID, 0)
}

// deliver resolves recipients, renders the template, and sends. Synchronous and
// best-effort: all errors are logged and dropped; empty recipients are skipped.
func (n *Notifier) deliver(kind string, appID, productID, planID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()

	product, err := n.repo.ProductName(ctx, productID)
	if err != nil {
		log.Printf("notify: product name (product=%d): %v", productID, err)
	}
	if product == "" {
		product = "une API"
	}
	owner, appName, err := n.repo.OwnerEmailForApp(ctx, appID)
	if err != nil {
		log.Printf("notify: owner email (app=%d): %v", appID, err)
	}
	if appName == "" {
		appName = "votre application"
	}

	var to []string
	var subject, body string
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
		subject = "Nouvelle demande d'abonnement à examiner"
		body = fmt.Sprintf("Une nouvelle demande d'abonnement attend votre validation.\n\nApplication : %s\nAPI : %s\nForfait : %s\n\nExaminez-la ici : %s/admin/approvals\n",
			appName, product, plan, n.baseURL)
	case kindApproved:
		to = []string{owner}
		plan, _ := n.repo.PlanName(ctx, planID)
		if plan == "" {
			plan = "votre forfait"
		}
		subject = "Votre abonnement est approuvé"
		body = fmt.Sprintf("Bonne nouvelle ! L'abonnement de %s à %s (%s) est approuvé.\n\nRetrouvez vos identifiants ici : %s/applications\n",
			appName, product, plan, n.baseURL)
	case kindRejected:
		to = []string{owner}
		subject = "Votre demande d'abonnement a été refusée"
		body = fmt.Sprintf("La demande d'abonnement de %s à %s n'a pas été approuvée.\n\nParcourez le catalogue : %s/\n",
			appName, product, n.baseURL)
	default:
		return
	}

	// Drop empty recipients (e.g. a missing owner email or no admins).
	clean := to[:0]
	for _, addr := range to {
		if addr != "" {
			clean = append(clean, addr)
		}
	}
	if len(clean) == 0 {
		return
	}
	if err := n.sender.Send(ctx, clean, subject, body); err != nil {
		log.Printf("notify: send %q to %v: %v", kind, clean, err)
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/notify/ && go vet ./internal/notify/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notify/notifier.go internal/notify/notifier_test.go
git commit -m "feat(notify): Notifier templates + async best-effort deliver"
```

---

## Task 5: Service integration — SetNotifier + hooks

**Files:**
- Modify: `internal/subscriptions/service.go`
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Consumes: the `notify.Notifier` methods (Task 4) — via a narrow interface declared in `subscriptions` (so `subscriptions` does NOT import `notify`).
- Produces: `subscriptions.Notifier` interface; `Service.SetNotifier(Notifier)`.

- [ ] **Step 1: Write the failing test**

In `internal/subscriptions/service_test.go` add a fake notifier + assertions:
```go
type fakeNotifier struct{ requested, approved, rejected int }

func (f *fakeNotifier) SubscriptionRequested(_, _, _ int64) { f.requested++ }
func (f *fakeNotifier) SubscriptionApproved(_, _, _ int64)  { f.approved++ }
func (f *fakeNotifier) SubscriptionRejected(_, _ int64)     { f.rejected++ }

func TestSubscribeNotifiesAdmins(t *testing.T) {
	store := newMemStore() // product 3 key-auth published, plan 2
	fn := &fakeNotifier{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.SetNotifier(fn)
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if fn.requested != 1 {
		t.Fatalf("requested=%d want 1", fn.requested)
	}
}

func TestApproveRejectNotifyDeveloper(t *testing.T) {
	store := newMemStore()
	fn := &fakeNotifier{}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.SetNotifier(fn)
	// seed a pending subscription record the store can approve/reject (mirror the
	// existing approve/reject tests' record setup in this file)
	store.records[10] = &SubscriptionRecord{ID: 10, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusPending}
	if err := svc.Approve(context.Background(), 10); err != nil {
		t.Fatalf("approve: %v", err)
	}
	store.records[11] = &SubscriptionRecord{ID: 11, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusPending}
	if err := svc.Reject(context.Background(), 11); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if fn.approved != 1 || fn.rejected != 1 {
		t.Fatalf("approved=%d rejected=%d", fn.approved, fn.rejected)
	}
}

func TestNilNotifierSafe(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	// no SetNotifier — must not panic
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
}
```
(Adapt the record/store seeding to the file's real `memStore` shape — Approve reads `GetSubscription`/`GetPlan`/`GetOrCreateCredential`; mirror how the existing `TestApprove*` tests seed it.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run 'TestSubscribeNotifies|TestApproveRejectNotify|TestNilNotifier' -v`
Expected: FAIL — `SetNotifier`/`Notifier` undefined.

- [ ] **Step 3: Add the interface, field, setter, and hooks**

In `internal/subscriptions/service.go`:
- Add the narrow interface near `EventLogger`:
```go
// Notifier sends approval-loop emails (satisfied by *notify.Notifier). nil =
// disabled. Calls are best-effort and async (the implementation never blocks).
type Notifier interface {
	SubscriptionRequested(appID, productID, planID int64)
	SubscriptionApproved(appID, productID, planID int64)
	SubscriptionRejected(appID, productID int64)
}
```
- Add the field to `Service` (next to `events EventLogger`): `notifier Notifier`.
- Add the setter (near `SetUsageReader`-style setters if any, else after `NewService`):
```go
// SetNotifier wires email notifications. Left unset (nil) = disabled.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }
```
- Add the hook calls (guarded), right after the matching `logEvent`:
  - In `Subscribe`, the **oauth2** path (after `s.logEvent(ctx, appID, events.KindSubscribed, &productID, &planID)` and before `return Credential{}, nil`):
    ```go
    		if s.notifier != nil { s.notifier.SubscriptionRequested(appID, productID, planID) }
    ```
  - In `Subscribe`, the **key-auth** path (after the same `logEvent`, before `return cred, nil`):
    ```go
    	if s.notifier != nil { s.notifier.SubscriptionRequested(appID, productID, planID) }
    ```
  - In `Approve`, BOTH paths (after each `s.logEvent(ctx, rec.AppID, events.KindApproved, &rec.ProductID, &rec.PlanID)`):
    ```go
    		if s.notifier != nil { s.notifier.SubscriptionApproved(rec.AppID, rec.ProductID, rec.PlanID) }
    ```
  - In `Reject` (after `s.logEvent(ctx, rec.AppID, events.KindRejected, &rec.ProductID, nil)`):
    ```go
    	if s.notifier != nil { s.notifier.SubscriptionRejected(rec.AppID, rec.ProductID) }
    ```

- [ ] **Step 4: Run to verify it passes + full package**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go vet ./internal/subscriptions/`
Expected: PASS (existing tests unaffected — the notifier is nil in them).

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/service.go internal/subscriptions/service_test.go
git commit -m "feat(notify): subscriptions Service notifies the approval loop"
```

---

## Task 6: Wire server + full suite + live verification

**Files:**
- Modify: `internal/server/server.go`
- Test: full backend suite + live (Mailpit)

**Interfaces:**
- Consumes: `config.SMTPConfigured`, `notify.NewSMTPSender`/`NewRepo`/`NewNotifier`, `subscriptions.Service.SetNotifier`.

- [ ] **Step 1: Wire in server.go**

In `internal/server/server.go` `New`, after `subSvc` is built (and after its other setters):
```go
	if cfg.SMTPConfigured() {
		sender := notify.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
		subSvc.SetNotifier(notify.NewNotifier(sender, notify.NewRepo(pool), cfg.PortalBaseURL))
	}
```
Add the import `apisix-portal/internal/notify`. (`pool` is the `*pgxpool.Pool` already in scope in `New`.)

- [ ] **Step 2: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(notify): wire SMTP notifier into the server"
```

- [ ] **Step 4: Live verification (Mailpit)**

1. `docker compose up -d mailpit` (SMTP `:1025`, web inbox `http://localhost:8025`). Restart the portal with `SMTP_HOST=localhost SMTP_PORT=1025 SMTP_FROM=portal@local PORTAL_BASE_URL=http://localhost:5173` (plus the usual dev env). (If the portal runs on the host and Mailpit's `1025` is host-published, `SMTP_HOST=localhost` works.)
2. As a developer, subscribe an app to a published product → check Mailpit: an email to the admin address(es), subject "Nouvelle demande d'abonnement à examiner", body naming the app/API/plan + a `/admin/approvals` link.
3. As admin, approve that subscription → the developer (app owner) gets "Votre abonnement est approuvé" with an `/applications` link. Reject another pending one → "Votre demande d'abonnement a été refusée".
4. Confirm all three in the Mailpit inbox (`:8025`) with correct recipients/subjects/links. **Look at the inbox.**

---

## Self-Review notes

- **Spec coverage:** SMTP config + SMTPConfigured + Mailpit (T1) ✅; Sender/SMTP + buildMessage (T2) ✅; recipient/name repo (T3) ✅; Notifier templates + async best-effort deliver + empty-recipient skip (T4) ✅; Service narrow-interface + SetNotifier + Subscribe/Approve/Reject hooks (T5) ✅; wiring + live Mailpit (T6) ✅. Out-of-scope items (HTML, prefs, digests, outbox, non-approval events) intentionally absent.
- **Type consistency:** `Sender.Send(ctx, to, subject, body)` (T2 defines, T4 calls); `Resolver` methods identical between the interface (T4), `*Repo` (T3), and the fake (T4 test); `Notifier` narrow interface (T5) matches `*notify.Notifier`'s method set (T4) exactly — `SubscriptionRequested/Approved(appID,productID,planID)`, `SubscriptionRejected(appID,productID)`; `NewNotifier(sender, repo, baseURL)`, `NewSMTPSender(host,port,username,password,from)`, `NewRepo(pool)` consistent (T2/T3/T4 → T6).
- **Implementer notes:** `subscriptions` must NOT import `notify` — the `Notifier` interface is declared in `subscriptions` and `*notify.Notifier` satisfies it structurally (same pattern as `EventLogger`). Adapt the T5 service tests to the real `memStore`/record seeding used by the existing `TestApprove*`/`TestReject*` tests. `smtp.SendMail` has no ctx timeout — the goroutine + `deliverTimeout` ctx bound the DB resolution; a hung SMTP dial is isolated to the best-effort goroutine (documented in T2).
