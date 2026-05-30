ALTER TABLE api_products ADD COLUMN IF NOT EXISTS apisix_route_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS applications (
    id          BIGSERIAL PRIMARY KEY,
    owner_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_applications_owner ON applications(owner_id);

CREATE TABLE IF NOT EXISTS credentials (
    id                BIGSERIAL PRIMARY KEY,
    application_id    BIGINT NOT NULL UNIQUE REFERENCES applications(id) ON DELETE CASCADE,
    api_key           TEXT NOT NULL UNIQUE,
    consumer_username TEXT NOT NULL UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id             BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    api_product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    plan_id        BIGINT NOT NULL REFERENCES plans(id),
    status         TEXT NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, api_product_id)
);
