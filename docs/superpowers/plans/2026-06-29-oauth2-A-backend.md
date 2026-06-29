# OAuth2 for Consumers — Plan 1 (Backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin mark a product `oauth2` so its APISIX route validates bearer JWTs against a configured OIDC issuer and admits only the client_ids of its active subscribers — the OAuth2 analogue of today's key-auth + consumer-restriction model.

**Architecture:** New config (`OIDC_ISSUER`, `OIDC_CLIENT_ID_CLAIM`). A new `apisix.Gateway.EnsureOAuthRoute` builds a route with `openid-connect` (bearer_only, JWKS) + a generated `serverless-pre-function` that 403s unless the token's client_id claim is in a portal-built allow-list. Products gain `auth_type`; apps gain `oidc_client_id`. `subscriptions` provisioning branches on `auth_type`; the existing lifecycle hooks inherit the branch.

**Tech Stack:** Go 1.25 (chi, pgx), APISIX 3.9.1 (openid-connect + serverless-pre-function plugins), docker-compose.

## Global Constraints

- Module `apisix-portal`. OAuth2 route composition in `internal/apisix`; product `auth_type` in `internal/admin`/`internal/catalog`; app `oidc_client_id` + provisioning in `internal/subscriptions`; config in `internal/config`.
- **OAuth2-route whitelist invariant:** an `oauth2` product's route admits exactly the `client_id`s of its **active** subscribers whose app has a non-empty `oidc_client_id`.
- **client_id charset guard (security):** every client_id is validated against `^[A-Za-z0-9._:@-]{1,200}$` at the portal boundary before it can reach the gateway; rejected otherwise. client_ids are emitted into the serverless Lua as **table keys**, never concatenated into code.
- Per-product auth: `auth_type ∈ {'key-auth','oauth2'}`, default `'key-auth'`. key-auth flow is UNCHANGED. `oauth2` apps get NO APISIX consumer and NO key.
- OAuth2 is selectable only when `config.OIDCConfigured()` (`OIDC_ISSUER != ""`); admin rejects `oauth2` with 400 otherwise. The portal never stores a client secret.
- Tests: backend `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w` touched files; `go vet ./...`.

---

## Task 1: Config + apisix EnsureOAuthRoute

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `internal/apisix/gateway.go` (interface), `internal/apisix/client.go` (impl), `internal/apisix/fake.go` (fake)
- Test: `internal/apisix/client_test.go`, `internal/apisix/fake.go` (FakeRoute already records plugins? — see below)

**Interfaces:**
- Produces: `Config.OIDCIssuer string`, `Config.OIDCClientIDClaim string`, `func (c Config) OIDCConfigured() bool`; `Gateway.EnsureOAuthRoute(ctx, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error`; `apisix.ValidClientID(s string) bool`.

- [ ] **Step 1: Config fields + predicate**

In `internal/config/config.go` add to `Config`: `OIDCIssuer string`, `OIDCClientIDClaim string`. In `Load()`:
```go
		OIDCIssuer:        get("OIDC_ISSUER", ""),
		OIDCClientIDClaim: get("OIDC_CLIENT_ID_CLAIM", "azp"),
```
Add:
```go
// OIDCConfigured reports whether OAuth2 (bring-your-own OIDC) is wired up.
func (c Config) OIDCConfigured() bool { return c.OIDCIssuer != "" }
```
In `internal/config/config_test.go` add a test: with `OIDC_ISSUER` unset, `OIDCConfigured()` is false and `OIDCClientIDClaim=="azp"`; with `OIDC_ISSUER="https://idp.example"` set (via `t.Setenv`), `OIDCConfigured()` is true. (Mirror the existing `TestSandboxConfigDefaultsAndPredicate` style.)

- [ ] **Step 2: Write the failing apisix test**

