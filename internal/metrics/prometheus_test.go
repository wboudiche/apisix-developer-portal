package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeProm returns a canned Prometheus response and records the last query it
// was asked, so tests can assert both parsing and the PromQL we send.
func fakeProm(t *testing.T, body string) (*Client, *string) {
	t.Helper()
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastQuery = r.Form.Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL), &lastQuery
}

func TestScalarParsesVectorValue(t *testing.T) {
	c, q := fakeProm(t, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1781449839.068,"12"]}]}}`)
	got, err := c.Scalar(context.Background(), `sum(foo)`)
	if err != nil {
		t.Fatalf("Scalar: %v", err)
	}
	if got != 12 {
		t.Errorf("Scalar = %v, want 12", got)
	}
	if *q != "sum(foo)" {
		t.Errorf("sent query %q", *q)
	}
}

func TestScalarEmptyResultIsZero(t *testing.T) {
	// No matching series (e.g. an app with no traffic yet) is zero, not an error.
	c, _ := fakeProm(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	got, err := c.Scalar(context.Background(), `sum(foo)`)
	if err != nil {
		t.Fatalf("Scalar: %v", err)
	}
	if got != 0 {
		t.Errorf("Scalar = %v, want 0", got)
	}
}

func TestScalarNaNIsZero(t *testing.T) {
	// histogram_quantile over an empty histogram yields NaN; a card shows 0.
	c, _ := fakeProm(t, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1781449839.068,"NaN"]}]}}`)
	got, err := c.Scalar(context.Background(), `q`)
	if err != nil {
		t.Fatalf("Scalar: %v", err)
	}
	if got != 0 {
		t.Errorf("Scalar = %v, want 0", got)
	}
}

func TestScalarPropagatesPrometheusError(t *testing.T) {
	c, _ := fakeProm(t, `{"status":"error","errorType":"bad_data","error":"parse error"}`)
	if _, err := c.Scalar(context.Background(), `q`); err == nil {
		t.Fatal("Scalar: want error on status=error, got nil")
	}
}

func TestRangeParsesMatrix(t *testing.T) {
	c, q := fakeProm(t, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1000,"3"],[1015,"5"]]}]}}`)
	start := time.Unix(1000, 0)
	pts, err := c.Range(context.Background(), `sum(bar)`, start, start.Add(time.Minute), 15*time.Second)
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("Range len = %d, want 2", len(pts))
	}
	if pts[0].T.Unix() != 1000 || pts[0].V != 3 || pts[1].V != 5 {
		t.Errorf("Range points = %+v", pts)
	}
	if *q != "sum(bar)" {
		t.Errorf("sent query %q", *q)
	}
}

func TestRangeEmptyMatrixIsEmptySlice(t *testing.T) {
	c, _ := fakeProm(t, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	start := time.Unix(1000, 0)
	pts, err := c.Range(context.Background(), `q`, start, start.Add(time.Minute), 15*time.Second)
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if len(pts) != 0 {
		t.Errorf("Range = %+v, want empty", pts)
	}
}

func TestClientErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	if _, err := c.Scalar(context.Background(), `q`); err == nil {
		t.Fatal("want error on 502, got nil")
	}
}
