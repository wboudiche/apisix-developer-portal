package subscriptions

import (
	"time"

	"apisix-portal/internal/events"
)

// SubscriptionView is one of an application's subscriptions, enriched with the
// product and plan names for display, including its approval status.
type SubscriptionView struct {
	ProductID        int64  `json:"productId"`
	ProductName      string `json:"productName"`
	Version          string `json:"version"`
	ContextPath      string `json:"contextPath"`
	PlanID           int64  `json:"planId"`
	PlanName         string `json:"planName"`
	Status           string `json:"status"`
	SandboxAvailable bool   `json:"sandboxAvailable"`
}

// AppDetail is the response for GET /api/applications/{id}: the app's gateway
// key (empty until it has at least one subscription) and its subscriptions.
type AppDetail struct {
	APIKey            string             `json:"apiKey"`
	ConsumerUsername  string             `json:"consumerUsername"`
	Subscriptions     []SubscriptionView `json:"subscriptions"`
	Events            []events.View      `json:"events"`
	SandboxEnabled    bool               `json:"sandboxEnabled"`
	SandboxGatewayUrl string             `json:"sandboxGatewayUrl"`
	OIDCClientID      string             `json:"oidcClientId"`
	OAuthEligible     bool               `json:"oauthEligible"`
	OIDCIssuer        string             `json:"oidcIssuer"`
}

// SubscriptionRecord is the minimal subscription identity used by the approval
// flow (look up a subscription by id to provision/transition it).
type SubscriptionRecord struct {
	ID        int64
	AppID     int64
	ProductID int64
	PlanID    int64
	Status    string
}

// AdminSubscriptionView is one row of the admin approval queue.
type AdminSubscriptionView struct {
	ID              int64     `json:"id"`
	ApplicationName string    `json:"applicationName"`
	OwnerEmail      string    `json:"ownerEmail"`
	ProductName     string    `json:"productName"`
	Version         string    `json:"version"`
	PlanName        string    `json:"planName"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
}
