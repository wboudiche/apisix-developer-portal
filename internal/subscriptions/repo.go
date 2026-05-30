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
