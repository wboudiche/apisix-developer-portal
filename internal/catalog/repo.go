package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/paging"
)

// ErrNotFound is returned by GetBySlug when the product does not exist.
var ErrNotFound = errors.New("catalog: product not found")

// Repo provides read-only access to published API products.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo constructs a Repo backed by the given connection pool.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const baseSelect = `SELECT id, name, slug, category, version, context_path, description, tags, icon, rating
	FROM api_products WHERE published = true`

// filterClause builds the shared WHERE tail (after "published = true") and its
// args, so the count and page queries always apply the same filters.
func filterClause(q Query) (string, []any) {
	sql := ""
	args := []any{}
	if q.Category != "" {
		args = append(args, q.Category)
		sql += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if q.Tag != "" {
		args = append(args, q.Tag)
		sql += fmt.Sprintf(" AND $%d = ANY(tags)", len(args))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		n := len(args)
		sql += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", n, n)
	}
	return sql, args
}

// List returns one page of published products plus the total matching the same
// filters.
func (r *Repo) List(ctx context.Context, q Query, p paging.Params) ([]Product, int, error) {
	filter, args := filterClause(q)

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM api_products WHERE published = true`+filter, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	sql := baseSelect + filter
	if q.Sort == "alpha" {
		sql += " ORDER BY name ASC"
	} else {
		sql += " ORDER BY rating DESC, name ASC"
	}
	args = append(args, p.Limit(), p.Offset())
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanProducts(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetBySlug returns the product with the given slug, or ErrNotFound.
func (r *Repo) GetBySlug(ctx context.Context, slug string) (Product, error) {
	sql := baseSelect + " AND slug = $1"
	rows, err := r.pool.Query(ctx, sql, slug)
	if err != nil {
		return Product{}, err
	}
	defer rows.Close()

	products, err := scanProducts(rows)
	if err != nil {
		return Product{}, err
	}
	if len(products) == 0 {
		return Product{}, ErrNotFound
	}
	return products[0], nil
}

// ProductBySlug returns the id and context path of a PUBLISHED product, or
// ErrNotFound. Lighter than GetBySlug — used by the try-it proxy.
func (r *Repo) ProductBySlug(ctx context.Context, slug string) (int64, string, error) {
	var id int64
	var ctxPath string
	err := r.pool.QueryRow(ctx,
		`SELECT id, context_path FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&id, &ctxPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, ctxPath, err
}

// GetSpecBySlug returns the raw OpenAPI spec for a published product, or
// ErrNotFound when the product is missing, unpublished, or has no spec.
func (r *Repo) GetSpecBySlug(ctx context.Context, slug string) (string, error) {
	var spec string
	err := r.pool.QueryRow(ctx,
		`SELECT openapi_spec FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&spec)
	if errors.Is(err, pgx.ErrNoRows) || spec == "" {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return spec, nil
}

// scanProducts collects all rows into a slice of Product.
func scanProducts(rows pgx.Rows) ([]Product, error) {
	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Slug,
			&p.Category,
			&p.Version,
			&p.ContextPath,
			&p.Description,
			&p.Tags,
			&p.Icon,
			&p.Rating,
		); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}
