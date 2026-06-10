package httpx_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestNewRateLimiterPanicsOnZeroBurst(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewRateLimiter(0, 1) must panic")
		}
	}()
	httpx.NewRateLimiter(0, 1)
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
