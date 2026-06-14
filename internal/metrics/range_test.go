package metrics

import "testing"

func TestParseRangeAcceptsAllowListed(t *testing.T) {
	for _, key := range []string{"24h", "7d", "30d"} {
		r, err := ParseRange(key)
		if err != nil {
			t.Fatalf("ParseRange(%q) unexpected error: %v", key, err)
		}
		if r.Key != key {
			t.Errorf("ParseRange(%q).Key = %q", key, r.Key)
		}
		if r.Window <= 0 || r.Step <= 0 {
			t.Errorf("ParseRange(%q) window=%v step=%v, want positive", key, r.Window, r.Step)
		}
		// The chart must stay bounded regardless of range: never more than a
		// ~hundred points reach the client.
		if points := int(r.Window / r.Step); points > 130 {
			t.Errorf("ParseRange(%q) yields %d points, want <=130", key, points)
		}
	}
}

func TestParseRangeRejectsAnythingElse(t *testing.T) {
	// An unbounded/free-form range is a DoS and PromQL-injection lever, so the
	// parser is a strict allow-list — not a duration parser.
	for _, bad := range []string{"", "1h", "12h", "365d", "0d", "24", "24H", "24h ", " 24h",
		"24h] or vector(1)", "24h\"}", "*", "999999d"} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("ParseRange(%q) = nil error, want rejection", bad)
		}
	}
}
