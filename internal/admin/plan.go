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

// SupportedCurrencies are the ISO 4217 codes a plan may be priced in. All use
// two minor-unit decimals, matching price_cents storage (zero- or three-decimal
// currencies such as JPY or TND would need a different scale). Keep in sync
// with web/src/currencies.ts.
var SupportedCurrencies = []string{"EUR", "USD", "GBP", "CHF", "CAD", "AUD", "SEK", "NOK", "DKK", "PLN", "MAD"}

func validCurrency(c string) bool {
	for _, s := range SupportedCurrencies {
		if c == s {
			return true
		}
	}
	return false
}
