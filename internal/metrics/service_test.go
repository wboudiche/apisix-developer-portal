package metrics

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeQuerier scripts Prometheus responses by inspecting the PromQL, and counts
// calls so cache behavior is observable.
type fakeQuerier struct {
	scalarCalls int
	rangeCalls  int
	scalar      func(q string) (float64, error)
	rng         func(q string) ([]Sample, error)
}

func (f *fakeQuerier) Scalar(_ context.Context, q string) (float64, error) {
	f.scalarCalls++
	return f.scalar(q)
}
func (f *fakeQuerier) Range(_ context.Context, q string, _, _ time.Time, _ time.Duration) ([]Sample, error) {
	f.rangeCalls++
	return f.rng(q)
}

// fixedClock: 2026-06-15T12:00:00Z. Since-midnight = 43200s; since-month-start
// (June 1) = 14d12h = 1252800s. The 24h range window = 86400s.
func fixedClock() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }

func mappingQuerier(t *testing.T) *fakeQuerier {
	t.Helper()
	t1 := time.Unix(1000, 0).UTC()
	t2 := time.Unix(2000, 0).UTC()
	return &fakeQuerier{
		scalar: func(q string) (float64, error) {
			if !strings.Contains(q, `consumer="app_7"`) {
				t.Errorf("scalar query not scoped to consumer: %q", q)
			}
			switch {
			case strings.Contains(q, "histogram_quantile"):
				return 42, nil
			case strings.Contains(q, "/"): // error rate is a ratio query
				return 0.05, nil
			case strings.Contains(q, "[43200s]"): // since midnight
				return 1000, nil
			case strings.Contains(q, "[1252800s]"): // since month start
				return 25000, nil
			}
			t.Errorf("unexpected scalar query: %q", q)
			return 0, nil
		},
		rng: func(q string) ([]Sample, error) {
			if !strings.Contains(q, `consumer="app_7"`) {
				t.Errorf("range query not scoped to consumer: %q", q)
			}
			if strings.Contains(q, `code=~"5.."`) {
				return []Sample{{T: t1, V: 2}, {T: t2, V: 3}}, nil
			}
			return []Sample{{T: t1, V: 100}, {T: t2, V: 150}}, nil
		},
	}
}

func TestUsageMapsSummaryAndSeries(t *testing.T) {
	q := mappingQuerier(t)
	svc := NewService(q)
	svc.now = fixedClock
	r, _ := ParseRange("24h")

	u, err := svc.Usage(context.Background(), "app_7", r)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.Summary.RequestsToday != 1000 || u.Summary.MonthToDate != 25000 {
		t.Errorf("summary counts = %+v", u.Summary)
	}
	if u.Summary.P95Ms != 42 || u.Summary.ErrorRate != 0.05 {
		t.Errorf("summary p95/err = %+v", u.Summary)
	}
	want := []Point{{T: time.Unix(1000, 0).UTC(), Requests: 100, Errors: 2}, {T: time.Unix(2000, 0).UTC(), Requests: 150, Errors: 3}}
	if len(u.Series) != 2 || u.Series[0] != want[0] || u.Series[1] != want[1] {
		t.Errorf("series = %+v, want %+v", u.Series, want)
	}
}

func TestUsageCachesWithinTTL(t *testing.T) {
	q := mappingQuerier(t)
	svc := NewService(q)
	svc.now = fixedClock
	r, _ := ParseRange("24h")

	if _, err := svc.Usage(context.Background(), "app_7", r); err != nil {
		t.Fatal(err)
	}
	firstScalar, firstRange := q.scalarCalls, q.rangeCalls
	if firstScalar == 0 || firstRange == 0 {
		t.Fatal("expected queries on first call")
	}
	if _, err := svc.Usage(context.Background(), "app_7", r); err != nil {
		t.Fatal(err)
	}
	if q.scalarCalls != firstScalar || q.rangeCalls != firstRange {
		t.Errorf("second call re-queried: scalar %d->%d range %d->%d", firstScalar, q.scalarCalls, firstRange, q.rangeCalls)
	}
}

func TestUsageCacheKeyedByConsumerAndRange(t *testing.T) {
	q := mappingQuerier(t)
	svc := NewService(q)
	svc.now = fixedClock
	r24, _ := ParseRange("24h")
	r7, _ := ParseRange("7d")

	if _, err := svc.Usage(context.Background(), "app_7", r24); err != nil {
		t.Fatal(err)
	}
	calls := q.scalarCalls
	// Different range → cache miss → re-query.
	if _, err := svc.Usage(context.Background(), "app_7", r7); err != nil {
		t.Fatal(err)
	}
	if q.scalarCalls == calls {
		t.Error("different range should miss cache and re-query")
	}
}

func TestUsageRejectsInvalidConsumer(t *testing.T) {
	q := mappingQuerier(t)
	svc := NewService(q)
	svc.now = fixedClock
	r, _ := ParseRange("24h")
	// A consumer name with PromQL metacharacters must never be interpolated.
	if _, err := svc.Usage(context.Background(), `app_7"} or up{`, r); err == nil {
		t.Fatal("want error on invalid consumer, got nil")
	}
	if q.scalarCalls != 0 || q.rangeCalls != 0 {
		t.Error("invalid consumer should be rejected before any query")
	}
}
