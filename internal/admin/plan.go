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

// validate returns "" when the plan is valid, otherwise an i18n message key
// (see internal/i18n) describing the reason.
func (p Plan) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "common.nameRequired"
	}
	if p.RateLimit <= 0 {
		return "admin.plan.badRateLimit"
	}
	if p.WindowSeconds <= 0 {
		return "admin.plan.badWindowSeconds"
	}
	return ""
}
