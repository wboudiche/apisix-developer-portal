package admin

import "strings"

// Plan is a rate-limit tier as managed by an admin. Its JSON shape matches
// plans.Plan so the frontend can reuse one type across read and admin APIs.
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
	PriceCents    int    `json:"priceCents"`
	Currency      string `json:"currency"`
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
	if p.PriceCents < 0 {
		return "admin.plan.badPrice"
	}
	if !validCurrency(p.Currency) {
		return "admin.plan.badCurrency"
	}
	return ""
}

// validCurrency accepts exactly three ASCII uppercase letters (ISO 4217 shape).
func validCurrency(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
