package httpx

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
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
// shared state). Stale buckets are swept on a fixed cadence, so sustained
// unique-key floods are bounded per sweep interval rather than growing
// indefinitely.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	burst      float64
	refill     float64 // tokens per second
	retryAfter string  // seconds, precomputed for the 429 header
	now        func() time.Time
	lastSweep  time.Time
	trusted    []*net.IPNet // proxy CIDRs whose X-Forwarded-For we honor
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter creates a limiter allowing `burst` requests immediately, then
// refilling at `refillPerSec` tokens/second per key. burst must be positive;
// refillPerSec may be 0 for a fixed-quota limiter that never refills.
func NewRateLimiter(burst, refillPerSec float64) *RateLimiter {
	if burst <= 0 {
		panic("httpx: rate limiter burst must be positive")
	}
	var retry string
	if refillPerSec > 0 {
		retry = strconv.Itoa(int(math.Ceil(1 / refillPerSec)))
	}
	return &RateLimiter{
		buckets:    make(map[string]*bucket),
		burst:      burst,
		refill:     refillPerSec,
		retryAfter: retry,
		now:        time.Now,
	}
}

// SetNow overrides the limiter's clock. Test hook only.
func (rl *RateLimiter) SetNow(fn func() time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.now = fn
}

// SetTrustedProxies tells the limiter which immediate-peer CIDRs are trusted
// reverse proxies. Only when the direct peer (RemoteAddr) is in one of these
// will X-Forwarded-For be consulted to recover the real client IP — so a
// spoofed XFF from an untrusted client is ignored. Empty (the default) means
// RemoteAddr is always used as-is.
func (rl *RateLimiter) SetTrustedProxies(cidrs []*net.IPNet) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.trusted = cidrs
}

// ParseProxyCIDRs parses a comma-separated list of CIDRs (e.g.
// "10.0.0.0/8,127.0.0.1/32") for use with SetTrustedProxies. Empty input
// yields a nil slice and no error.
func ParseProxyCIDRs(csv string) ([]*net.IPNet, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}
	var out []*net.IPNet
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy CIDR %q: %w", part, err)
		}
		out = append(out, ipnet)
	}
	return out, nil
}

// Len reports the number of tracked buckets. Test hook only.
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

func ipInAny(ip string, nets []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// clientIP returns the key the limiter buckets on. Normally this is the direct
// peer (RemoteAddr). Only when the peer is a trusted proxy do we walk
// X-Forwarded-For right-to-left and return the first address that isn't itself
// a trusted proxy — the real client behind the proxy chain. This keeps XFF
// unspoofable from untrusted peers while restoring per-client isolation when
// the portal runs behind a TLS-terminating reverse proxy.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(trusted) == 0 || !ipInAny(host, trusted) {
		return host
	}
	var parts []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, p := range strings.Split(v, ",") {
			parts = append(parts, strings.TrimSpace(p))
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" && !ipInAny(parts[i], trusted) {
			return parts[i]
		}
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

// RetryAfter is the suggested wait (whole seconds) once a key is limited.
func (rl *RateLimiter) RetryAfter() string { return rl.retryAfter }

// Middleware returns an http middleware that 429s requests over the per-IP limit.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rl.mu.Lock()
			trusted := rl.trusted
			rl.mu.Unlock()
			if !rl.Allow(clientIP(r, trusted)) {
				if ra := rl.RetryAfter(); ra != "" {
					w.Header().Set("Retry-After", ra)
				}
				Error(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
