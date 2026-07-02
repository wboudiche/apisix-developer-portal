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

const productCols = `id, name, slug, category, version, context_path, description, tags, icon, upstream_url, sandbox_upstream_url, published, auth_type, lifecycle_status, to_char(sunset_date,'YYYY-MM-DD')`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Category, &p.Version,
		&p.ContextPath, &p.Description, &p.Tags, &p.Icon, &p.UpstreamURL, &p.SandboxUpstreamURL, &p.Published, &p.AuthType,
		&p.LifecycleStatus, &p.SunsetDate)
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
		`INSERT INTO api_products(name, slug, category, version, context_path, description, tags, icon, upstream_url, sandbox_upstream_url, published, openapi_spec, auth_type, lifecycle_status, sunset_date)
		 VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1.0.0'),$5,$6,$7,$8,$9,$10,$11,$12,COALESCE(NULLIF($13,''),'key-auth'),COALESCE(NULLIF($14,''),'active'),NULLIF($15,'')::date)
		 RETURNING `+productCols,
		p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.SandboxUpstreamURL, p.Published, p.OpenAPISpec, p.AuthType, p.LifecycleStatus, derefStr(p.SunsetDate)))
	if err != nil {
		return Product{}, uniqueErr(err)
	}
	return created, nil
}

func (r *Repo) Update(ctx context.Context, p Product) (Product, error) {
	updated, err := scanProduct(r.pool.QueryRow(ctx,
		`UPDATE api_products SET name=$2, slug=$3, category=$4, version=COALESCE(NULLIF($5,''),'1.0.0'),
		   context_path=$6, description=$7, tags=$8, icon=$9, upstream_url=$10, sandbox_upstream_url=$11, published=$12,
		   openapi_spec=COALESCE(NULLIF($13,''), openapi_spec),
		   auth_type=COALESCE(NULLIF($14,''),'key-auth'),
		   lifecycle_status=COALESCE(NULLIF($15,''),'active'), sunset_date=NULLIF($16,'')::date
		 WHERE id=$1
		 RETURNING `+productCols,
		p.ID, p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.SandboxUpstreamURL, p.Published, p.OpenAPISpec, p.AuthType, p.LifecycleStatus, derefStr(p.SunsetDate)))
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

// derefStr returns "" for a nil pointer, otherwise the pointed-to value —
// used to pass a nullable *string date field as a query param.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// AddChangelog inserts a changelog entry for productID and returns it with its
// generated id.
func (r *Repo) AddChangelog(ctx context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO changelog_entries(product_id, version, kind, notes, entry_date)
		 VALUES($1,$2,$3,$4,$5::date)
		 RETURNING id, version, kind, notes, to_char(entry_date,'YYYY-MM-DD')`,
		productID, e.Version, e.Kind, e.Notes, e.Date).
		Scan(&e.ID, &e.Version, &e.Kind, &e.Notes, &e.Date)
	return e, err
}

// ListChangelog returns all changelog entries for productID, newest first.
// Unlike the public catalog listing, this is not filtered to published
// products/entries — admins need to see (and delete) entries on drafts too.
func (r *Repo) ListChangelog(ctx context.Context, productID int64) ([]ChangelogEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, version, kind, notes, to_char(entry_date,'YYYY-MM-DD') FROM changelog_entries
		 WHERE product_id=$1 ORDER BY entry_date DESC, id DESC`,
		productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangelogEntry
	for rows.Next() {
		var e ChangelogEntry
		if err := rows.Scan(&e.ID, &e.Version, &e.Kind, &e.Notes, &e.Date); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteChangelog removes a changelog entry, scoped to productID so one
// product's admin can't delete another product's entries by id guessing.
// ErrNotFound when no matching row exists.
func (r *Repo) DeleteChangelog(ctx context.Context, productID, entryID int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM changelog_entries WHERE id=$1 AND product_id=$2`, entryID, productID)
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
