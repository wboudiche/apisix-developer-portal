// Package metrics serves the application Usage page (the Overview stat cards and
// the traffic chart) from gateway request metrics. It is a read-only consumer
// of Prometheus — it never counts requests itself and never touches the portal
// database, so page-load cost stays constant regardless of traffic volume.
package metrics

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"
)

// Summary is the four Overview stat cards. Counts are over calendar windows
// (today, month-to-date, UTC); p95 latency and error rate are over the selected
// range so they track recent behavior.
type Summary struct {
	RequestsToday int64   `json:"requestsToday"`
	MonthToDate   int64   `json:"monthToDate"`
	P95Ms         float64 `json:"p95Ms"`
	ErrorRate     float64 `json:"errorRate"`
}

// Point is one bucket of the usage chart.
type Point struct {
	T        time.Time `json:"t"`
	Requests int64     `json:"requests"`
	Errors   int64     `json:"errors"`
}

// Usage is the full response payload for one application.
type Usage struct {
	Summary Summary `json:"summary"`
	Series  []Point `json:"series"`
}

// Querier is the read surface this package needs from Prometheus (satisfied by
// *Client); an interface so the service is testable without a live backend.
type Querier interface {
	Scalar(ctx context.Context, query string) (float64, error)
	Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Sample, error)
}

// consumerRe bounds the consumer label we interpolate into PromQL. Portal
// consumers are always "app_<id>"; this is defense-in-depth so a malformed name
// can never break out of the label literal.
var consumerRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

const defaultTTL = 15 * time.Second

type cacheEntry struct {
	usage Usage
	at    time.Time
}

// Service builds per-consumer PromQL, maps results to Usage, and caches each
// (consumer,range) result for a short TTL so repeated page loads don't fan out
// to Prometheus.
type Service struct {
	q   Querier
	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

func NewService(q Querier) *Service {
	return &Service{q: q, ttl: defaultTTL, now: time.Now, cache: map[string]cacheEntry{}}
}

// RequestsInWindow returns the approximate number of requests by the consumer
// over the last windowSeconds, from Prometheus. Used by the per-app rate-limit
// meter; it is an approximation of the gateway's limit-count counter, not the
// exact value.
func (s *Service) RequestsInWindow(ctx context.Context, consumer string, windowSeconds int) (int64, error) {
	if !consumerRe.MatchString(consumer) {
		return 0, fmt.Errorf("invalid consumer %q", consumer)
	}
	sel := fmt.Sprintf(`consumer="%s"`, consumer)
	v, err := s.q.Scalar(ctx, countQuery(sel, time.Duration(windowSeconds)*time.Second))
	if err != nil {
		return 0, err
	}
	if v < 0 {
		v = 0
	}
	return int64(v + 0.5), nil
}

// Usage returns the cards + chart for one consumer over the given range.
func (s *Service) Usage(ctx context.Context, consumer string, r Range) (Usage, error) {
	if !consumerRe.MatchString(consumer) {
		return Usage{}, fmt.Errorf("invalid consumer %q", consumer)
	}
	key := consumer + "|" + r.Key
	now := s.now().UTC()

	s.mu.Lock()
	if e, ok := s.cache[key]; ok && now.Sub(e.at) < s.ttl {
		s.mu.Unlock()
		return e.usage, nil
	}
	s.mu.Unlock()

	u, err := s.fetch(ctx, consumer, r, now)
	if err != nil {
		return Usage{}, err
	}
	s.mu.Lock()
	s.cache[key] = cacheEntry{usage: u, at: now}
	s.mu.Unlock()
	return u, nil
}

func (s *Service) fetch(ctx context.Context, consumer string, r Range, now time.Time) (Usage, error) {
	sel := fmt.Sprintf(`consumer="%s"`, consumer)
	win := r.Window

	// Calendar windows for the count cards, expressed as a duration back from
	// now (Prometheus has no calendar alignment without recording rules).
	sinceMidnight := floorDur(now.Sub(now.Truncate(24*time.Hour)), time.Minute)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	sinceMonth := floorDur(now.Sub(monthStart), time.Minute)

	today, err := s.q.Scalar(ctx, countQuery(sel, sinceMidnight))
	if err != nil {
		return Usage{}, err
	}
	month, err := s.q.Scalar(ctx, countQuery(sel, sinceMonth))
	if err != nil {
		return Usage{}, err
	}
	p95, err := s.q.Scalar(ctx, p95Query(sel, win))
	if err != nil {
		return Usage{}, err
	}
	errRate, err := s.q.Scalar(ctx, errorRateQuery(sel, win))
	if err != nil {
		return Usage{}, err
	}
	reqSeries, err := s.q.Range(ctx, seriesQuery(sel, r.Step, ""), now.Add(-win), now, r.Step)
	if err != nil {
		return Usage{}, err
	}
	errSeries, err := s.q.Range(ctx, seriesQuery(sel, r.Step, `,code=~"5.."`), now.Add(-win), now, r.Step)
	if err != nil {
		return Usage{}, err
	}

	return Usage{
		Summary: Summary{
			RequestsToday: int64(today),
			MonthToDate:   int64(month),
			P95Ms:         p95,
			ErrorRate:     errRate,
		},
		Series: mergeSeries(reqSeries, errSeries),
	}, nil
}

// PromQL builders. apisix_http_status is a request counter; apisix_http_latency
// (type="request") is the end-to-end latency histogram in milliseconds.

func countQuery(sel string, d time.Duration) string {
	return fmt.Sprintf(`sum(increase(apisix_http_status{%s}[%ds]))`, sel, int(d.Seconds()))
}

func p95Query(sel string, d time.Duration) string {
	return fmt.Sprintf(`histogram_quantile(0.95, sum(rate(apisix_http_latency_bucket{%s,type="request"}[%ds])) by (le))`, sel, int(d.Seconds()))
}

func errorRateQuery(sel string, d time.Duration) string {
	// clamp_min the denominator so no traffic yields 0, never a divide error.
	return fmt.Sprintf(`sum(increase(apisix_http_status{%s,code=~"5.."}[%ds])) / clamp_min(sum(increase(apisix_http_status{%s}[%ds])), 1)`, sel, int(d.Seconds()), sel, int(d.Seconds()))
}

func seriesQuery(sel string, step time.Duration, extra string) string {
	return fmt.Sprintf(`sum(increase(apisix_http_status{%s%s}[%ds]))`, sel, extra, int(step.Seconds()))
}

// mergeSeries aligns the requests and errors range results by timestamp; errors
// missing at a bucket count as 0.
func mergeSeries(requests, errors []Sample) []Point {
	errByT := make(map[int64]float64, len(errors))
	for _, e := range errors {
		errByT[e.T.Unix()] = e.V
	}
	out := make([]Point, 0, len(requests))
	for _, p := range requests {
		out = append(out, Point{
			T:        p.T,
			Requests: int64(p.V),
			Errors:   int64(errByT[p.T.Unix()]),
		})
	}
	return out
}

func floorDur(d, min time.Duration) time.Duration {
	if d < min {
		return min
	}
	return d
}
