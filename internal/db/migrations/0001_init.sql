CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'developer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    rate_limit_count    INT  NOT NULL,
    rate_limit_window_s INT  NOT NULL
);

CREATE TABLE IF NOT EXISTS api_products (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    category     TEXT NOT NULL,
    version      TEXT NOT NULL DEFAULT '1.0.0',
    context_path TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    icon         TEXT NOT NULL DEFAULT '',
    rating       NUMERIC(2,1) NOT NULL DEFAULT 0,
    published    BOOLEAN NOT NULL DEFAULT true,
    upstream_url TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_products_category ON api_products(category);
