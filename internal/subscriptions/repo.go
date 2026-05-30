package subscriptions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// GenerateKey returns a random 32-hex-char API key.
func GenerateKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Repo) GetOrCreateCredential(ctx context.Context, appID int64, genKey func() string) (Credential, error) {
	// Atomic upsert: one credential per application, race-safe across concurrent
	// subscribe calls. ON CONFLICT performs a no-op UPDATE so RETURNING yields the
	// existing row; a freshly generated key on a losing INSERT is simply discarded.
	want := Credential{ApplicationID: appID, APIKey: genKey(), ConsumerUsername: consumerName(appID)}
	var c Credential
	err := r.pool.QueryRow(ctx,
		`INSERT INTO credentials(application_id, api_key, consumer_username) VALUES($1,$2,$3)
		 ON CONFLICT (application_id) DO UPDATE SET application_id = credentials.application_id
		 RETURNING application_id, api_key, consumer_username`,
		want.ApplicationID, want.APIKey, want.ConsumerUsername,
	).Scan(&c.ApplicationID, &c.APIKey, &c.ConsumerUsername)
	return c, err
}

func (r *Repo) GetProduct(ctx context.Context, id int64) (ProductInfo, error) {
	var p ProductInfo
	err := r.pool.QueryRow(ctx,
		`SELECT id, context_path, upstream_url FROM api_products WHERE id=$1`, id,
	).Scan(&p.ID, &p.ContextPath, &p.Upstream)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductInfo{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) GetPlan(ctx context.Context, id int64) (PlanInfo, error) {
	var p PlanInfo
	err := r.pool.QueryRow(ctx,
		`SELECT id, rate_limit_count, rate_limit_window_s FROM plans WHERE id=$1`, id,
	).Scan(&p.ID, &p.Count, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanInfo{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) SaveSubscription(ctx context.Context, appID, productID, planID int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id, api_product_id, plan_id) VALUES($1,$2,$3)
		 ON CONFLICT (application_id, api_product_id)
		 DO UPDATE SET plan_id=EXCLUDED.plan_id, status='active'`,
		appID, productID, planID)
	return err
}

func (r *Repo) DeleteSubscription(ctx context.Context, appID, productID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE application_id=$1 AND api_product_id=$2`, appID, productID)
	return err
}

func (r *Repo) ConsumersForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.consumer_username FROM subscriptions s
		 JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active'`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

var _ Store = (*Repo)(nil)

// GetCredential returns the application's credential, or ErrNotFound if it has none yet.
func (r *Repo) GetCredential(ctx context.Context, appID int64) (Credential, error) {
	var c Credential
	err := r.pool.QueryRow(ctx,
		`SELECT application_id, api_key, consumer_username FROM credentials WHERE application_id=$1`, appID,
	).Scan(&c.ApplicationID, &c.APIKey, &c.ConsumerUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	return c, err
}

// SubscriptionsForApp returns the application's active subscriptions for display.
func (r *Repo) SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.api_product_id, p.name, p.version, p.context_path, s.plan_id, pl.name
		 FROM subscriptions s
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE s.application_id=$1 AND s.status='active'
		 ORDER BY p.name`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionView
	for rows.Next() {
		var v SubscriptionView
		if err := rows.Scan(&v.ProductID, &v.ProductName, &v.Version, &v.ContextPath, &v.PlanID, &v.PlanName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ Reader = (*Repo)(nil)
