package admin

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/paging"
)

// ErrNotFound is returned when a product id does not exist.
var ErrNotFound = errors.New("admin: product not found")

// ErrSlugTaken is returned when a create/update would duplicate a slug.
var ErrSlugTaken = errors.New("admin: slug already exists")

// ErrContextPathTaken is returned when a create/update would duplicate a context path.
var ErrContextPathTaken = errors.New("admin: context path already in use")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const productCols = `id, name, slug, category, version, context_path, description, tags, icon, upstream_url, published`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Category, &p.Version,
		&p.ContextPath, &p.Description, &p.Tags, &p.Icon, &p.UpstreamURL, &p.Published)
	return p, err
}

func (r *Repo) ListAll(ctx context.Context, p paging.Params) ([]Product, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM api_products`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+productCols+` FROM api_products ORDER BY name ASC LIMIT $1 OFFSET $2`,
		p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		pr, err := scanProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, pr)
	}
	return out, total, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id int64) (Product, error) {
	p, err := scanProduct(r.pool.QueryRow(ctx, `SELECT `+productCols+` FROM api_products WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) Create(ctx context.Context, p Product) (Product, error) {
	created, err := scanProduct(r.pool.QueryRow(ctx,
		`INSERT INTO api_products(name, slug, category, version, context_path, description, tags, icon, upstream_url, published)
		 VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1.0.0'),$5,$6,$7,$8,$9,$10)
		 RETURNING `+productCols,
		p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published))
	if err != nil {
		return Product{}, uniqueErr(err)
	}
	return created, nil
}

func (r *Repo) Update(ctx context.Context, p Product) (Product, error) {
	updated, err := scanProduct(r.pool.QueryRow(ctx,
		`UPDATE api_products SET name=$2, slug=$3, category=$4, version=COALESCE(NULLIF($5,''),'1.0.0'),
		   context_path=$6, description=$7, tags=$8, icon=$9, upstream_url=$10, published=$11
		 WHERE id=$1
		 RETURNING `+productCols,
		p.ID, p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, uniqueErr(err)
	}
	return updated, nil
}

// ContextPathOverlaps reports whether p would collide with an existing
// product's route prefix: equal, or a path-prefix on a "/" boundary in either
// direction (/v1 vs /v1/orders — APISIX's /v1/* shadows /v1/orders/*).
// "_" is escaped on the pattern side of each LIKE: it is a single-char
// wildcard in LIKE but a legal context-path character ("%" is not, so it
// needs no escaping).
func (r *Repo) ContextPathOverlaps(ctx context.Context, p string, exceptID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM api_products
		   WHERE id <> $2
		     AND (context_path = $1
		          OR context_path LIKE replace($1, '_', '\_') || '/%'
		          OR $1 LIKE replace(context_path, '_', '\_') || '/%'))`,
		p, exceptID).Scan(&exists)
	return exists, err
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) CountActiveSubscriptions(ctx context.Context, productID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions WHERE api_product_id=$1 AND status='active'`, productID,
	).Scan(&n)
	return n, err
}

// isUniqueViolation returns true for Postgres 23505 unique-constraint errors.
// Used by plan_repo.go where a single unique constraint is in play.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// uniqueErr converts a 23505 unique-violation into the appropriate sentinel
// error based on the constraint name; all others pass through unchanged.
func uniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "api_products_context_path_key" {
			return ErrContextPathTaken
		}
		return ErrSlugTaken
	}
	return err
}
