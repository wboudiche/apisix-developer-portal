package billing

import "context"

// BillingProvider settles an invoice with a payment backend. A real PSP creates
// a payment intent / checkout and returns its reference; the built-in
// ManualProvider records nothing external (the invoice stays pending until an
// admin marks it paid).
type BillingProvider interface {
	Charge(ctx context.Context, inv Invoice) (ref string, err error)
}

type ManualProvider struct{}

func (ManualProvider) Charge(ctx context.Context, inv Invoice) (string, error) { return "", nil }