In `internal/apisix/client_test.go` add (these are pure unit tests on the route body builder — no live APISIX):
```go
func TestOAuthRouteBodyHasOIDCAndWhitelist(t *testing.T) {
	body, err := oauthRouteBody("/orders", "echo:8080", "https://idp.example/realms/dev", "azp", []string{"client-a", "client-b"})
	if err != nil {
		t.Fatalf("oauthRouteBody: %v", err)
	}
	plugins := body["plugins"].(map[string]any)
	oidc, ok := plugins["openid-connect"].(map[string]any)
	if !ok || oidc["bearer_only"] != true {
		t.Fatalf("openid-connect missing/!bearer_only: %v", plugins["openid-connect"])
	}
	if d, _ := oidc["discovery"].(string); d != "https://idp.example/realms/dev/.well-known/openid-configuration" {
		t.Fatalf("discovery = %v", oidc["discovery"])
	}
	sp, ok := plugins["serverless-pre-function"].(map[string]any)
	if !ok {
		t.Fatalf("serverless-pre-function missing")
	}
	fns := sp["functions"].([]string)
	if len(fns) != 1 || !strings.Contains(fns[0], `["client-a"]=true`) || !strings.Contains(fns[0], `["client-b"]=true`) {
		t.Fatalf("allow table missing client ids: %s", fns[0])
	}
	if !strings.Contains(fns[0], `claims["azp"]`) {
		t.Fatalf("claim name not wired: %s", fns[0])
	}
	// no key-auth / consumer-restriction on an oauth route
	if _, has := plugins["key-auth"]; has {
		t.Fatalf("oauth route must not carry key-auth")
	}
}

func TestValidClientIDRejectsInjection(t *testing.T) {
	for _, good := range []string{"client-a", "svc.account@corp", "ABC_123:role"} {
		if !ValidClientID(good) {
			t.Errorf("ValidClientID(%q) = false, want true", good)
		}
	}
	for _, bad := range []string{`a"]=true os.execute("x")--`, "a b", "", "a\nb", strings.Repeat("x", 201)} {
		if ValidClientID(bad) {
			t.Errorf("ValidClientID(%q) = true, want false", bad)
		}
	}
}
```
(Add `"strings"` to the test imports if not present.)

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/apisix/ -run 'TestOAuthRouteBody|TestValidClientID' -v`
Expected: FAIL — `oauthRouteBody`/`ValidClientID` undefined.

- [ ] **Step 4: Implement ValidClientID + oauthRouteBody + EnsureOAuthRoute**

In `internal/apisix/client.go` add (reuse the existing `parseUpstream` + the proxy-rewrite prefix logic from `routeBody`):
```go
var clientIDRe = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,200}$`)

// ValidClientID guards every OIDC client id before it is embedded (as a Lua
// table key) in a route's serverless-pre-function. Strict charset = no Lua
// injection is possible from a client id.
func ValidClientID(s string) bool { return clientIDRe.MatchString(s) }

func (c *Client) EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error {
	body, err := oauthRouteBody(contextPath, upstreamURL, issuer, claimName, allowedClientIDs)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/routes/"+routeID, body)
}

// oauthRouteBody builds an OAuth2 product route: openid-connect validates the
// bearer JWT against the issuer's JWKS (bearer_only); a serverless-pre-function
// then 403s unless the token's claimName claim is in the allow-list of the
// product's active subscribers' client ids. Same context-prefix strip as routeBody.
func oauthRouteBody(contextPath, upstreamURL, issuer, claimName string, allowed []string) (map[string]any, error) {
	scheme, node, err := parseUpstream(upstreamURL)
	if err != nil {
		return nil, err
	}
	if !clientIDRe.MatchString(claimName) { // claim name is config-controlled; guard it too
		return nil, fmt.Errorf("bad oidc claim name %q", claimName)
	}
	var b strings.Builder
	for _, cid := range allowed {
		if !ValidClientID(cid) {
			return nil, fmt.Errorf("bad client id %q", cid)
		}
		fmt.Fprintf(&b, "[%q]=true,", cid)
	}
	lua := `return function(conf, ctx)
  local core = require("apisix.core")
  local hdr = core.request.header(ctx, "Authorization")
  if not hdr then return end
  local tok = hdr:match("[Bb]earer%s+(.+)")
  if not tok then return end
  local payload = tok:match("^[^.]+%.([^.]+)")
  if not payload then return 403, {message="forbidden"} end
  payload = payload:gsub("-","+"):gsub("_","/")
  local pad = #payload % 4
  if pad > 0 then payload = payload .. string.rep("=", 4 - pad) end
  local raw = ngx.decode_base64(payload)
  if not raw then return 403, {message="forbidden"} end
  local claims = core.json.decode(raw)
  if not claims then return 403, {message="forbidden"} end
  local allow = {` + b.String() + `}
  local cid = claims["` + claimName + `"]
  if not cid or not allow[cid] then return 403, {message="not subscribed"} end
end`
	prefix := regexp.QuoteMeta(strings.TrimRight(contextPath, "/"))
	return map[string]any{
		"uris": []string{contextPath, contextPath + "/*"},
		"upstream": map[string]any{
			"type": "roundrobin", "scheme": scheme, "nodes": map[string]int{node: 1},
		},
		"plugins": map[string]any{
			"openid-connect": map[string]any{
				"bearer_only": true,
				"discovery":   strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration",
				"use_jwks":    true,
			},
			"serverless-pre-function": map[string]any{
				"phase":     "access",
				"functions": []string{lua},
			},
			"proxy-rewrite": map[string]any{"regex_uri": []string{"^" + prefix + "/?(.*)$", "/$1"}},
		},
	}, nil
}
```
**Security note (carry into review):** `openid-connect` enforces the token's signature/issuer/expiry; the serverless function only reads the claim for *authorization*. Even though serverless may run before openid-connect (priority), BOTH must pass, so a forged token with an allowed `azp` still fails openid-connect's signature check. The `ValidClientID`/claim-name guards prevent Lua injection.

In `internal/apisix/gateway.go` add to the `Gateway` interface:
```go
	// EnsureOAuthRoute creates/updates an OAuth2 product route: openid-connect
	// (bearer_only, JWKS from issuer) + a serverless-pre-function whitelisting the
	// token's claimName claim against allowedClientIDs.
	EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error
