package subscriptions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/crypto"
	"apisix-portal/internal/paging"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

func NewRepo(pool *pgxpool.Pool, cipher *crypto.Cipher) *Repo {
	return &Repo{pool: pool, cipher: cipher}
}

// GenerateKey returns a random 32-hex-char API key. It panics if the system
// CSPRNG fails (an unrecoverable entropy outage) rather than emitting a
// predictable all-zero key.
func GenerateKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("subscriptions: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (r *Repo) GetOrCreateCredential(ctx context.Context, appID int64, genKey func() string) (Credential, error) {
	// Atomic upsert: one credential per application, race-safe across concurrent
	// subscribe calls. ON CONFLICT performs a no-op UPDATE so RETURNING yields the
	// existing row; a freshly generated key on a losing INSERT is simply discarded.
	plain := genKey()
	encKey, err := r.cipher.Encrypt(plain)
	if err != nil {
		return Credential{}, err
	}
	var c Credential
	var stored string
	err = r.pool.QueryRow(ctx,
		`INSERT INTO credentials(application_id, api_key, consumer_username) VALUES($1,$2,$3)
		 ON CONFLICT (application_id) DO UPDATE SET application_id = credentials.application_id
		 RETURNING application_id, api_key, consumer_username`,
		appID, encKey, consumerName(appID),
	).Scan(&c.ApplicationID, &stored, &c.ConsumerUsername)
	if err != nil {
		return Credential{}, err
	}
	c.APIKey, err = r.cipher.Decrypt(stored)
	if err != nil {
		return Credential{}, err
	}
	return c, nil
}

