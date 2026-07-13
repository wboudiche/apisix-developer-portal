package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/config"
	"apisix-portal/internal/db"
	"apisix-portal/internal/server"

	"github.com/jackc/pgx/v5/pgxpool"
)

// uniq gives collision-free names across runs without time.Now() in the body.
// Seeded once from the PID; uniqueness (not reproducibility) is the goal.
var uniqCounter = os.Getpid() * 100000
var uniqN = 0

func uniq(prefix string) string {
	uniqN++
	return fmt.Sprintf("%s_%d_%d", prefix, uniqCounter, uniqN)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type harness struct {
	t   *testing.T
	srv *httptest.Server
	gw  *apisix.Client
	pool *pgxpool.Pool
}

// newHarness skips unless RUN_E2E=1 and the DB is reachable; mounts the real
// portal handler against the real DB + APISIX client.
func newHarness(t *testing.T) *harness {
	t.Helper()
	if os.Getenv("RUN_E2E") != "1" {
		t.Skip("set RUN_E2E=1 with the compose stack up to run E2E")
	}
	ctx := context.Background()
	dbURL := envOr("DATABASE_URL", "postgres://portal:portal@localhost:5432/portal?sslmode=disable")
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := config.Load()
	cfg.DatabaseURL = dbURL
	cfg.APISIXAdminURL = envOr("APISIX_ADMIN_URL", "http://localhost:19180")
	cfg.APISIXAdminKey = envOr("APISIX_ADMIN_KEY", "edd1c9f034335f136f87ad84b625c8f1")
	gw := apisix.NewClient(cfg.APISIXAdminURL, cfg.APISIXAdminKey)
	srv := httptest.NewServer(server.New(ctx, pool, cfg))
	h := &harness{t: t, srv: srv, gw: gw, pool: pool}
	t.Cleanup(func() { srv.Close(); pool.Close() })
	return h
}

// api calls a portal endpoint with an optional bearer token and JSON body,
// decodes the response into out (if non-nil), and returns the status code.
func (h *harness) api(method, path, token string, body, out any) int {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rdr)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("decode %s %s (%d): %v; body=%s", method, path, res.StatusCode, err, raw)
		}
	}
	return res.StatusCode
}

// gatewayGet calls the APISIX gateway directly; empty apikey omits the header.
func (h *harness) gatewayGet(path, apikey string) int {
	h.t.Helper()
	gwURL := envOr("APISIX_GATEWAY_URL", "http://localhost:9080")
	req, _ := http.NewRequest(http.MethodGet, gwURL+path, nil)
	if apikey != "" {
		req.Header.Set("apikey", apikey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("gateway %s: %v", path, err)
	}
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)
	return res.StatusCode
}

func TestHarnessSmoke(t *testing.T) {
	h := newHarness(t)
	var reg struct {
		Token string `json:"token"`
	}
	email := uniq("smoke") + "@e2e.test"
	code := h.api(http.MethodPost, "/api/auth/register", "", map[string]any{
		"email": email, "password": "pw-12345", "name": "Smoke",
	}, &reg)
	if code != http.StatusCreated {
		t.Fatalf("register: got %d want 201", code)
	}
	if reg.Token == "" {
		t.Fatal("register returned empty token")
	}
}
