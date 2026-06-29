-- Per-product sandbox backend + per-app sandbox key (encrypted, '' = disabled).
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS sandbox_upstream_url TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials   ADD COLUMN IF NOT EXISTS sandbox_api_key      TEXT NOT NULL DEFAULT '';