func (r *Repo) GetProduct(ctx context.Context, id int64) (ProductInfo, error) {
	var p ProductInfo
	err := r.pool.QueryRow(ctx,
		`SELECT id, context_path, upstream_url, sandbox_upstream_url, published, auth_type FROM api_products WHERE id=$1`, id,
	).Scan(&p.ID, &p.ContextPath, &p.Upstream, &p.SandboxUpstream, &p.Published, &p.AuthType)
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
		`INSERT INTO subscriptions(application_id, api_product_id, plan_id, status) VALUES($1,$2,$3,'pending')
		 ON CONFLICT (application_id, api_product_id)
		 DO UPDATE SET plan_id=EXCLUDED.plan_id, status='pending'`,
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

// ConsumersForPlan returns the credentials of every application with an active
// subscription on the plan (used to re-apply new rate limits to live consumers).
func (r *Repo) ConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT c.application_id, c.api_key, c.consumer_username
		 FROM subscriptions s
		 JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.plan_id=$1 AND s.status='active'`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		var stored string
		if err := rows.Scan(&c.ApplicationID, &stored, &c.ConsumerUsername); err != nil {
			return nil, err
		}
		plain, err := r.cipher.Decrypt(stored)
		if err != nil {
			return nil, err
		}
		c.APIKey = plain
		out = append(out, c)
	}
	return out, rows.Err()
}

// OAuthClientsForProduct returns the client ids of active subscribers whose app
// has a non-empty oidc_client_id (the OAuth2 route whitelist).
func (r *Repo) OAuthClientsForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT a.oidc_client_id FROM subscriptions s
		   JOIN applications a ON a.id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active' AND a.oidc_client_id <> ''`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// OAuthProductsForApp returns the oauth2 products the app is actively subscribed to.
func (r *Repo) OAuthProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT p.id, p.context_path, p.upstream_url, p.auth_type
		   FROM subscriptions s JOIN api_products p ON p.id = s.api_product_id
		 WHERE s.application_id=$1 AND s.status='active' AND p.auth_type='oauth2'`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductInfo
	for rows.Next() {
		var p ProductInfo
		if err := rows.Scan(&p.ID, &p.ContextPath, &p.Upstream, &p.AuthType); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) GetAppOIDCClientID(ctx context.Context, appID int64) (string, error) {
	var cid string
	err := r.pool.QueryRow(ctx, `SELECT oidc_client_id FROM applications WHERE id=$1`, appID).Scan(&cid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return cid, err
}

func (r *Repo) SetAppOIDCClientID(ctx context.Context, appID int64, clientID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE applications SET oidc_client_id=$2 WHERE id=$1`, appID, clientID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

var _ Store = (*Repo)(nil)

// GetCredential returns the application's credential, or ErrNotFound if it has none yet.
func (r *Repo) GetCredential(ctx context.Context, appID int64) (Credential, error) {
	var c Credential
	var stored string
	err := r.pool.QueryRow(ctx,
		`SELECT application_id, api_key, consumer_username FROM credentials WHERE application_id=$1`, appID,
	).Scan(&c.ApplicationID, &stored, &c.ConsumerUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	plain, err := r.cipher.Decrypt(stored)
	if err != nil {
		return Credential{}, err
	}
	c.APIKey = plain
	return c, nil
}

// ActivePlanForApp returns the plan of the application's most-recent active
// subscription, or ErrNoActiveSubscription when none exists.
func (r *Repo) ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error) {
	var p PlanInfo
	err := r.pool.QueryRow(ctx,
		`SELECT pl.id, pl.rate_limit_count, pl.rate_limit_window_s
		   FROM subscriptions s
		   JOIN plans pl ON pl.id = s.plan_id
		  WHERE s.application_id=$1 AND s.status='active'
		  ORDER BY s.created_at DESC
		  LIMIT 1`, appID,
	).Scan(&p.ID, &p.Count, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanInfo{}, ErrNoActiveSubscription
	}
	return p, err
}

// UpdateCredentialKey replaces the stored API key for the application's credential.
func (r *Repo) UpdateCredentialKey(ctx context.Context, appID int64, newKey string) error {
	encKey, err := r.cipher.Encrypt(newKey)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE credentials SET api_key=$2 WHERE application_id=$1`, appID, encKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SubscriptionsForApp returns the application's subscriptions for display,
// including pending/rejected ones so the developer can see their status.
func (r *Repo) SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.api_product_id, p.name, p.version, p.context_path, s.plan_id, pl.name, s.status, (p.sandbox_upstream_url <> '')
		 FROM subscriptions s
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE s.application_id=$1
		 ORDER BY p.name`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionView
	for rows.Next() {
		var v SubscriptionView
		if err := rows.Scan(&v.ProductID, &v.ProductName, &v.Version, &v.ContextPath, &v.PlanID, &v.PlanName, &v.Status, &v.SandboxAvailable); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetSubscription returns the subscription's identity + status, or ErrNotFound.
func (r *Repo) GetSubscription(ctx context.Context, subID int64) (SubscriptionRecord, error) {
	var s SubscriptionRecord
	err := r.pool.QueryRow(ctx,
		`SELECT id, application_id, api_product_id, plan_id, status FROM subscriptions WHERE id=$1`, subID,
	).Scan(&s.ID, &s.AppID, &s.ProductID, &s.PlanID, &s.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionRecord{}, ErrNotFound
	}
	return s, err
}

// SubscriptionStatus returns the status of the (app, product) subscription, or "" if none.
func (r *Repo) SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx,
		`SELECT status FROM subscriptions WHERE application_id=$1 AND api_product_id=$2`, appID, productID,
	).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return status, err
}

// SetSubscriptionStatus transitions a subscription to the given status.
func (r *Repo) SetSubscriptionStatus(ctx context.Context, subID int64, status string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE subscriptions SET status=$2 WHERE id=$1`, subID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminSubscriptions lists subscriptions for the admin queue, newest first. An
// empty statusFilter returns all rows; otherwise only those with that status.
func (r *Repo) AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions s WHERE ($1 = '' OR s.status = $1)`, statusFilter,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, a.name, u.email, p.name, p.version, pl.name, s.status, s.created_at
		 FROM subscriptions s
		 JOIN applications a ON a.id = s.application_id
		 JOIN users u ON u.id = a.owner_id
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE ($1 = '' OR s.status = $1)
		 ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`, statusFilter, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminSubscriptionView
	for rows.Next() {
		var v AdminSubscriptionView
		if err := rows.Scan(&v.ID, &v.ApplicationName, &v.OwnerEmail, &v.ProductName, &v.Version, &v.PlanName, &v.Status, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// AppRef is an application id+name the developer can use for Try-it.
type AppRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ApprovedAppsForProduct returns the user's applications that hold an ACTIVE
// subscription to the product.
func (r *Repo) ApprovedAppsForProduct(ctx context.Context, userID, productID int64) ([]AppRef, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.name FROM applications a
		   JOIN subscriptions s ON s.application_id = a.id
		 WHERE a.owner_id=$1 AND s.api_product_id=$2 AND s.status='active'
		 ORDER BY a.created_at`, userID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppRef
	for rows.Next() {
		var a AppRef
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetSandboxKey returns the application's decrypted sandbox key ("" when not
// enabled), or ErrNotFound when the app has no credential row.
func (r *Repo) GetSandboxKey(ctx context.Context, appID int64) (string, error) {
	var stored string
	err := r.pool.QueryRow(ctx,
		`SELECT sandbox_api_key FROM credentials WHERE application_id=$1`, appID).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if stored == "" {
		return "", nil
	}
	return r.cipher.Decrypt(stored)
}

// UpdateSandboxKey encrypts and stores the application's sandbox key.
func (r *Repo) UpdateSandboxKey(ctx context.Context, appID int64, key string) error {
	enc, err := r.cipher.Encrypt(key)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE credentials SET sandbox_api_key=$2 WHERE application_id=$1`, appID, enc)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SandboxConsumersForProduct returns the usernames of active subscribers whose
// app has a sandbox key (the product's sandbox-route whitelist).
func (r *Repo) SandboxConsumersForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.consumer_username FROM subscriptions s
		   JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active' AND c.sandbox_api_key <> ''`, productID)
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

// SandboxConsumersForPlan returns the sandbox credential (username + sandbox key)
// of active subscribers on the plan whose app has a sandbox key.
func (r *Repo) SandboxConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT c.application_id, c.sandbox_api_key, c.consumer_username
		   FROM subscriptions s
		   JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.plan_id=$1 AND s.status='active' AND c.sandbox_api_key <> ''`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		var stored string
		if err := rows.Scan(&c.ApplicationID, &stored, &c.ConsumerUsername); err != nil {
			return nil, err
		}
		if c.APIKey, err = r.cipher.Decrypt(stored); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SandboxProductsForApp returns the products the app is ACTIVELY subscribed to
// that have a sandbox upstream configured.
func (r *Repo) SandboxProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT p.id, p.context_path, p.sandbox_upstream_url
		   FROM subscriptions s
		   JOIN api_products p ON p.id = s.api_product_id
		 WHERE s.application_id=$1 AND s.status='active' AND p.sandbox_upstream_url <> ''`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductInfo
	for rows.Next() {
		var p ProductInfo
		if err := rows.Scan(&p.ID, &p.ContextPath, &p.SandboxUpstream); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

var _ Reader = (*Repo)(nil)
