-- Real per-user ratings; the api_products.rating/rating_count are a denormalized
-- cache recomputed on each write. Seeded ratings reset to real-only.
CREATE TABLE IF NOT EXISTS product_ratings (
    id             BIGSERIAL PRIMARY KEY,
    api_product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stars          SMALLINT NOT NULL CHECK (stars BETWEEN 1 AND 5),
    comment        TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (api_product_id, user_id)
);
CREATE INDEX IF NOT EXISTS product_ratings_product_idx ON product_ratings (api_product_id, created_at DESC);

ALTER TABLE api_products ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;
-- Real-only: drop the seeded static ratings.
UPDATE api_products SET rating = 0, rating_count = 0;
