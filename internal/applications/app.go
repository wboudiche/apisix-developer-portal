package applications

import "time"

type Application struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"ownerId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	// SubscriptionCount and HasKey are populated by ListByOwner so the
	// applications list can show per-app status without an N+1 fetch. Create and
	// Get leave them zero/false — a brand-new app has no subscriptions or key,
	// and Get is used only for an ownership check.
	SubscriptionCount int  `json:"subscriptionCount"`
	HasKey            bool `json:"hasKey"`
}
