package metrics

import (
	"context"
	"os"
	"testing"
)

// TestIntegrationUsageQueriesProm validates that the PromQL the service builds
// is accepted and parsed by a real Prometheus (catches query-syntax/label
// mistakes the fake-backed unit tests can't). It does not assert specific
// counts — driving deterministic traffic is the E2E harness's job.
//
// Run with the compose stack up:
//
//	RUN_PROM_IT=1 go test ./internal/metrics/ -run Integration
func TestIntegrationUsageQueriesProm(t *testing.T) {
	if os.Getenv("RUN_PROM_IT") != "1" {
		t.Skip("set RUN_PROM_IT=1 with the compose stack up to run")
	}
	url := os.Getenv("PROMETHEUS_URL")
	if url == "" {
		url = "http://localhost:9099"
	}
	svc := NewService(NewClient(url))
	for _, key := range []string{"24h", "7d", "30d"} {
		r, err := ParseRange(key)
		if err != nil {
			t.Fatal(err)
		}
		// A consumer that need not exist: a valid query over no data returns
		// zeroed usage with no error, which is exactly what we assert.
		u, err := svc.Usage(context.Background(), "it_nonexistent_consumer", r)
		if err != nil {
			t.Fatalf("Usage(%s): %v", key, err)
		}
		if u.Summary.ErrorRate < 0 {
			t.Errorf("Usage(%s): negative error rate %v", key, u.Summary.ErrorRate)
		}
	}
}
