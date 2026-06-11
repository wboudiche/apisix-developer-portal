package httpx_test

import (
	"fmt"
	"net"
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

func TestRateLimiterTrustsXFFOnlyFromTrustedProxy(t *testing.T) {
	mustCIDRs := func(csv string) []*net.IPNet {
		nets, err := httpx.ParseProxyCIDRs(csv)
		if err != nil {
			t.Fatalf("parse %q: %v", csv, err)
		}
		return nets
	}

	// Limiter behind a trusted proxy: requests arrive with RemoteAddr = proxy,
	// and the real client must be read from X-Forwarded-For so distinct clients
	// stay isolated instead of sharing one bucket.
	rl := httpx.NewRateLimiter(1, 0)
	rl.SetTrustedProxies(mustCIDRs("10.0.0.0/8"))
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	call := func(xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.RemoteAddr = "10.1.2.3:9999" // the trusted proxy
		req.Header.Set("X-Forwarded-For", xff)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if call("203.0.113.1") != 200 || call("203.0.113.2") != 200 {
		t.Fatal("distinct real clients (via XFF) must each get their own bucket")
	}
	if call("203.0.113.1") != http.StatusTooManyRequests {
		t.Fatal("second request from the same real client must 429")
	}
	// A multi-hop chain "client, innerproxy(10.x)": the rightmost non-trusted
	// entry is the real client.
	if call("198.51.100.7, 10.9.9.9") != 200 {
		t.Fatal("rightmost untrusted XFF entry should be the bucket key")
	}
	if call("198.51.100.7, 10.9.9.9") != http.StatusTooManyRequests {
		t.Fatal("same real client through a proxy chain must 429 on repeat")
	}
}

func TestRateLimiterIgnoresXFFFromUntrustedPeer(t *testing.T) {
	// No trusted proxies configured: a spoofed XFF must be ignored and the
	// peer (RemoteAddr) used, so an attacker can't dodge the limit by rotating
	// the header.
	rl := httpx.NewRateLimiter(1, 0)
	h := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	call := func(xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.RemoteAddr = "203.0.113.9:4444"
		req.Header.Set("X-Forwarded-For", xff)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if call("1.1.1.1") != 200 {
		t.Fatal("first request must pass")
	}
	if call("2.2.2.2") != http.StatusTooManyRequests {
		t.Fatal("a rotated spoofed XFF must NOT mint a new bucket when no proxy is trusted")
	}
}

func TestParseProxyCIDRs(t *testing.T) {
	if nets, err := httpx.ParseProxyCIDRs(""); err != nil || nets != nil {
		t.Fatalf("empty input must yield (nil, nil); got %v %v", nets, err)
	}
	if _, err := httpx.ParseProxyCIDRs("10.0.0.0/8, 127.0.0.1/32"); err != nil {
		t.Fatalf("valid CIDR list must parse: %v", err)
	}
	if _, err := httpx.ParseProxyCIDRs("not-a-cidr"); err == nil {
		t.Fatal("malformed CIDR must error")
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
