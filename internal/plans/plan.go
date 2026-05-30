package plans

// Plan is a subscription rate-limit tier.
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
}
