package admin

import "strings"

// Plan is a rate-limit tier as managed by an admin. Its JSON shape matches
// plans.Plan so the frontend can reuse one type across read and admin APIs.
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
}

// validate returns "" when the plan is valid, otherwise a human-readable reason.
func (p Plan) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "name is required"
	}
	if p.RateLimit <= 0 {
		return "rateLimit must be greater than zero"
	}
	if p.WindowSeconds <= 0 {
		return "windowSeconds must be greater than zero"
	}
	return ""
}
