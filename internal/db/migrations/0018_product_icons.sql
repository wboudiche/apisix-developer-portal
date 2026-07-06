CREATE TABLE product_icons (
    product_id BIGINT PRIMARY KEY REFERENCES api_products(id) ON DELETE CASCADE,
    data       BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
