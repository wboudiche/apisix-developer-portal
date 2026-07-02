ALTER TABLE api_products
  ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'active'
    CHECK (lifecycle_status IN ('active','deprecated','sunset')),
  ADD COLUMN sunset_date DATE;

CREATE TABLE changelog_entries (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    version    TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('added','changed','fixed','removed','deprecated','security')),
    notes      TEXT NOT NULL DEFAULT '',
    entry_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_changelog_product ON changelog_entries(product_id, entry_date DESC);
