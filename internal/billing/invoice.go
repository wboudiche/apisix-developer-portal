// Package billing records plan pricing and invoices for paid subscriptions.
package billing

import (
	"errors"
	"time"
)

const (
	StatusPending = "pending"
	StatusPaid    = "paid"
	StatusVoid    = "void"
)

var (
	ErrInvalidTransition = errors.New("billing: invalid invoice status transition")
	ErrNotFound          = errors.New("billing: invoice not found")
)

type Invoice struct {
	ID               int64      `json:"id"`
	BillingAccountID int64      `json:"-"`
	TeamID           int64      `json:"teamId"`
	SubscriptionID   *int64     `json:"subscriptionId"`
	PlanName         string     `json:"planName"`
	PriceCents       int        `json:"priceCents"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	PaidAt           *time.Time `json:"paidAt"`
}
