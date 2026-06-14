package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a read-only Prometheus HTTP API client. It exposes only the two
// query shapes the usage endpoint needs (instant scalar, range matrix); it
// never writes and never touches the portal's database.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Sample is one (timestamp, value) point from a range query.
type Sample struct {
	T time.Time
	V float64
}

// promEnvelope is the common Prometheus API response wrapper.
type promEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

func (c *Client) get(ctx context.Context, path string, form url.Values) (*promEnvelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+form.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus %s: %d %s", path, resp.StatusCode, string(body))
	}
	var env promEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("prometheus %s: decode: %w", path, err)
	}
	if env.Status != "success" {
		return nil, fmt.Errorf("prometheus %s: status=%s: %s", path, env.Status, env.Error)
	}
	return &env, nil
}

// Scalar runs an instant query and returns the first series' value. An empty
// result (no matching series) and a NaN value both map to 0 — a card shows
// "no traffic", not an error.
func (c *Client) Scalar(ctx context.Context, query string) (float64, error) {
	env, err := c.get(ctx, "/api/v1/query", url.Values{"query": {query}})
	if err != nil {
		return 0, err
	}
	if len(env.Data.Result) == 0 {
		return 0, nil
	}
	// Vector result element: {"metric":{...},"value":[<ts>,"<val>"]}.
	var elem struct {
		Value [2]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(env.Data.Result[0], &elem); err != nil {
		return 0, fmt.Errorf("prometheus scalar: decode value: %w", err)
	}
	return parseSampleValue(elem.Value[1])
}

// Range runs a range query and returns the first series' points in order.
func (c *Client) Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Sample, error) {
	form := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}
	env, err := c.get(ctx, "/api/v1/query_range", form)
	if err != nil {
		return nil, err
	}
	if len(env.Data.Result) == 0 {
		return nil, nil
	}
	var elem struct {
		Values [][2]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(env.Data.Result[0], &elem); err != nil {
		return nil, fmt.Errorf("prometheus range: decode values: %w", err)
	}
	out := make([]Sample, 0, len(elem.Values))
	for _, pair := range elem.Values {
		ts, err := parseSampleValue(pair[0])
		if err != nil {
			return nil, err
		}
		v, err := parseSampleValue(pair[1])
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{T: time.Unix(int64(ts), 0).UTC(), V: v})
	}
	return out, nil
}

// parseSampleValue decodes a Prometheus sample component, which is a
// JSON-quoted string ("12", "NaN", "1.5e3") or a bare number (timestamps in
// some encodings). NaN/Inf collapse to 0 so they never reach a UI card.
func parseSampleValue(raw json.RawMessage) (float64, error) {
	s := strings.Trim(string(raw), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sample value %q: %w", s, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, nil
	}
	return v, nil
}
