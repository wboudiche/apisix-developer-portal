// Package events is the application activity log behind the Overview feed.
// Writes are append-only and best-effort (a failed log must never fail the
// action it describes); reads resolve product/plan names with a LEFT JOIN so
// the feed reflects current names and tolerates deleted products.
package events

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Event kinds. Stored verbatim in app_events.kind; the frontend maps each to an
// icon and label, so keep these stable.
const (
	KindAppCreated        = "app_created"
	KindSubscribed        = "subscribed"
	KindApproved          = "approved"
	KindRejected          = "rejected"
	KindUnsubscribed      = "unsubscribed"
	KindKeyRotated        = "key_rotated"
	KindSandboxEnabled    = "sandbox_enabled"
	KindSandboxKeyRotated = "sandbox_key_rotated"
)

// View is one feed entry as returned in the application detail response. The
// product/plan names are resolved at read time and empty when not applicable
// (app_created) or when the referenced row was since deleted.
type View struct {
	Kind        string    `json:"kind"`
	ProductName string    `json:"productName"`
	PlanName    string    `json:"planName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Log appends an activity event. productID/planID may be nil when not relevant
// (e.g. app creation). Callers treat a returned error as non-fatal: the action
// already happened; only the feed entry is lost.
func (r *Repo) Log(ctx context.Context, appID int64, kind string, productID, planID *int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO app_events(application_id, kind, product_id, plan_id) VALUES($1,$2,$3,$4)`,
		appID, kind, productID, planID)
	return err
}

// Recent returns the latest `limit` events for an application, newest first,
// with product/plan names resolved.
func (r *Repo) Recent(ctx context.Context, appID int64, limit int) ([]View, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT e.kind, COALESCE(p.name,''), COALESCE(pl.name,''), e.created_at
		   FROM app_events e
		   LEFT JOIN api_products p ON p.id = e.product_id
		   LEFT JOIN plans pl       ON pl.id = e.plan_id
		  WHERE e.application_id = $1
		  ORDER BY e.created_at DESC, e.id DESC
		  LIMIT $2`,
		appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]View, 0, limit)
	for rows.Next() {
		var v View
		if err := rows.Scan(&v.Kind, &v.ProductName, &v.PlanName, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
