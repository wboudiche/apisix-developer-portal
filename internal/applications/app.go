package applications

import "time"

type Application struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"ownerId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	// SubscriptionCount is populated by ListByOwner so the applications list can
	// show each app's subscription count without an N+1 fetch. Create and Get
	// leave it zero — a brand-new app has no subscriptions, and Get is used only
	// for an ownership check.
	SubscriptionCount int `json:"subscriptionCount"`
}
