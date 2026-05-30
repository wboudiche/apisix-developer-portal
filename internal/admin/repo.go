package admin

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a product id does not exist.
var ErrNotFound = errors.New("admin: product not found")

// ErrSlugTaken is returned when a create/update would duplicate a slug.
var ErrSlugTaken = errors.New("admin: slug already exists")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const productCols = `id, name, slug, category, version, context_path, description, tags, icon, upstream_url, published`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Category, &p.Version,
		&p.ContextPath, &p.Description, &p.Tags, &p.Icon, &p.UpstreamURL, &p.Published)
	return p, err
}

func (r *Repo) ListAll(ctx context.Context) ([]Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+productCols+` FROM api_products ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
	if isUniqueViolation(err) {
		return Product{}, ErrSlugTaken
	}
	return created, err
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
	if isUniqueViolation(err) {
		return Product{}, ErrSlugTaken
	}
	return updated, err
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