```

In `internal/apisix/fake.go` add (record enough to assert in service tests — store the allowed ids + issuer on the FakeRoute, or a parallel map):
```go
// add fields to Fake if helpful, e.g. OAuthRoutes map[string]FakeOAuthRoute
func (f *Fake) EnsureOAuthRoute(_ context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowed []string) error {
	if f.fail { // mirror however Fake signals failure, if it does
	}
	f.Routes[routeID] = FakeRoute{URI: contextPath, Upstream: upstreamURL, Allowed: append([]string(nil), allowed...)}
	return nil
}
```
(Reuse the existing `FakeRoute` struct so service tests can assert `Routes[RouteID(id)].Allowed` == the client_ids. If `FakeRoute` lacks a field to distinguish oauth from key-auth routes, add a `OAuth bool` field and set it true here; update `EnsureRoute` to set it false. Keep it minimal.)

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/apisix/ ./internal/config/ && go vet ./internal/apisix/ ./internal/config/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/apisix/gateway.go internal/apisix/client.go internal/apisix/client_test.go internal/apisix/fake.go
git commit -m "feat(oauth2): OIDC config + apisix EnsureOAuthRoute (openid-connect + claim whitelist)"
```

---

## Task 2: Migration 0012 + product auth_type + app oidc_client_id column

**Files:**
- Create: `internal/db/migrations/0012_oauth2.sql`
- Modify: `internal/admin/product.go` (`AuthType` + validate), `internal/admin/repo.go` (`productCols`, scan, Create, Update)
- Modify: `internal/catalog/product.go` (`AuthType`), `internal/catalog/repo.go` (`baseSelect`, scan)
- Modify: `internal/subscriptions/service.go` (`ProductInfo.AuthType`), `internal/subscriptions/repo.go` (`GetProduct`)
- Test: `internal/admin/product_test.go`

**Interfaces:**
- Produces: `admin.Product.AuthType string` (json `authType`); `catalog.Product.AuthType string` (json `authType`); `subscriptions.ProductInfo.AuthType string`; columns `api_products.auth_type`, `applications.oidc_client_id`.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0012_oauth2.sql`:
```sql
-- Per-product auth method + per-app OIDC client id (plaintext; not a secret).
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS auth_type TEXT NOT NULL DEFAULT 'key-auth'
    CHECK (auth_type IN ('key-auth','oauth2'));
ALTER TABLE applications ADD COLUMN IF NOT EXISTS oidc_client_id TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Write the failing admin test**

