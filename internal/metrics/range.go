package metrics

import (
	"fmt"
	"time"
)

// Range is an allow-listed query window for the usage endpoint. The set is
// closed on purpose: the range feeds a PromQL duration literal, so a free-form
// value would be both a DoS lever (unbounded scan) and an injection surface.
// Step is chosen so the downsampled chart never returns more than ~100 points.
type Range struct {
	Key    string
	Window time.Duration
	Step   time.Duration
}

const (
	day  = 24 * time.Hour
	hour = time.Hour
)

// ranges is the closed allow-list. Steps: 24h→15m (96 pts), 7d→2h (84 pts),
// 30d→8h (90 pts).
var ranges = map[string]Range{
	"24h": {Key: "24h", Window: 24 * hour, Step: 15 * time.Minute},
	"7d":  {Key: "7d", Window: 7 * day, Step: 2 * hour},
	"30d": {Key: "30d", Window: 30 * day, Step: 8 * hour},
}

// ParseRange resolves an allow-listed range key, rejecting everything else.
func ParseRange(s string) (Range, error) {
	if r, ok := ranges[s]; ok {
		return r, nil
	}
	return Range{}, fmt.Errorf("unsupported range %q (allowed: 24h, 7d, 30d)", s)
}
