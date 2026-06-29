-- Per-product auth method + per-app OIDC client id (plaintext; not a secret).
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS auth_type TEXT NOT NULL DEFAULT 'key-auth'
    CHECK (auth_type IN ('key-auth','oauth2'));
ALTER TABLE applications ADD COLUMN IF NOT EXISTS oidc_client_id TEXT NOT NULL DEFAULT '';
