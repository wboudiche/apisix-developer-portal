package billing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const invoiceCols = `id, billing_account_id, team_id, subscription_id, plan_name, price_cents, currency, status, created_at, paid_at`

func scanInvoice(row pgx.Row) (Invoice, error) {
	var v Invoice
	err := row.Scan(&v.ID, &v.BillingAccountID, &v.TeamID, &v.SubscriptionID,
		&v.PlanName, &v.PriceCents, &v.Currency, &v.Status, &v.CreatedAt, &v.PaidAt)
	return v, err
}

// PlanPricing returns the plan's snapshot pricing.
func (r *Repo) PlanPricing(ctx context.Context, planID int64) (name string, priceCents int, currency string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT name, price_cents, currency FROM plans WHERE id=$1`, planID).
		Scan(&name, &priceCents, &currency)
	return
}

// TeamForApp returns the team that owns the app.
func (r *Repo) TeamForApp(ctx context.Context, appID int64) (int64, error) {
	var teamID int64
	err := r.pool.QueryRow(ctx, `SELECT team_id FROM applications WHERE id=$1`, appID).Scan(&teamID)
	return teamID, err
}

// EnsureAccount returns the team's billing account id, creating it if absent.
func (r *Repo) EnsureAccount(ctx context.Context, teamID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO billing_accounts(team_id) VALUES($1)
		 ON CONFLICT (team_id) DO UPDATE SET team_id=EXCLUDED.team_id
		 RETURNING id`, teamID).Scan(&id)
	return id, err
}

// PendingInvoiceExists reports whether a non-void invoice already exists for the
// subscription (idempotency guard for re-approval).
func (r *Repo) PendingInvoiceExists(ctx context.Context, subID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM invoices WHERE subscription_id=$1 AND status <> 'void')`, subID).Scan(&exists)
	return exists, err
}

func (r *Repo) CreateInvoice(ctx context.Context, accountID, teamID, subID int64, planName string, priceCents int, currency string) (Invoice, error) {
	return scanInvoice(r.pool.QueryRow(ctx,
		`INSERT INTO invoices(billing_account_id, team_id, subscription_id, plan_name, price_cents, currency, status)
		 VALUES($1,$2,$3,$4,$5,$6,'pending') RETURNING `+invoiceCols,
		accountID, teamID, subID, planName, priceCents, currency))
}

func (r *Repo) Get(ctx context.Context, id int64) (Invoice, error) {
	v, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	return v, err
}

// MarkPaid flips a pending invoice to paid; any other current status → ErrInvalidTransition.
func (r *Repo) MarkPaid(ctx context.Context, id int64) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE invoices SET status='paid', paid_at=now() WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return r.transitionError(ctx, id)
	}
	return nil
}

// Void flips a pending invoice to void; any other current status → ErrInvalidTransition.
func (r *Repo) Void(ctx context.Context, id int64) error {
	ct, err := r.pool.Exec(ctx, `UPDATE invoices SET status='void' WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return r.transitionError(ctx, id)
	}
	return nil
}

// transitionError distinguishes "no such invoice" from "wrong current status".
func (r *Repo) transitionError(ctx context.Context, id int64) error {
	if _, err := r.Get(ctx, id); errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return ErrInvalidTransition
}

func (r *Repo) list(ctx context.Context, where string, args ...any) ([]Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices `+where+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		v, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListByTeams returns invoices for the given team ids (newest first).
func (r *Repo) ListByTeams(ctx context.Context, teamIDs []int64) ([]Invoice, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	return r.list(ctx, `WHERE team_id = ANY($1)`, teamIDs)
}

// ListAll returns every invoice, optionally filtered by status ("" = all).
func (r *Repo) ListAll(ctx context.Context, status string) ([]Invoice, error) {
	if status == "" {
		return r.list(ctx, ``)
	}
	return r.list(ctx, `WHERE status=$1`, status)
}

// TeamsForUser returns the ids of the teams the user belongs to.
func (r *Repo) TeamsForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT team_id FROM team_members WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
