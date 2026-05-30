package subscriptions

// SubscriptionView is one of an application's active subscriptions, enriched
// with the product and plan names for display.
type SubscriptionView struct {
	ProductID   int64  `json:"productId"`
	ProductName string `json:"productName"`
	Version     string `json:"version"`
	ContextPath string `json:"contextPath"`
	PlanID      int64  `json:"planId"`
	PlanName    string `json:"planName"`
}

// AppDetail is the response for GET /api/applications/{id}: the app's gateway
// key (empty until it has at least one subscription) and its subscriptions.
type AppDetail struct {
	APIKey           string             `json:"apiKey"`
	ConsumerUsername string             `json:"consumerUsername"`
	Subscriptions    []SubscriptionView `json:"subscriptions"`
}