In `internal/admin/product_test.go` add (the project's product validation is `validate(allowPrivate bool) string` — empty string = valid; mirror that):
```go
func TestValidateRejectsBadAuthType(t *testing.T) {
	p := Product{Name: "X", Slug: "x", Category: "C", ContextPath: "/x", AuthType: "bogus"}
	if p.validate(false) == "" {
		t.Fatal("expected invalid auth_type to fail validation")
	}
}

func TestValidateAcceptsKnownAuthTypes(t *testing.T) {
	for _, at := range []string{"", "key-auth", "oauth2"} { // "" defaults to key-auth at the DB
		p := Product{Name: "X", Slug: "x", Category: "C", ContextPath: "/x", AuthType: at}
		if msg := p.validate(false); msg != "" {
			t.Fatalf("authType %q should be valid: %s", at, msg)
		}
	}
}
```
(Confirm the real method name/return — sandbox Task 2 established it is `validate(bool) string`.)

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/admin/ -run TestValidate -v`
Expected: FAIL — `AuthType` undefined.

- [ ] **Step 4: Add the field, validation, persistence, and reads**

`internal/admin/product.go`: add `AuthType string \`json:"authType"\`` to `Product`; in `validate`, after the upstream checks add:
```go
	if p.AuthType != "" && p.AuthType != "key-auth" && p.AuthType != "oauth2" {
		return "authType must be key-auth or oauth2"
	}
```
`internal/admin/repo.go`: add `auth_type` to `productCols` (after `published` is fine, but keep scan order consistent), scan `&p.AuthType`, and include it in `Create` + `Update` (column + placeholder + arg — renumber `$` placeholders carefully, mirroring the sandbox column add). On Create, when `p.AuthType==""` the DB default `'key-auth'` applies (or set it explicitly to `'key-auth'`).

`internal/catalog/product.go`: add `AuthType string \`json:"authType"\``. `internal/catalog/repo.go`: add `auth_type` to `baseSelect` and `&p.AuthType` to `scanProducts` (matching column order).

`internal/subscriptions/service.go`: add `AuthType string` to `ProductInfo`. `internal/subscriptions/repo.go` `GetProduct`: add `auth_type` to the SELECT and scan into `p.AuthType` (place it consistently in the column/scan order).

- [ ] **Step 5: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/admin/ ./internal/catalog/ ./internal/subscriptions/ && go vet ./internal/admin/ ./internal/catalog/ ./internal/subscriptions/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0012_oauth2.sql internal/admin/product.go internal/admin/repo.go internal/admin/product_test.go internal/catalog/product.go internal/catalog/repo.go internal/subscriptions/service.go internal/subscriptions/repo.go
git commit -m "feat(oauth2): auth_type on products + oidc_client_id column + reads"
```

---

## Task 3: Subscriptions repo — OAuth queries + SetOIDCClientID

**Files:**
- Modify: `internal/subscriptions/repo.go`
- Modify: `internal/subscriptions/service.go` (extend `Store`)
- Test: `internal/subscriptions/repo_oauth_test.go` (new, DB-backed)

**Interfaces:**
- Produces on `*Repo` (added to `Store`):
  - `OAuthClientsForProduct(ctx, productID int64) ([]string, error)` — `oidc_client_id`s of active subscribers whose app has a non-empty `oidc_client_id`.
  - `OAuthProductsForApp(ctx, appID int64) ([]ProductInfo, error)` — `oauth2` products the app is actively subscribed to (id, context_path, upstream, auth_type).
  - `GetAppOIDCClientID(ctx, appID int64) (string, error)` — current value (`""` when unset; `ErrNotFound` if no app row).
  - `SetAppOIDCClientID(ctx, appID int64, clientID string) error` — persist (`ErrNotFound` when no app row).

- [ ] **Step 1: Write the failing DB test**

Create `internal/subscriptions/repo_oauth_test.go` (mirror `repo_sandbox_test.go`'s setup — `crypto.New(config.DevCredentialEncKey)`, `db.Connect`/`db.Migrate`, seed user/app/product/plan/subscription, FK-ordered `t.Cleanup`; seed the product with `auth_type='oauth2'`):
```go
func TestOAuthClientWhitelistAndProductsForApp(t *testing.T) {
	ctx, repo, appID, pid := oauthTestRepo(t) // product seeded auth_type='oauth2', active sub for appID
	// no client id yet → not in whitelist
	if ids, err := repo.OAuthClientsForProduct(ctx, pid); err != nil || len(ids) != 0 {
		t.Fatalf("whitelist before = %v, %v", ids, err)
	}
	if err := repo.SetAppOIDCClientID(ctx, appID, "client-xyz"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, err := repo.GetAppOIDCClientID(ctx, appID); err != nil || got != "client-xyz" {
		t.Fatalf("get = %q, %v", got, err)
	}
	ids, err := repo.OAuthClientsForProduct(ctx, pid)
	if err != nil || len(ids) != 1 || ids[0] != "client-xyz" {
		t.Fatalf("whitelist after = %v, %v", ids, err)
	}
	prods, err := repo.OAuthProductsForApp(ctx, appID)
	if err != nil || len(prods) != 1 || prods[0].AuthType != "oauth2" {
		t.Fatalf("OAuthProductsForApp = %+v, %v", prods, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestOAuthClientWhitelist -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the repo methods**

Add to `internal/subscriptions/repo.go`:
```go
// OAuthClientsForProduct returns the client ids of active subscribers whose app
// has a non-empty oidc_client_id (the OAuth2 route whitelist).
func (r *Repo) OAuthClientsForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT a.oidc_client_id FROM subscriptions s
		   JOIN applications a ON a.id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active' AND a.oidc_client_id <> ''`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// OAuthProductsForApp returns the oauth2 products the app is actively subscribed to.
func (r *Repo) OAuthProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT p.id, p.context_path, p.upstream_url, p.auth_type
		   FROM subscriptions s JOIN api_products p ON p.id = s.api_product_id
		 WHERE s.application_id=$1 AND s.status='active' AND p.auth_type='oauth2'`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductInfo
	for rows.Next() {
		var p ProductInfo
		if err := rows.Scan(&p.ID, &p.ContextPath, &p.Upstream, &p.AuthType); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) GetAppOIDCClientID(ctx context.Context, appID int64) (string, error) {
	var cid string
	err := r.pool.QueryRow(ctx, `SELECT oidc_client_id FROM applications WHERE id=$1`, appID).Scan(&cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return cid, err
}

func (r *Repo) SetAppOIDCClientID(ctx context.Context, appID int64, clientID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE applications SET oidc_client_id=$2 WHERE id=$1`, appID, clientID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```
Add the four signatures to the `Store` interface in `internal/subscriptions/service.go`. If the fake `memStore` in `service_test.go` then fails to compile, add trivial stubs there now (fleshed out in Task 4) so this package builds (same approach as the sandbox Task 3).

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestOAuthClientWhitelist && go vet ./internal/subscriptions/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/repo.go internal/subscriptions/service.go internal/subscriptions/repo_oauth_test.go
git commit -m "feat(oauth2): subscriptions OAuth client whitelist + SetOIDCClientID repo"
```

---

## Task 4: Service — auth_type branch + SetOIDCClientID + lifecycle

**Files:**
- Modify: `internal/subscriptions/service.go`
- Modify: `internal/subscriptions/service_test.go` (flesh out memStore + tests)
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Produces: `Service.ConfigureOIDC(issuer, claim string)` (setter; empty issuer = OAuth disabled); `Service.SetOIDCClientID(ctx, appID int64, clientID string) (err)`; `reprovisionRoute` branches on `prod.AuthType`; `Subscribe` issues no credential for oauth2; errors `ErrOIDCNotConfigured`, `ErrInvalidClientID`.

- [ ] **Step 1: Write the failing service test**

In `internal/subscriptions/service_test.go`: flesh out the `memStore` OAuth stubs from Task 3 with real maps (`oidcClientIDs map[int64]string`, `oauthWhitelist map[int64][]string`, `oauthProducts map[int64][]ProductInfo`), make `GetProduct` return `AuthType` for the test product, and add:
```go
func TestReprovisionBranchesToOAuthRoute(t *testing.T) {
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	store.oauthWhitelist[9] = []string{"client-a"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")
	if err := svc.ReprovisionRoute(context.Background(), 9); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(9)]
	if !ok || len(r.Allowed) != 1 || r.Allowed[0] != "client-a" {
		t.Fatalf("oauth route not provisioned with whitelist: %+v", r)
	}
	// (if FakeRoute has an OAuth bool, assert r.OAuth == true)
}

func TestSetOIDCClientIDReprovisions(t *testing.T) {
	store := newMemStore()
	store.products[9] = ProductInfo{ID: 9, ContextPath: "/orders", Upstream: "echo:8080", AuthType: "oauth2"}
	store.oauthProducts[42] = []ProductInfo{store.products[9]}
	store.oauthWhitelist[9] = []string{"client-a"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example/realms/dev", "azp")
	if err := svc.SetOIDCClientID(context.Background(), 42, "client-a"); err != nil {
		t.Fatalf("SetOIDCClientID: %v", err)
	}
	if store.oidcClientIDs[42] != "client-a" {
		t.Fatalf("client id not persisted")
	}
	if _, ok := gw.Routes[RouteID(9)]; !ok {
		t.Fatalf("route not reprovisioned")
	}
}

func TestSetOIDCClientIDRejectsBadCharset(t *testing.T) {
	svc := NewService(newMemStore(), apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example", "azp")
	if err := svc.SetOIDCClientID(context.Background(), 42, `evil"]=true--`); !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("err = %v, want ErrInvalidClientID", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run 'TestReprovisionBranches|TestSetOIDCClientID' -v`
Expected: FAIL.

- [ ] **Step 3: Implement the branch + setter**

In `internal/subscriptions/service.go`:
- Add errors: `var ErrOIDCNotConfigured = errors.New("subscriptions: oidc not configured")` and `var ErrInvalidClientID = errors.New("subscriptions: invalid oidc client id")`.
- Add fields `oidcIssuer, oidcClaim string` to `Service` + a setter:
```go
// ConfigureOIDC wires the trusted issuer + client-id claim for oauth2 product
// routes. Empty issuer leaves OAuth2 provisioning disabled.
func (s *Service) ConfigureOIDC(issuer, claim string) { s.oidcIssuer, s.oidcClaim = issuer, claim }
```
- In `reprovisionRoute`, after fetching `prod`, branch BEFORE the key-auth body:
```go
	if prod.AuthType == "oauth2" {
		allowed, err := s.store.OAuthClientsForProduct(ctx, productID)
		if err != nil {
			return err
		}
		allowed = append(allowed, extraConsumers...) // extras carry through for the approve path (client ids)
		if len(allowed) == 0 || s.oidcIssuer == "" {
			return s.gw.DeleteRoute(ctx, RouteID(prod.ID))
		}
		return s.gw.EnsureOAuthRoute(ctx, RouteID(prod.ID), prod.ContextPath, prod.Upstream, s.oidcIssuer, s.oidcClaim, dedup(allowed))
	}
	// ...existing key-auth path unchanged...
```
  (Provide a small `dedup([]string) []string` helper, or inline the existing dedup loop. For oauth2, `extraConsumers` passed by `Approve` must be the about-to-be-active app's **client id**, not its consumer name — see Step 4.)
- Add `SetOIDCClientID`:
```go
func (s *Service) SetOIDCClientID(ctx context.Context, appID int64, clientID string) error {
	if clientID != "" && !apisix.ValidClientID(clientID) {
		return ErrInvalidClientID
	}
	if err := s.store.SetAppOIDCClientID(ctx, appID, clientID); err != nil {
		return err
	}
	prods, err := s.store.OAuthProductsForApp(ctx, appID)
	if err != nil {
		return err
	}
	for _, p := range prods {
		if err := s.reprovisionRoute(ctx, p.ID); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Branch Subscribe + Approve for oauth2**

- In `Subscribe`, after resolving `prod` (and the published/already-subscribed checks), skip credential issuance for oauth2:
```go
	if prod.AuthType == "oauth2" {
		if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
			return Credential{}, err
		}
		s.logEvent(ctx, appID, events.KindSubscribed, &productID, &planID)
		return Credential{}, nil // no key for oauth2 apps
	}
	// ...existing key-auth path (GetOrCreateCredential + SaveSubscription)...
```
- In `Approve`, the existing code calls `reprovisionRoute(ctx, rec.ProductID, cred.ConsumerUsername)`. For oauth2 there is no consumer/cred; pass the app's client id as the extra instead. Restructure:
```go
	prod, err := s.store.GetProduct(ctx, rec.ProductID)
	if err != nil { return err }
	if prod.AuthType == "oauth2" {
		cid, _ := s.store.GetAppOIDCClientID(ctx, rec.AppID)
		if err := s.reprovisionRoute(ctx, rec.ProductID, cid); err != nil { return err } // cid "" → harmless extra
		if err := s.store.SetSubscriptionStatus(ctx, subID, StatusActive); err != nil { return err }
		s.logEvent(ctx, rec.AppID, events.KindApproved, &rec.ProductID, &rec.PlanID)
		return nil
	}
	// ...existing key-auth Approve (EnsureConsumer + reprovisionRoute(.., cred.ConsumerUsername) + SetStatus)...
```
  (Guard: an empty `cid` extra must be filtered by `reprovisionRoute`/`dedup` so it never enters the whitelist. Add `if e != "" {...}` to the extras merge.)
- `Reject`/`Unsubscribe` already call `ReprovisionRoute(productID)` which now branches correctly — no change needed beyond confirming they don't assume a consumer.

- [ ] **Step 5: Run to verify it passes + full package**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go vet ./internal/subscriptions/`
Expected: PASS (existing key-auth tests still green — the branch only adds an oauth2 path).

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/service.go internal/subscriptions/service_test.go
git commit -m "feat(oauth2): provisioning branches on auth_type + SetOIDCClientID + lifecycle"
```

---

## Task 5: HTTP endpoints — set oidc-client + app-detail fields + admin authType guard

**Files:**
- Modify: `internal/subscriptions/handler.go` (route + handler; `Reader`/detail fields), `internal/subscriptions/view.go` (`AppDetail` fields)
- Modify: `internal/admin/handler.go` or product decode (reject oauth2 when !OIDCConfigured)
- Test: `internal/subscriptions/handler_test.go`, `internal/admin/*_test.go`

**Interfaces:**
- Produces: `PUT /api/applications/{appID}/oidc-client` → 200 / 400; `AppDetail.OIDCClientID string`, `OAuthEligible bool`, `OIDCIssuer string`; admin create/update returns 400 for `oauth2` when OIDC unconfigured.

- [ ] **Step 1: Write the failing handler tests**

In `internal/subscriptions/handler_test.go` add (mirror the sandbox enable/rotate handler tests — real `*Service` over `memStore`, `owns` returns true for `appID==1 && userID==5`, authed via `auth.WithUserID(ctx,5)`, `NewHandler(...)` arity):
```go
func TestSetOIDCClientEndpoint(t *testing.T) {
	store := newMemStore() // app 1 exists
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example", "azp")
	h := newTestHandler(t, svc, store) // mirror the existing helper; ensure owns(1,5)=true
	req := httptest.NewRequest(http.MethodPut, "/api/applications/1/oidc-client", strings.NewReader(`{"clientId":"client-a"}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body) }
	if store.oidcClientIDs[1] != "client-a" { t.Fatalf("not saved") }
}

func TestSetOIDCClient400OnBadCharset(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	svc.ConfigureOIDC("https://idp.example", "azp")
	h := newTestHandler(t, svc, store)
	req := httptest.NewRequest(http.MethodPut, "/api/applications/1/oidc-client", strings.NewReader(`{"clientId":"a b\"]=true"}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest { t.Fatalf("status=%d", rec.Code) }
}
```
(Match the file's real handler-construction helper; if `NewHandler` needs a sandbox URL / other params from earlier features, pass them as the existing tests do.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestSetOIDCClient -v`
Expected: FAIL — route/handler absent.

- [ ] **Step 3: Implement the endpoint + detail fields**

In `internal/subscriptions/view.go` add to `AppDetail`:
```go
	OIDCClientID  string `json:"oidcClientId"`
	OAuthEligible bool   `json:"oauthEligible"`
	OIDCIssuer    string `json:"oidcIssuer"`
```
In `internal/subscriptions/handler.go`:
- Add `GetAppOIDCClientID(ctx, appID) (string, error)` and `OAuthProductsForApp(ctx, appID) ([]ProductInfo, error)` to the `Reader` interface (the `*Repo` already implements them). Add an `oidcIssuer string` field to `Handler` set via `NewHandler` (new trailing param) OR a setter `SetOIDCIssuer(string)` (prefer a setter to avoid churning callers — mirror `SetUsageReader`).
- Register: `h.router.Put("/api/applications/{appID}/oidc-client", h.setOIDCClient)`.
- Handler:
```go
func (h *Handler) setOIDCClient(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok { return }
	var body struct{ ClientID string `json:"clientId"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad body"); return
	}
	if err := h.svc.SetOIDCClientID(r.Context(), appID, body.ClientID); errors.Is(err, ErrInvalidClientID) {
		httpx.Error(w, http.StatusBadRequest, "invalid client id"); return
	} else if err != nil {
		log.Printf("set oidc client (app=%d): %v", appID, err)
		httpx.Error(w, http.StatusInternalServerError, "failed"); return
	}
	w.WriteHeader(http.StatusNoContent)
}
```
- In `detail`, populate the fields: `out.OIDCIssuer = h.oidcIssuer`; `out.OIDCClientID, _ = h.reader.GetAppOIDCClientID(...)`; `out.OAuthEligible = len(OAuthProductsForApp(...)) > 0` (best-effort, like the sandbox fields).

In `internal/admin` (the product create/update handler that already calls `validate(allowPrivate)`): after validation, reject oauth2 when OIDC is unconfigured. The handler has access to config-derived flags (mirror how `allowPrivate` reaches the handler) — add an `oidcConfigured bool` to the admin handler (set from `cfg.OIDCConfigured()` at construction) and:
```go
	if p.AuthType == "oauth2" && !h.oidcConfigured {
		httpx.Error(w, http.StatusBadRequest, "OAuth2 is not configured on this portal"); return
	}
```
Add an admin handler test asserting this 400.

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ ./internal/admin/ && go vet ./internal/subscriptions/ ./internal/admin/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/handler.go internal/subscriptions/view.go internal/subscriptions/handler_test.go internal/admin/
git commit -m "feat(oauth2): set-oidc-client endpoint + app-detail fields + admin authType guard"
```

---

## Task 6: Wire server/main + full suite + live verification

**Files:**
- Modify: `internal/server/server.go` (ConfigureOIDC on subSvc; SetOIDCIssuer on subH; oidcConfigured on adminH)
- Modify: `cmd/portal/main.go` if needed
- Test: full backend suite + live

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Wire in server.go**

In `internal/server/server.go` `New`, after `subSvc` is built:
```go
	subSvc.ConfigureOIDC(cfg.OIDCIssuer, cfg.OIDCClientIDClaim)
```
After `subH := subscriptions.NewHandler(...)`:
```go
	subH.SetOIDCIssuer(cfg.OIDCIssuer)
```
Pass `cfg.OIDCConfigured()` into the admin handler construction (add the param/field per Task 5). `oauth2` products' routes are provisioned through the existing subscribe/approve flow — no extra wiring.

- [ ] **Step 2: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go cmd/portal/main.go
git commit -m "feat(oauth2): wire OIDC config into server"
```

- [ ] **Step 4: Live verification (disposable test issuer)**

1. Stand up a **throwaway OIDC issuer** for testing only (NOT bundled): the simplest is a tiny static server exposing `/.well-known/openid-configuration` + `/jwks` with one RSA key, plus a hand-minted JWT (`azp` = a test client id, `iss` = the issuer, `exp` in the future) signed by that key. (A small Go/Node script or a minimal `mock-oauth2-server` container works; document what you used.)
2. Restart the portal with `OIDC_ISSUER=<test issuer>` (and `OIDC_CLIENT_ID_CLAIM=azp`); the sandbox/portal already run on `:8090`.
3. As admin, set a product `authType='oauth2'` (`PUT /api/admin/products/{id}`).
4. As a developer: subscribe an app to that product (no key returned), set the app's client id (`PUT /api/applications/{id}/oidc-client {"clientId":"<test client id>"}`), and get the subscription approved.
5. Call the gateway:
   - `curl -H "Authorization: Bearer <valid test JWT, azp=client>" http://localhost:9080/<context>/...` → **200**.
   - a valid JWT whose `azp` is a DIFFERENT (non-subscribed) client → **403**.
   - no token / malformed token → **401** (openid-connect).
6. Flip the product back to `key-auth` → confirm the route is replaced (key-auth behavior returns).

---

## Self-Review notes

- **Spec coverage:** config + OIDCConfigured (T1) ✅; EnsureOAuthRoute openid-connect+serverless+injection guard (T1) ✅; migration auth_type+oidc_client_id + reads (T2) ✅; OAuth whitelist/products/get/set repo (T3) ✅; service auth_type branch + SetOIDCClientID + Subscribe/Approve branches + lifecycle (T4) ✅; set-oidc-client endpoint + app-detail fields + admin 400 guard (T5) ✅; wiring + live (T6) ✅. Frontend (admin auth selector, OAuth2 credentials card, authType badge) is **Plan 2**, authored after this ships. Per-app OAuth2 rate-limiting + interactive Try-it deferred per spec.
- **Type consistency:** `EnsureOAuthRoute(routeID,contextPath,upstreamURL,issuer,claimName,allowed)` consistent (T1 defines, T4 calls); `ValidClientID` (T1) used by T4; Store methods `OAuthClientsForProduct`/`OAuthProductsForApp`/`GetAppOIDCClientID`/`SetAppOIDCClientID` identical across repo (T3), interface (T3), fakes (T4); `ProductInfo.AuthType` (T2) read in T3/T4; `Service.ConfigureOIDC`/`SetOIDCClientID`/`ErrInvalidClientID` (T4) used by T5/T6; `AppDetail.OIDCClientID/OAuthEligible/OIDCIssuer` (T5).
- **Implementer notes:** confirm the exact APISIX 3.9.1 `openid-connect` attribute names in T1 (the live check in T6 is the real proof the validation works); renumber admin `Create`/`Update` `$` placeholders carefully for `auth_type` (T2); the `extraConsumers` merge in `reprovisionRoute` must filter `""` so an app without a client id never enters a whitelist (T4); prefer setters (`ConfigureOIDC`, `SetOIDCIssuer`) over `NewService`/`NewHandler` arity changes to avoid churning the many existing callers.
